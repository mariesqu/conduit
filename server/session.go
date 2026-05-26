package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	xpty "github.com/aymanbagabas/go-pty"
)

const (
	maxBufferBytes = 256 * 1024 // per-session ring of recent PTY output (~2k lines)
	attachQueueCap = 256        // per-attachment outbound queue capacity
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-]{0,31}$`)

// Attachment is a single WebSocket subscriber to a Session.
// The Out channel is closed by the Session when the attachment is removed
// (either by Detach or because the client was too slow to keep up).
type Attachment struct {
	Out  chan []byte
	done chan struct{}
}

// IsClosed reports whether this attachment's outbound channel has been closed.
func (a *Attachment) IsClosed() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

type Session struct {
	Name      string
	Shell     string
	CreatedAt time.Time

	pty xpty.Pty
	cmd *xpty.Cmd

	mu          sync.Mutex
	writeMu     sync.Mutex // serializes pty writes (multiple attachments may type)
	buffer      []byte
	attachments map[*Attachment]struct{}
	closed      bool
	closeReason string

	onClose func() // set by SessionManager so it can drop us from the map
}

// SessionInfo is the JSON view of a session for the REST API.
type SessionInfo struct {
	Name      string    `json:"name"`
	Shell     string    `json:"shell"`
	CreatedAt time.Time `json:"created_at"`
	Attached  int       `json:"attached"`
	Alive     bool      `json:"alive"`
}

func (s *Session) Info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionInfo{
		Name:      s.Name,
		Shell:     s.Shell,
		CreatedAt: s.CreatedAt,
		Attached:  len(s.attachments),
		Alive:     !s.closed,
	}
}

// Attach registers a new subscriber and returns the current replay buffer.
// The returned attachment's Out channel will receive subsequent PTY output.
// When the session ends (or the client is too slow), Out is closed.
func (s *Session) Attach() (*Attachment, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, errors.New("session ended")
	}
	a := &Attachment{
		Out:  make(chan []byte, attachQueueCap),
		done: make(chan struct{}),
	}
	s.attachments[a] = struct{}{}
	backlog := append([]byte(nil), s.buffer...)
	return a, backlog, nil
}

// Detach removes an attachment. Safe to call multiple times.
func (s *Session) Detach(a *Attachment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeAttachmentLocked(a)
}

func (s *Session) removeAttachmentLocked(a *Attachment) {
	if _, ok := s.attachments[a]; !ok {
		return
	}
	delete(s.attachments, a)
	close(a.Out)
	close(a.done)
}

// Write sends bytes to the PTY stdin. Concurrent-safe across attachments.
func (s *Session) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.pty.Write(p)
}

// Resize updates the PTY window size. Last-writer-wins across attachments.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.pty.Resize(int(cols), int(rows))
}

// Kill terminates the underlying shell. The session will transition to closed
// and all attachments will be notified via their Out channel closing.
func (s *Session) Kill() {
	s.closeInternal("killed")
}

func (s *Session) closeInternal(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeReason = reason
	atts := make([]*Attachment, 0, len(s.attachments))
	for a := range s.attachments {
		atts = append(atts, a)
	}
	for _, a := range atts {
		s.removeAttachmentLocked(a)
	}
	s.mu.Unlock()

	if err := s.pty.Close(); err != nil {
		log.Printf("session %s: close pty: %v", s.Name, err)
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		// cmd.Wait performs go-pty's platform-specific cleanup
		// (Windows ConPTY handles, Unix wait+reap). Bypassing it via
		// cmd.Process.Wait leaks resources on Windows.
		_ = s.cmd.Wait()
	}
	if s.onClose != nil {
		s.onClose()
	}
}

// run is the PTY reader loop: reads PTY → appends to buffer → broadcasts to
// all attachments. Exits (and closes the session) when the PTY is done.
func (s *Session) run() {
	defer s.closeInternal("shell exited")
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) broadcast(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.appendToBufferLocked(data)
	var slow []*Attachment
	for a := range s.attachments {
		select {
		case a.Out <- data:
		default:
			slow = append(slow, a)
		}
	}
	for _, a := range slow {
		log.Printf("session %s: dropping slow attachment", s.Name)
		s.removeAttachmentLocked(a)
	}
}

func (s *Session) appendToBufferLocked(data []byte) {
	s.buffer = append(s.buffer, data...)
	if len(s.buffer) > maxBufferBytes {
		keep := s.buffer[len(s.buffer)-maxBufferBytes:]
		s.buffer = append(make([]byte, 0, maxBufferBytes), keep...)
	}
}

// ---------------- SessionManager ----------------

type SessionManager struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	maxSessions int

	// OnSessionRemoved is called (without holding any lock) after a
	// session is removed from the manager. Used to e.g. revoke shares
	// tied to a now-defunct session. Optional.
	OnSessionRemoved func(name string)
}

// ErrSessionLimit is returned by Create when MaxSessions would be exceeded.
var ErrSessionLimit = errors.New("session limit reached")

func NewSessionManager(maxSessions int) *SessionManager {
	if maxSessions <= 0 {
		maxSessions = DefaultMaxSessions
	}
	return &SessionManager{
		sessions:    make(map[string]*Session),
		maxSessions: maxSessions,
	}
}

// Create starts a new named session. If name is empty, one is generated.
// Returns an error if the name already exists or the shell can't start.
func (m *SessionManager) Create(name, shellName string, cols, rows uint16) (*Session, error) {
	shell := FindShellByName(shellName)
	if shell == nil {
		return nil, fmt.Errorf("unknown shell: %s", shellName)
	}
	m.mu.Lock()
	if len(m.sessions) >= m.maxSessions {
		m.mu.Unlock()
		return nil, ErrSessionLimit
	}
	if name == "" {
		name = m.generateNameLocked(shellName)
	} else {
		if !nameRe.MatchString(name) {
			m.mu.Unlock()
			return nil, errors.New("invalid name (use letters, digits, '-', '_'; max 32 chars)")
		}
		if _, exists := m.sessions[name]; exists {
			m.mu.Unlock()
			return nil, fmt.Errorf("session %q already exists", name)
		}
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	p, err := xpty.New()
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("create pty: %w", err)
	}
	if err := p.Resize(int(cols), int(rows)); err != nil {
		log.Printf("initial resize failed: %v", err)
	}
	cmd := p.Command(shell.Path, shell.Args...)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CONDUIT_SESSION="+name,
	)
	if err := cmd.Start(); err != nil {
		_ = p.Close()
		m.mu.Unlock()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	sess := &Session{
		Name:        name,
		Shell:       shellName,
		CreatedAt:   time.Now(),
		pty:         p,
		cmd:         cmd,
		attachments: make(map[*Attachment]struct{}),
	}
	sess.onClose = func() { m.remove(name) }
	m.sessions[name] = sess
	m.mu.Unlock()

	go sess.run()
	return sess, nil
}

func (m *SessionManager) Get(name string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[name]
	return s, ok
}

func (m *SessionManager) List() []SessionInfo {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	out := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info())
	}
	return out
}

// Kill terminates a session by name. Returns false if it doesn't exist.
func (m *SessionManager) Kill(name string) bool {
	s, ok := m.Get(name)
	if !ok {
		return false
	}
	s.Kill()
	return true
}

// Shutdown terminates every session — called on server shutdown.
func (m *SessionManager) Shutdown() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.Kill()
	}
}

func (m *SessionManager) remove(name string) {
	m.mu.Lock()
	delete(m.sessions, name)
	cb := m.OnSessionRemoved
	m.mu.Unlock()
	if cb != nil {
		cb(name)
	}
}

func (m *SessionManager) generateNameLocked(shellName string) string {
	prefix := shellName
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	for i := 0; i < 16; i++ {
		b := make([]byte, 3)
		if _, err := rand.Read(b); err != nil {
			panic(fmt.Errorf("crypto/rand failed: %w", err))
		}
		candidate := prefix + "-" + hex.EncodeToString(b)
		if _, exists := m.sessions[candidate]; !exists {
			return candidate
		}
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	return prefix + "-" + hex.EncodeToString(b)
}
