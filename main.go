package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/samyhajal/conduit/server"
)

//go:embed all:ui/dist
var embeddedUI embed.FS

func main() {
	var (
		configPath string
		publicURL  string
		noQR       bool
	)
	flag.StringVar(&configPath, "config", "conduit.config.json", "path to config file")
	flag.StringVar(&publicURL, "public-url", "", "override the URL printed on startup (e.g. https://term.example.com — useful behind a tunnel)")
	flag.BoolVar(&noQR, "no-qr", false, "suppress the startup QR code")
	flag.Parse()

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

	files, err := server.NewFileService(cfg.FilesRoot, cfg.MaxUploadMB)
	if err != nil {
		log.Fatalf("file service: %v", err)
	}
	log.Printf("files root: %s", files.Root)

	mux := http.NewServeMux()
	mux.Handle("/ws", server.NewWSHandler(cfg, mgr, shares))
	mux.HandleFunc("/api/shells", server.NewShellsHandler(cfg))
	mux.HandleFunc("/api/auth", server.NewAuthHandler(cfg))
	server.RegisterSessionRoutes(mux, cfg, mgr)
	server.RegisterShareRoutes(mux, cfg, mgr, shares)
	server.RegisterFileRoutes(mux, cfg, files)
	mux.Handle("/", server.NewUIHandler(uiFS))

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("conduit listening on http://%s", addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
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
	log.Printf("auth token: %s", cfg.Token)

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

// buildAccessURL composes the URL to print on startup, with the auth
// token embedded so a phone scanning the QR can sign in with one tap.
func buildAccessURL(publicURL string, cfg *server.Config) string {
	base := publicURL
	if base == "" {
		host := server.HostForURL(cfg.Bind)
		base = fmt.Sprintf("http://%s:%d", host, cfg.Port)
	}
	base = strings.TrimRight(base, "/")
	q := url.Values{}
	q.Set("token", cfg.Token)
	return base + "/?" + q.Encode()
}
