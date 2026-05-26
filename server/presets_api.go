package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// RegisterPresetRoutes adds preset endpoints to mux. All require token auth.
//
//   GET  /api/presets               → []Preset
//   POST /api/presets/{name}/launch → {launched:[...]}
//
// On launch:
//   • Sessions that already exist are kept as-is (Command is NOT replayed).
//   • Sessions that don't exist are created. If Command is non-empty it's
//     written to the new shell as initial input (after a short settle for
//     the shell to print its prompt).
//   • Failures on individual sessions are reported per-entry; the launch
//     attempts the remainder rather than aborting the whole bundle.
func RegisterPresetRoutes(mux *http.ServeMux, cfg *Config, mgr *SessionManager) {
	mux.HandleFunc("GET /api/presets", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		presets := cfg.Presets
		if presets == nil {
			presets = []Preset{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(presets)
	})

	mux.HandleFunc("POST /api/presets/{name}/launch", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		name := r.PathValue("name")
		var preset *Preset
		for i := range cfg.Presets {
			if cfg.Presets[i].Name == name {
				preset = &cfg.Presets[i]
				break
			}
		}
		if preset == nil {
			http.Error(w, "preset not found", http.StatusNotFound)
			return
		}

		type result struct {
			Session string `json:"session"`
			Status  string `json:"status"` // created | attached | error
			Error   string `json:"error,omitempty"`
		}
		results := make([]result, 0, len(preset.Sessions))

		for _, ps := range preset.Sessions {
			r := result{Session: ps.Name}
			if existing, ok := mgr.Get(ps.Name); ok && existing.Info().Alive {
				r.Status = "attached"
				results = append(results, r)
				continue
			}
			sess, err := mgr.Create(ps.Name, ps.Shell, 0, 0)
			if err != nil {
				r.Status = "error"
				r.Error = err.Error()
				results = append(results, r)
				continue
			}
			r.Status = "created"
			results = append(results, r)

			// Build the initial command. Use cd if Dir is set, then the
			// user-supplied command. Use ; (PowerShell/Unix) or && for
			// chains; we don't know the shell semantics in detail, so
			// keep it simple — separate lines.
			go func(s *Session, dir, cmd string) {
				// Give the shell a moment to print its prompt before injecting
				// input so the prompt doesn't visually overlap the command.
				time.Sleep(400 * time.Millisecond)
				if dir != "" {
					_, _ = s.Write([]byte("cd " + quoteForShell(dir) + "\r\n"))
					time.Sleep(80 * time.Millisecond)
				}
				if cmd != "" {
					_, _ = s.Write([]byte(cmd + "\r\n"))
				}
			}(sess, ps.Dir, ps.Command)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"preset":   preset.Name,
			"launched": results,
		})
	})
}

// quoteForShell wraps s in double quotes if it contains whitespace or
// shell-special characters. Good enough for the cd argument on both
// PowerShell and POSIX shells.
func quoteForShell(s string) string {
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '(' || c == ')' || c == '&' || c == ';' || c == '|' || c == '<' || c == '>' {
			return `"` + s + `"`
		}
	}
	return s
}
