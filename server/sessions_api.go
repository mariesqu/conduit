package server

import (
	"encoding/json"
	"net/http"
)

// RegisterSessionRoutes adds the session REST endpoints to mux.
//
//   GET    /api/sessions          → JSON list of SessionInfo
//   DELETE /api/sessions/{name}   → 204 / 404
//
// All routes require token auth.
func RegisterSessionRoutes(mux *http.ServeMux, cfg *Config, mgr *SessionManager) {
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		list := mgr.List()
		if list == nil {
			list = []SessionInfo{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("DELETE /api/sessions/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		if !mgr.Kill(name) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
