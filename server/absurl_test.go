package server

import (
	"crypto/tls"
	"net/http/httptest"
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
