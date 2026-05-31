package server

import (
	"encoding/json"
	"net/http"
	"strings"
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

			// In locked mode we ONLY create the session — we never type
			// anything into the shell. Operators who want hardened
			// behavior can ship the binary with presets_locked=true and
			// presets that just organize sessions by name.
			if cfg.PresetsLocked {
				continue
			}

			// Build the initial command. Use cd if Dir is set, then the
			// user-supplied command. Use ; (PowerShell/Unix) or && for
			// chains; we don't know the shell semantics in detail, so
			// keep it simple — separate lines.
			// Strip control characters (notably CR/LF) so a preset value
			// can't smuggle extra command lines into the shell beyond the
			// single line the operator intended.
			dir := sanitizeInline(ps.Dir)
			cmd := sanitizeInline(ps.Command)
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
			}(sess, dir, cmd)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"preset":   preset.Name,
			"launched": results,
		})
	})
}

// sanitizeInline removes control characters (including CR and LF) from a
// preset-supplied value. Presets come from the operator's own config
// file, but stripping line breaks here is cheap defense-in-depth that
// keeps a single configured value from expanding into multiple shell
// command lines — and it pairs with presets_locked for contexts where
// the config might be written by a less-trusted party.
func sanitizeInline(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || (r < 0x20 && r != '\t') || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// quoteForShell wraps s in double quotes (escaping any embedded double
// quote by doubling it, which both PowerShell and POSIX shells accept
// inside a double-quoted string) when s contains whitespace or a
// shell-special character. Good enough for the cd argument.
func quoteForShell(s string) string {
	needsQuote := false
	for _, c := range s {
		switch c {
		case ' ', '\t', '(', ')', '&', ';', '|', '<', '>', '"', '`', '$', '\'':
			needsQuote = true
		}
	}
	if !needsQuote {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
