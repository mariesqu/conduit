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
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Bind: "127.0.0.1",
		Port: 7180,
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
