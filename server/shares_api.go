package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RegisterShareRoutes adds the share REST endpoints to mux.
//
//   POST   /api/sessions/{name}/share   body: {mode, ttl_seconds}
//                                       → {token, url, expires_at}
//   GET    /api/shares                  → []Share
//   DELETE /api/shares/{token}          → 204 / 404
//
// All routes require the main auth token.
func RegisterShareRoutes(mux *http.ServeMux, cfg *Config, mgr *SessionManager, shares *ShareManager) {
	mux.HandleFunc("POST /api/sessions/{name}/share", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "missing session name", http.StatusBadRequest)
			return
		}
		if _, ok := mgr.Get(name); !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
		var body struct {
			Mode       string `json:"mode"`
			TTLSeconds int    `json:"ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.TTLSeconds <= 0 {
			body.TTLSeconds = 3600 // 1h default
		}
		mode := ShareMode(strings.ToLower(strings.TrimSpace(body.Mode)))
		if mode == "" {
			mode = ShareModeViewer
		}
		share, err := shares.Create(name, mode, time.Duration(body.TTLSeconds)*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		path := fmt.Sprintf("/?share=%s", share.Token)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":        share.Token,
			"url":          path,
			"absolute_url": AbsoluteURL(cfg, r, path),
			"session":      share.Session,
			"mode":         share.Mode,
			"created_at":   share.CreatedAt,
			"expires_at":   share.ExpiresAt,
		})
	})

	mux.HandleFunc("GET /api/shares", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		list := shares.List()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("DELETE /api/shares/{token}", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token := r.PathValue("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		if !shares.Revoke(token) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
