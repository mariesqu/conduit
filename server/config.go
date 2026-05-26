package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	Bind          string   `json:"bind"`
	Port          int      `json:"port"`
	Token         string   `json:"token"`
	AllowedShells []string `json:"allowed_shells"`
	MaxSessions   int      `json:"max_sessions"`

	// FilesRoot is the only directory the file API will read or write
	// under. Empty → ~/Conduit-Files. Path traversal is blocked.
	FilesRoot string `json:"files_root"`

	// MaxUploadMB caps a single uploaded file's size. 0 → 50.
	MaxUploadMB int `json:"max_upload_mb"`

	// Tunnel controls the optional public URL bootstrap.
	//   "off"          → no tunnel attempt
	//   "auto"         → detect cloudflared in PATH, spawn a quick tunnel if found
	//   "cloudflared"  → require cloudflared, fail loudly if not present
	// Defaults to "off".
	Tunnel string `json:"tunnel"`

	// Presets are named bundles of sessions launched together.
	Presets []Preset `json:"presets,omitempty"`

	// TrustProxyHeaders enables honoring X-Forwarded-Proto and
	// X-Forwarded-Host when building absolute URLs (currently used for
	// share-link responses). Off by default — direct-internet exposure
	// would let any client spoof the absolute URL via these headers.
	// Turn ON only when a trusted reverse proxy (Cloudflare Tunnel,
	// Tailscale serve, nginx, Caddy) sets them.
	TrustProxyHeaders bool `json:"trust_proxy_headers"`
}

// Preset is a named collection of sessions to launch in one click.
type Preset struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Sessions    []PresetSession `json:"sessions"`
}

// PresetSession is one session within a preset. Command is optional —
// when non-empty it's written to the new shell as initial input.
type PresetSession struct {
	Name    string `json:"name"`
	Shell   string `json:"shell"`
	Command string `json:"command,omitempty"`
	Dir     string `json:"dir,omitempty"` // optional cd before command
}

// DefaultMaxSessions is the upper bound on concurrent live sessions per
// server. Bounds the blast radius of an authenticated DoS.
const DefaultMaxSessions = 64

// DefaultMaxUploadMB caps a single uploaded file's size.
const DefaultMaxUploadMB = 50

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Bind:        "127.0.0.1",
		Port:        7180,
		MaxSessions: DefaultMaxSessions,
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg.Token = generateToken()
		if err := saveConfig(path, cfg); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	changed := false
	if cfg.Token == "" {
		cfg.Token = generateToken()
		changed = true
	}
	if cfg.Port == 0 {
		cfg.Port = 7180
		changed = true
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
		changed = true
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = DefaultMaxSessions
		changed = true
	}
	if cfg.MaxUploadMB <= 0 {
		cfg.MaxUploadMB = DefaultMaxUploadMB
		changed = true
	}
	if changed {
		if err := saveConfig(path, cfg); err != nil {
			return nil, fmt.Errorf("update config: %w", err)
		}
	}
	return cfg, nil
}

func saveConfig(path string, cfg *Config) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (c *Config) IsShellAllowed(name string) bool {
	if len(c.AllowedShells) == 0 {
		return true
	}
	for _, s := range c.AllowedShells {
		if s == name {
			return true
		}
	}
	return false
}
