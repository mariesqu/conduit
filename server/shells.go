package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
)

type Shell struct {
	Name string   `json:"name"`
	Path string   `json:"path"`
	Args []string `json:"args,omitempty"`
}

type shellCandidate struct {
	name string
	exe  string
	args []string
}

func shellCandidates() []shellCandidate {
	if runtime.GOOS == "windows" {
		return []shellCandidate{
			{"powershell", "pwsh.exe", nil},
			{"powershell5", "powershell.exe", nil},
			{"cmd", "cmd.exe", nil},
			{"wsl", "wsl.exe", nil},
			{"git-bash", "bash.exe", nil},
		}
	}
	return []shellCandidate{
		{"bash", "bash", nil},
		{"zsh", "zsh", nil},
		{"sh", "sh", nil},
	}
}

func DetectShells() []Shell {
	var out []Shell
	for _, c := range shellCandidates() {
		if p, err := exec.LookPath(c.exe); err == nil {
			out = append(out, Shell{Name: c.name, Path: p, Args: c.args})
		}
	}
	return out
}

func FindShellByName(name string) *Shell {
	for _, s := range DetectShells() {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

func NewShellsHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		all := DetectShells()
		filtered := make([]Shell, 0, len(all))
		for _, s := range all {
			if cfg.IsShellAllowed(s.Name) {
				filtered = append(filtered, s)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(filtered)
	}
}

func authorize(cfg *Config, r *http.Request) bool {
	token := r.Header.Get("X-Auth-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return token != "" && token == cfg.Token
}
