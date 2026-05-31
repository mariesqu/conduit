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
	"sync"
)

type Config struct {
	Bind          string   `json:"bind"`
	Port          int      `json:"port"`
	Token         string   `json:"token"`
	AllowedShells []string `json:"allowed_shells"`
	MaxSessions   int      `json:"max_sessions"`

	// TLSCert and TLSKey, when both set, make the server speak HTTPS
	// directly (ListenAndServeTLS). Leave empty when TLS is terminated
	// upstream by a tunnel or reverse proxy.
	TLSCert string `json:"tls_cert"`
	TLSKey  string `json:"tls_key"`

	// AllowInsecure permits binding to a non-loopback address over plain
	// HTTP. OFF by default: doing so serves the auth token and every
	// keystroke in cleartext on the network. Only set this if you fully
	// understand the exposure (e.g. an already-encrypted overlay network).
	AllowInsecure bool `json:"allow_insecure"`

	// AllowedOrigins is an optional allowlist of extra Origin hosts the
	// WebSocket upgrade will accept in addition to same-origin. Same-origin
	// is always allowed; this is for split-origin deployments only.
	AllowedOrigins []string `json:"allowed_origins,omitempty"`

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

	// PresetsLocked, when true, makes the preset launch endpoint create
	// the named sessions but REFUSE to write any 'command' or 'dir' as
	// auto-typed input. Useful for hardened deployments where the
	// operator wants presets that organize sessions but never inject
	// shell input — defense-in-depth for multi-user contexts where the
	// config file itself might be writable by a less-trusted party.
	PresetsLocked bool `json:"presets_locked"`

	// TrustProxyHeaders enables honoring X-Forwarded-Proto and
	// X-Forwarded-Host when building absolute URLs (currently used for
	// share-link responses). Off by default — direct-internet exposure
	// would let any client spoof the absolute URL via these headers.
	// Turn ON only when a trusted reverse proxy (Cloudflare Tunnel,
	// Tailscale serve, nginx, Caddy) sets them.
	TrustProxyHeaders bool `json:"trust_proxy_headers"`

	// mu guards Token against the data race between request handlers
	// reading it (auth) and a rotation request mutating it. path is the
	// source config file, retained so RotateToken can persist the new
	// value. Both are unexported and therefore ignored by encoding/json.
	mu   sync.RWMutex
	path string
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
	cfg.path = path

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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// 0o600 is meaningful on Unix; on Windows we need to set the DACL
	// explicitly. Best-effort — log and continue if it fails (the token
	// is still high-entropy and the path is operator-controlled).
	if err := restrictToCurrentUser(path); err != nil {
		// Loud warning on stderr — on Windows this is when the token
		// file may still be world-readable via the parent directory's
		// inherited ACL. Operators can either move the config under a
		// pre-locked directory, or accept the residual risk.
		_, _ = fmt.Fprintf(os.Stderr,
			"WARNING: failed to tighten ACL on %s: %v\n"+
				"         The auth token in this file may be readable by other users\n"+
				"         on this machine. Place the config under a directory you've\n"+
				"         already restricted via icacls or NTFS permissions.\n",
			path, err,
		)
	}
	return nil
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// CurrentToken returns the active auth token under a read lock so it is
// safe to call concurrently with RotateToken.
func (c *Config) CurrentToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Token
}

// RotateToken generates a fresh auth token, persists it to the config
// file, and returns it. All clients holding the previous token are
// effectively logged out — this is the revoke path for a leaked token.
func (c *Config) RotateToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Token = generateToken()
	if c.path != "" {
		if err := saveConfig(c.path, c); err != nil {
			return "", err
		}
	}
	return c.Token, nil
}

// TLSEnabled reports whether a cert/key pair is configured for direct
// HTTPS serving.
func (c *Config) TLSEnabled() bool {
	return c.TLSCert != "" && c.TLSKey != ""
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
