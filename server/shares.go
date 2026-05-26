package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ShareMode controls what a holder of a share token may do.
//
//   - viewer: read-only — server ignores all keystrokes, resize, and
//     control messages other than detach/close.
//   - writer: full input — equivalent to attaching with the main token,
//     but scoped to a single session and time-limited.
type ShareMode string

const (
	ShareModeViewer ShareMode = "viewer"
	ShareModeWriter ShareMode = "writer"
)

// Share is a time-limited, single-session attachment grant.
type Share struct {
	Token     string    `json:"token"`
	Session   string    `json:"session"`
	Mode      ShareMode `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired reports whether the share has passed its ExpiresAt time.
func (s *Share) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

type ShareManager struct {
	mu     sync.Mutex
	shares map[string]*Share
	stop   chan struct{}
}

func NewShareManager() *ShareManager {
	m := &ShareManager{
		shares: make(map[string]*Share),
		stop:   make(chan struct{}),
	}
	go m.sweepLoop()
	return m
}

// Shutdown stops the background sweeper. Idempotent.
func (m *ShareManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stop:
		// already closed
	default:
		close(m.stop)
	}
}

// Create issues a new share for the given session name and mode with
// the supplied TTL. The returned share is not validated against an
// actual Session — the caller should verify the session exists first.
func (m *ShareManager) Create(session string, mode ShareMode, ttl time.Duration) (*Share, error) {
	if mode != ShareModeViewer && mode != ShareModeWriter {
		return nil, fmt.Errorf("invalid share mode: %q", mode)
	}
	if ttl <= 0 {
		return nil, errors.New("ttl must be positive")
	}
	if ttl > 30*24*time.Hour {
		return nil, errors.New("ttl too long (max 30 days)")
	}
	now := time.Now()
	s := &Share{
		Token:     generateShareToken(),
		Session:   session,
		Mode:      mode,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	m.mu.Lock()
	m.shares[s.Token] = s
	m.mu.Unlock()
	return s, nil
}

// Resolve returns the share for the given token if it exists and has
// not expired.
func (m *ShareManager) Resolve(token string) (*Share, bool) {
	if token == "" {
		return nil, false
	}
	m.mu.Lock()
	s, ok := m.shares[token]
	if ok && s.IsExpired() {
		delete(m.shares, token)
		ok = false
		s = nil
	}
	m.mu.Unlock()
	return s, ok
}

// Revoke removes a share by token. Returns false if not present.
func (m *ShareManager) Revoke(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.shares[token]; !ok {
		return false
	}
	delete(m.shares, token)
	return true
}

// RevokeForSession removes all shares pointing at a given session name.
// Used when the session itself is killed.
func (m *ShareManager) RevokeForSession(session string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tok, s := range m.shares {
		if s.Session == session {
			delete(m.shares, tok)
		}
	}
}

// List returns a snapshot of all live shares.
func (m *ShareManager) List() []Share {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Share, 0, len(m.shares))
	for _, s := range m.shares {
		if s.IsExpired() {
			continue
		}
		out = append(out, *s)
	}
	return out
}

func (m *ShareManager) sweepLoop() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.sweep()
		}
	}
}

func (m *ShareManager) sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tok, s := range m.shares {
		if s.IsExpired() {
			delete(m.shares, tok)
		}
	}
}

func generateShareToken() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	// URL-safe with no padding: hex is fine for our scale.
	return strings.ToLower(hex.EncodeToString(b))
}
