package server

import (
	"crypto/tls"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAbsoluteURL_TrustOff_IgnoresForwardedHeaders(t *testing.T) {
	cfg := &Config{TrustProxyHeaders: false}
	r := httptest.NewRequest("POST", "http://127.0.0.1:7180/api/foo", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "evil.example.com")
	got := AbsoluteURL(cfg, r, "/?share=abc")
	want := "http://127.0.0.1:7180/?share=abc"
	if got != want {
		t.Fatalf("got %q, want %q (spoofed headers should be ignored)", got, want)
	}
}

func TestAbsoluteURL_TrustOn_HonorsForwardedHeaders(t *testing.T) {
	cfg := &Config{TrustProxyHeaders: true}
	r := httptest.NewRequest("POST", "http://127.0.0.1:7180/api/foo", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "term.example.com")
	got := AbsoluteURL(cfg, r, "/?share=abc")
	want := "https://term.example.com/?share=abc"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAbsoluteURL_TLSImpliesHTTPS(t *testing.T) {
	cfg := &Config{TrustProxyHeaders: false}
	r := httptest.NewRequest("GET", "https://example.com/x", nil)
	// httptest doesn't set TLS automatically for https URL; fake it.
	r.TLS = &tls.ConnectionState{}
	got := AbsoluteURL(cfg, r, "/?share=abc")
	want := "https://example.com/?share=abc"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAbsoluteURL_HandlesCommaChainedForwardedHeaders(t *testing.T) {
	cfg := &Config{TrustProxyHeaders: true}
	r := httptest.NewRequest("GET", "http://localhost/x", nil)
	r.Header.Set("X-Forwarded-Proto", "https, http")
	r.Header.Set("X-Forwarded-Host", "front.example.com, internal")
	got := AbsoluteURL(cfg, r, "/x")
	want := "https://front.example.com/x"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAbsoluteURL_LeadingSlashEnforced(t *testing.T) {
	cfg := &Config{}
	r := httptest.NewRequest("GET", "http://localhost/x", nil)
	if got := AbsoluteURL(cfg, r, "no-slash"); got != "http://localhost/no-slash" {
		t.Fatalf("got %q", got)
	}
	if got := AbsoluteURL(cfg, r, ""); got != "http://localhost/" {
		t.Fatalf("empty path: got %q", got)
	}
}

func TestAbsoluteURL_RejectsInvalidForwardedValues(t *testing.T) {
	cfg := &Config{TrustProxyHeaders: true}
	tests := []struct {
		name           string
		fwdProto       string
		fwdHost        string
		wantContains   string
		wantNotContain string
	}{
		{
			name:           "javascript scheme rejected",
			fwdProto:       "javascript",
			fwdHost:        "evil.example.com",
			wantContains:   "http://", // falls back to request's scheme
			wantNotContain: "javascript:",
		},
		{
			name:           "host with credentials rejected",
			fwdProto:       "https",
			fwdHost:        "user:pass@evil.com",
			wantContains:   "127.0.0.1", // falls back to request's host
			wantNotContain: "@evil.com",
		},
		{
			name:           "host with path rejected",
			fwdProto:       "https",
			fwdHost:        "evil.com/attacker-path",
			wantContains:   "127.0.0.1",
			wantNotContain: "evil.com",
		},
		{
			name:           "host with whitespace rejected",
			fwdProto:       "https",
			fwdHost:        "evil .com",
			wantContains:   "127.0.0.1",
			wantNotContain: "evil",
		},
		{
			name:           "host with control char rejected",
			fwdProto:       "https",
			fwdHost:        "evil.com\r\nX-Injected: yes",
			wantContains:   "127.0.0.1",
			wantNotContain: "Injected",
		},
		{
			name:         "valid host:port accepted",
			fwdProto:     "https",
			fwdHost:      "term.example.com:8443",
			wantContains: "https://term.example.com:8443/",
		},
		{
			name:         "valid IPv6 bracketed accepted",
			fwdProto:     "https",
			fwdHost:      "[2001:db8::1]:443",
			wantContains: "https://[2001:db8::1]:443/",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://127.0.0.1:7180/api/foo", nil)
			r.Header.Set("X-Forwarded-Proto", tc.fwdProto)
			r.Header.Set("X-Forwarded-Host", tc.fwdHost)
			got := AbsoluteURL(cfg, r, "/x")
			if !strings.Contains(got, tc.wantContains) {
				t.Errorf("got %q, want it to contain %q", got, tc.wantContains)
			}
			if tc.wantNotContain != "" && strings.Contains(got, tc.wantNotContain) {
				t.Errorf("got %q, must NOT contain %q", got, tc.wantNotContain)
			}
		})
	}
}

func TestAbsoluteURL_EmptyHostFallback(t *testing.T) {
	cfg := &Config{}
	r := httptest.NewRequest("GET", "http://localhost/x", nil)
	r.Host = "" // some clients/proxies can blank it
	got := AbsoluteURL(cfg, r, "/y")
	if got != "http://localhost/y" {
		t.Fatalf("got %q, want http://localhost/y", got)
	}
}
