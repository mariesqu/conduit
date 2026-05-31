package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestTicketManager_IssueAndValidate(t *testing.T) {
	m := NewTicketManager()
	defer m.Shutdown()

	tok := m.Issue()
	if tok == "" {
		t.Fatal("Issue returned empty ticket")
	}
	if !m.Valid(tok) {
		t.Error("freshly issued ticket should be valid")
	}
	if m.Valid("") {
		t.Error("empty ticket must be rejected")
	}
	if m.Valid("not-a-real-ticket") {
		t.Error("unknown ticket must be rejected")
	}
}

func TestTicketManager_Expiry(t *testing.T) {
	m := &TicketManager{
		tickets: map[string]time.Time{"stale": time.Now().Add(-time.Second)},
		stop:    make(chan struct{}),
	}
	if m.Valid("stale") {
		t.Error("expired ticket must be rejected")
	}
	if _, ok := m.tickets["stale"]; ok {
		t.Error("expired ticket should be evicted on lookup")
	}
}

func TestRateLimiter_BurstThenBlock(t *testing.T) {
	rl := NewRateLimiter(1, 3)
	defer rl.Shutdown()

	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d within burst should be allowed", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Error("request beyond burst should be blocked")
	}
	// A different key has its own bucket.
	if !rl.Allow("5.6.7.8") {
		t.Error("a different IP should not be affected by another's limit")
	}
}

func TestOriginAllowed(t *testing.T) {
	cfg := &Config{AllowedOrigins: []string{"https://term.example.com"}}

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin (non-browser)", "", "conduit.local:7180", true},
		{"same origin", "http://conduit.local:7180", "conduit.local:7180", true},
		{"cross origin", "https://evil.example", "conduit.local:7180", false},
		{"allowlisted origin", "https://term.example.com", "localhost:7180", true},
		{"garbage origin", "::::not a url", "conduit.local:7180", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tc.host+"/ws", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := originAllowed(cfg, r); got != tc.want {
				t.Errorf("originAllowed(origin=%q, host=%q) = %v, want %v", tc.origin, tc.host, got, tc.want)
			}
		})
	}
}

func TestSanitizeInline(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"keep\ttab", "keep\ttab"},
		{"drop\r\nnewline", "dropnewline"},
		{"null\x00byte", "nullbyte"},
		{"cmd\nrm -rf /", "cmdrm -rf /"},
	}
	for _, tc := range tests {
		if got := sanitizeInline(tc.in); got != tc.want {
			t.Errorf("sanitizeInline(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRotateToken_ChangesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/conduit.config.json"
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	old := cfg.CurrentToken()
	if old == "" {
		t.Fatal("expected a generated token")
	}

	newTok, err := cfg.RotateToken()
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if newTok == old {
		t.Error("rotated token should differ from the old one")
	}
	if cfg.CurrentToken() != newTok {
		t.Error("CurrentToken should reflect the rotated value")
	}

	// Reload from disk: the new token must have been persisted.
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.CurrentToken() != newTok {
		t.Error("rotated token was not persisted to disk")
	}
}
