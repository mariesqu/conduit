package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// WebSocket liveness — pings keep the connection alive through proxies
	// that close idle sockets (Cloudflare ~100s) and detect dead clients.
	wsPingInterval = 30 * time.Second
	wsReadDeadline = 70 * time.Second

	// Bounds on terminal dimensions from clients (sanity / DoS).
	minDim = 1
	maxDim = 500

	// Max body size for JSON POSTs — bounds memory from malicious requests.
	maxJSONBody = 8 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// Same-origin in practice; auth enforced via token in query string.
		return true
	},
}

func clampDim(v uint16) uint16 {
	if v < minDim {
		return minDim
	}
	if v > maxDim {
		return maxDim
	}
	return v
}

type clientMsg struct {
	Type  string `json:"type"`
	Shell string `json:"shell,omitempty"`
	Name  string `json:"name,omitempty"`
	Cols  uint16 `json:"cols,omitempty"`
	Rows  uint16 `json:"rows,omitempty"`
}

type serverMsg struct {
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	Shell   string `json:"shell,omitempty"`
	Mode    string `json:"mode,omitempty"`    // present on share attaches
	Created bool   `json:"created,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func NewWSHandler(cfg *Config, mgr *SessionManager, shares *ShareManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Two auth paths:
		//   1) ?token=MAIN or X-Auth-Token header → full access (create/attach)
		//   2) ?share=SHARETOKEN → restricted; implicit attach to share's
		//      session, with mode enforced (viewer = read-only)
		shareToken := r.URL.Query().Get("share")
		if shareToken != "" {
			share, ok := shares.Resolve(shareToken)
			if !ok {
				http.Error(w, "invalid or expired share", http.StatusUnauthorized)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("ws upgrade: %v", err)
				return
			}
			handleShareWS(conn, share, mgr)
			return
		}
		if !authorize(cfg, r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}
		handleWS(conn, cfg, mgr)
	})
}

// handleShareWS attaches the WS to the share's session with mode
// enforcement. Viewers cannot send input, resize, or kill — those
// messages are silently dropped on the server side, so a malicious
// client can't escalate via a crafted JSON message.
func handleShareWS(conn *websocket.Conn, share *Share, mgr *SessionManager) {
	defer conn.Close()

	sess, ok := mgr.Get(share.Session)
	if !ok {
		sendErr(conn, "session no longer exists")
		return
	}
	att, backlog, err := sess.Attach()
	if err != nil {
		sendErr(conn, err.Error())
		return
	}
	defer sess.Detach(att)

	var writeMu sync.Mutex
	writeJSON := func(m serverMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(m)
	}
	writeBinary := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.BinaryMessage, b)
	}
	writeControl := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteControl(messageType, data, time.Now().Add(5*time.Second))
	}

	_ = conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
		return nil
	})
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		t := time.NewTicker(wsPingInterval)
		defer t.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-t.C:
				if err := writeControl(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	if err := writeJSON(serverMsg{
		Type:  "ready",
		Name:  sess.Name,
		Shell: sess.Shell,
		Mode:  string(share.Mode),
	}); err != nil {
		return
	}
	if len(backlog) > 0 {
		if err := writeBinary(backlog); err != nil {
			return
		}
	}

	go func() {
		defer conn.Close()
		for data := range att.Out {
			if err := writeBinary(data); err != nil {
				return
			}
		}
		_ = writeJSON(serverMsg{Type: "ended", Reason: "session ended"})
	}()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// Only writer mode forwards input. Viewer mode drops keystrokes.
		switch mt {
		case websocket.BinaryMessage:
			if share.Mode == ShareModeWriter {
				if _, err := sess.Write(data); err != nil {
					return
				}
			}
		case websocket.TextMessage:
			if looksLikeJSON(data) {
				var msg clientMsg
				if json.Unmarshal(data, &msg) == nil {
					switch msg.Type {
					case "resize":
						if share.Mode == ShareModeWriter {
							_ = sess.Resize(clampDim(msg.Cols), clampDim(msg.Rows))
						}
						continue
					case "detach", "close":
						return
					case "kill":
						// Never honored via share — kill requires the main token.
						continue
					}
				}
			}
			if share.Mode == ShareModeWriter {
				if _, err := sess.Write(data); err != nil {
					return
				}
			}
		}
	}
}

func handleWS(conn *websocket.Conn, cfg *Config, mgr *SessionManager) {
	defer conn.Close()

	// First message must be create or attach.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	mt, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	if mt != websocket.TextMessage {
		sendErr(conn, "expected text handshake message")
		return
	}
	var first clientMsg
	if err := json.Unmarshal(raw, &first); err != nil {
		sendErr(conn, "invalid handshake JSON")
		return
	}

	var (
		sess    *Session
		created bool
	)
	switch first.Type {
	case "create":
		if !cfg.IsShellAllowed(first.Shell) {
			sendErr(conn, "shell not allowed: "+first.Shell)
			return
		}
		s, err := mgr.Create(first.Name, first.Shell, first.Cols, first.Rows)
		if err != nil {
			sendErr(conn, err.Error())
			return
		}
		sess = s
		created = true
	case "attach":
		s, ok := mgr.Get(first.Name)
		if !ok {
			sendErr(conn, "no such session: "+first.Name)
			return
		}
		sess = s
	default:
		sendErr(conn, "first message must be {type:'create'|'attach'}")
		return
	}

	att, backlog, err := sess.Attach()
	if err != nil {
		sendErr(conn, err.Error())
		return
	}
	defer sess.Detach(att)

	// Apply initial size from the attaching client (last-writer-wins is fine).
	if first.Cols > 0 && first.Rows > 0 {
		_ = sess.Resize(clampDim(first.Cols), clampDim(first.Rows))
	}

	// Serialize writes to the WS — both goroutines may send concurrently.
	var writeMu sync.Mutex
	writeJSON := func(m serverMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(m)
	}
	writeBinary := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.BinaryMessage, b)
	}
	writeControl := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteControl(messageType, data, time.Now().Add(5*time.Second))
	}

	// Keepalive: server-initiated ping every wsPingInterval; client must
	// respond with pong, refreshing the read deadline.
	_ = conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
		return nil
	})
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		t := time.NewTicker(wsPingInterval)
		defer t.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-t.C:
				if err := writeControl(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	if err := writeJSON(serverMsg{
		Type:    "ready",
		Name:    sess.Name,
		Shell:   sess.Shell,
		Created: created,
	}); err != nil {
		return
	}
	if len(backlog) > 0 {
		if err := writeBinary(backlog); err != nil {
			return
		}
	}

	// PTY → WS pump
	go func() {
		defer conn.Close()
		for data := range att.Out {
			if err := writeBinary(data); err != nil {
				return
			}
		}
		// att.Out closed means the session ended (or we were dropped as slow).
		_ = writeJSON(serverMsg{Type: "ended", Reason: "session ended"})
	}()

	// WS → PTY pump
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			if _, err := sess.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			if looksLikeJSON(data) {
				var msg clientMsg
				if json.Unmarshal(data, &msg) == nil {
					switch msg.Type {
					case "resize":
						_ = sess.Resize(clampDim(msg.Cols), clampDim(msg.Rows))
						continue
					case "detach", "close":
						return
					case "kill":
						sess.Kill()
						return
					}
				}
			}
			if _, err := sess.Write(data); err != nil {
				return
			}
		}
	}
}

func looksLikeJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '{'
	}
	return false
}

func sendErr(conn *websocket.Conn, m string) {
	_ = conn.WriteJSON(serverMsg{Type: "error", Message: m})
}

func NewAuthHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !tokenEqual(body.Token, cfg.Token) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func NewUIHandler(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		f, err := uiFS.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for unknown paths.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
