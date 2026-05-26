package server

import (
	"net/http"
	"regexp"
	"strings"
)

// AbsoluteURL composes an absolute URL for the given path on the same
// origin as the request. When cfg.TrustProxyHeaders is false (default),
// it uses only the request's Host header and TLS state — anything a
// direct client could spoof gets used, but no header forgery escalates
// the truth value of the URL.
//
// When cfg.TrustProxyHeaders is true, X-Forwarded-Proto and
// X-Forwarded-Host (when valid) override the local view, which is what
// you want behind Cloudflare/Tailscale/nginx where the app sees an
// internal Host like "localhost:7180".
//
// Forwarded values are validated even when trusted:
//   - Proto must be exactly "http" or "https" (case-insensitive).
//   - Host must match a permissive but bounded charset.
//
// Anything else is silently ignored — we fall back to the request's
// own Host/TLS values rather than splice attacker-controlled text into
// the URL we return.
//
// path must start with "/"; missing slash is added.
func AbsoluteURL(cfg *Config, r *http.Request, path string) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	host := strings.TrimSpace(r.Host)

	if cfg.TrustProxyHeaders {
		if p := firstChainValue(r.Header.Get("X-Forwarded-Proto")); p != "" && isValidProto(p) {
			proto = strings.ToLower(p)
		}
		if h := firstChainValue(r.Header.Get("X-Forwarded-Host")); h != "" && isValidHost(h) {
			host = h
		}
	}

	// Final guard: an empty Host header would yield "http://path",
	// which is malformed. Use a clearly-self-referential fallback so
	// callers can detect this in tests/logs rather than emit a broken URL.
	if host == "" {
		host = "localhost"
	}
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return proto + "://" + host + path
}

// firstChainValue extracts the leftmost value from a comma-separated
// forwarded-header chain and trims whitespace. Returns "" for empty.
func firstChainValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func isValidProto(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	return p == "http" || p == "https"
}

// validHostRe accepts hostnames, IPv4, and bracketed IPv6, each with an
// optional :port. Permissive on charset to support internationalized
// domains via punycode (xn--…) and tunnel hostnames, but rejects
// whitespace, control chars, slashes, and credentials (user:pass@host).
var validHostRe = regexp.MustCompile(`^(?:\[[0-9a-fA-F:.]+\]|[a-zA-Z0-9._\-]+)(?::[0-9]{1,5})?$`)

func isValidHost(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > 253+6 { // hostname max + ":65535"
		return false
	}
	return validHostRe.MatchString(h)
}
