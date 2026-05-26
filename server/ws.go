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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// Same-origin in practice; auth enforced via token in query string.
		return true
	},
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
	Created bool   `json:"created,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func NewWSHandler(cfg *Config, mgr *SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		_ = sess.Resize(first.Cols, first.Rows)
	}

	// Serialize writes to the WS — both goroutines may send concurrently.
	var writeMu sync.Mutex
	writeJSON := func(m serverMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(m)
	}
	writeBinary := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, b)
	}

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
						_ = sess.Resize(msg.Cols, msg.Rows)
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
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Token == "" || body.Token != cfg.Token {
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
