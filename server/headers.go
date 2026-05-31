package server

import (
	"net"
	"net/http"
	"strings"
)

// contentSecurityPolicy is intentionally strict. The UI is fully
// self-hosted (embedded via go:embed) and loads no third-party origins,
// so 'self' is sufficient for scripts, fonts, images, and the
// same-origin WebSocket. 'unsafe-inline' is required only for styles:
// xterm.js injects a <style> element at runtime and the app sets inline
// style properties, neither of which can carry a nonce. Scripts remain
// locked to 'self' — that's the part that actually backstops XSS.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"worker-src 'self'; " +
	"manifest-src 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'"

// SecurityHeaders wraps a handler and sets defensive response headers on
// every response. These are cheap, broadly-recommended hardening for an
// internet-exposed tool whose only credential can travel in a URL:
//
//   - Referrer-Policy: no-referrer  → never leak the token (or a share
//     token) via the Referer header to anything the page might touch.
//   - X-Content-Type-Options: nosniff → no MIME sniffing on downloads.
//   - X-Frame-Options / frame-ancestors → no clickjacking the terminal.
//   - Content-Security-Policy → confine scripts to 'self' as an XSS
//     backstop and limit exfiltration destinations.
//   - HSTS → honored only by browsers over HTTPS (ignored over plain
//     HTTP), so it is safe to send unconditionally.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the best-effort source IP for logging and rate
// limiting. X-Forwarded-For is honored ONLY when the operator has opted
// into TrustProxyHeaders (i.e. a trusted reverse proxy sits in front);
// otherwise any direct client could spoof it.
func clientIP(cfg *Config, r *http.Request) string {
	if cfg != nil && cfg.TrustProxyHeaders {
		if xff := firstChainValue(r.Header.Get("X-Forwarded-For")); xff != "" {
			return xff
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
