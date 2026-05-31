package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mariesqu/conduit/server"
)

//go:embed all:ui/dist
var embeddedUI embed.FS

// version is injected at build time via -ldflags "-X main.version=v0.1.0".
// Defaults to "dev" for local non-release builds.
var version = "dev"

func main() {
	var (
		configPath  string
		publicURL   string
		noQR        bool
		showVersion bool
	)
	flag.StringVar(&configPath, "config", "conduit.config.json", "path to config file")
	flag.StringVar(&publicURL, "public-url", "", "override the URL printed on startup (e.g. https://term.example.com — useful behind a tunnel)")
	flag.BoolVar(&noQR, "no-qr", false, "suppress the startup QR code")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println("conduit", version)
		return
	}

	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	uiFS, err := fs.Sub(embeddedUI, "ui/dist")
	if err != nil {
		log.Fatalf("ui sub-fs: %v", err)
	}

	mgr := server.NewSessionManager(cfg.MaxSessions)
	shares := server.NewShareManager()
	defer shares.Shutdown()
	mgr.OnSessionRemoved = shares.RevokeForSession

	tickets := server.NewTicketManager()
	defer tickets.Shutdown()

	// Tight per-IP limiter for the UNauthenticated guess surface
	// (/api/auth): ~1 req/s sustained, burst 10. The high-entropy token
	// makes brute force pointless anyway; this just caps flood/log noise.
	authLimiter := server.NewRateLimiter(1, 10)
	defer authLimiter.Shutdown()

	// Generous limiter for /api/ticket. It already requires a valid token,
	// and the UI fetches one per WebSocket connect and per download — so a
	// user restoring many tabs or launching a multi-session preset must
	// not be starved. This only bounds pathological abuse.
	ticketLimiter := server.NewRateLimiter(20, 60)
	defer ticketLimiter.Shutdown()

	files, err := server.NewFileService(cfg.FilesRoot, cfg.MaxUploadMB)
	if err != nil {
		log.Fatalf("file service: %v", err)
	}
	log.Printf("files root: %s", files.Root)

	mux := http.NewServeMux()
	mux.Handle("/ws", server.NewWSHandler(cfg, mgr, shares, tickets))
	mux.HandleFunc("/api/shells", server.NewShellsHandler(cfg))
	mux.Handle("/api/auth", server.RateLimit(cfg, authLimiter, server.NewAuthHandler(cfg)))
	mux.Handle("/api/ticket", server.RateLimit(cfg, ticketLimiter, server.NewTicketHandler(cfg, tickets)))
	mux.HandleFunc("/api/token/rotate", server.NewTokenRotateHandler(cfg))
	server.RegisterSessionRoutes(mux, cfg, mgr)
	server.RegisterShareRoutes(mux, cfg, mgr, shares)
	server.RegisterFileRoutes(mux, cfg, files, tickets)
	server.RegisterPresetRoutes(mux, cfg, mgr)
	mux.Handle("/", server.NewUIHandler(uiFS))

	// Refuse to expose the server on a non-loopback address over plain
	// HTTP unless TLS is configured or the operator explicitly opts in.
	// Plain HTTP on a reachable interface serves the auth token and every
	// keystroke in cleartext.
	if !isLoopbackBind(cfg.Bind) && !cfg.TLSEnabled() && !cfg.AllowInsecure {
		log.Fatalf("refusing to bind to non-loopback address %q over plain HTTP — "+
			"the auth token and all terminal traffic would travel in cleartext.\n"+
			"  • set \"tls_cert\"/\"tls_key\" in the config for direct HTTPS, or\n"+
			"  • bind to 127.0.0.1 and expose via a tunnel (cloudflared/tailscale), or\n"+
			"  • set \"allow_insecure\": true only if the network is already encrypted.", cfg.Bind)
	}

	scheme := "http"
	if cfg.TLSEnabled() {
		scheme = "https"
	}
	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.SecurityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("conduit listening on %s://%s", scheme, addr)

	go func() {
		var serveErr error
		if cfg.TLSEnabled() {
			serveErr = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatalf("listen: %v", serveErr)
		}
	}()

	// Attempt to bring up a tunnel BEFORE printing the URL/QR so the URL
	// is the public one. publicURL flag always wins over auto-tunnel.
	var tunnel *server.Tunnel
	if publicURL == "" {
		tunnel = maybeStartTunnel(cfg.Tunnel, cfg.Port)
		if tunnel != nil {
			publicURL = tunnel.URL
			log.Printf("tunnel (%s) active: %s", tunnel.Provider, tunnel.URL)
			defer tunnel.Stop()
		}
	}

	accessURL := buildAccessURL(publicURL, cfg)
	log.Printf("access URL: %s", accessURL)
	log.Printf("auth token: %s", cfg.CurrentToken())

	if !noQR {
		qr, err := server.RenderQRToTerminal(accessURL)
		if err == nil {
			fmt.Println()
			fmt.Println("Scan from your phone to sign in:")
			fmt.Println()
			fmt.Print(qr)
			fmt.Println()
		} else {
			log.Printf("could not render QR: %v", err)
		}
	}

	if cfg.Bind == "127.0.0.1" && publicURL == "" {
		log.Println("bound to localhost — expose with: cloudflared tunnel --url http://localhost:" + fmt.Sprint(cfg.Port))
		if ts := server.DetectTailscale(); ts != "" {
			log.Println("tailscale detected — share with: tailscale funnel --bg " + fmt.Sprint(cfg.Port))
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	mgr.Shutdown()
	log.Println("bye")
}

// maybeStartTunnel honors cfg.Tunnel and tries to spawn a public
// tunnel. Returns nil if disabled, unavailable, or failed.
//
//   "off"          → nil (no attempt)
//   "auto"         → best-effort cloudflared, fall back to nil silently
//   "cloudflared"  → require cloudflared, log loudly on failure
//
// Any other value (or empty) means "off".
func maybeStartTunnel(mode string, port int) *server.Tunnel {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "auto":
		t, err := server.StartCloudflaredQuick(port, 20*time.Second)
		if err != nil {
			if !errors.Is(err, server.ErrNotFound) {
				log.Printf("tunnel auto: %v", err)
			}
			return nil
		}
		return t
	case "cloudflared":
		t, err := server.StartCloudflaredQuick(port, 20*time.Second)
		if err != nil {
			log.Printf("tunnel cloudflared: %v (install from https://github.com/cloudflare/cloudflared)", err)
			return nil
		}
		return t
	default:
		return nil
	}
}

// isLoopbackBind reports whether the configured bind address is a
// loopback interface (or the empty default, which LoadConfig turns into
// 127.0.0.1). Used to gate the cleartext-exposure guard.
func isLoopbackBind(bind string) bool {
	switch strings.ToLower(strings.TrimSpace(bind)) {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	}
	if ip := net.ParseIP(strings.TrimSpace(bind)); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// buildAccessURL composes the URL to print on startup, with the auth
// token embedded so a phone scanning the QR can sign in with one tap.
func buildAccessURL(publicURL string, cfg *server.Config) string {
	base := publicURL
	if base == "" {
		scheme := "http"
		if cfg.TLSEnabled() {
			scheme = "https"
		}
		host := server.HostForURL(cfg.Bind)
		base = fmt.Sprintf("%s://%s:%d", scheme, host, cfg.Port)
	}
	base = strings.TrimRight(base, "/")
	q := url.Values{}
	q.Set("token", cfg.CurrentToken())
	return base + "/?" + q.Encode()
}
