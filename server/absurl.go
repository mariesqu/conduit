package server

import (
	"net/http"
	"strings"
)

// AbsoluteURL composes an absolute URL for the given path on the same
// origin as the request. When cfg.TrustProxyHeaders is false (default),
// it uses only the request's Host header and TLS state — anything a
// direct client could spoof gets used, but no header forgery escalates
// the truth value of the URL.
//
// When cfg.TrustProxyHeaders is true, X-Forwarded-Proto and
// X-Forwarded-Host (or X-Forwarded-Server) override the local view,
// which is what you want behind Cloudflare/Tailscale/nginx where the
// app sees an internal Host like "localhost:7180".
//
// path must start with "/".
func AbsoluteURL(cfg *Config, r *http.Request, path string) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	host := r.Host

	if cfg.TrustProxyHeaders {
		if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); p != "" {
			// Some proxies send a comma-separated chain; take the leftmost.
			if i := strings.IndexByte(p, ','); i >= 0 {
				p = strings.TrimSpace(p[:i])
			}
			proto = p
		}
		if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
			if i := strings.IndexByte(h, ','); i >= 0 {
				h = strings.TrimSpace(h[:i])
			}
			host = h
		}
	}
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return proto + "://" + host + path
}
