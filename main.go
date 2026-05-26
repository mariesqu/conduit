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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/samyhajal/conduit/server"
)

//go:embed all:ui/dist
var embeddedUI embed.FS

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "conduit.config.json", "path to config file")
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
	mux := http.NewServeMux()
	mux.Handle("/ws", server.NewWSHandler(cfg, mgr))
	mux.HandleFunc("/api/shells", server.NewShellsHandler(cfg))
	mux.HandleFunc("/api/auth", server.NewAuthHandler(cfg))
	mux.Handle("/api/sessions", server.NewSessionsHandler(cfg, mgr))
	mux.Handle("/api/sessions/", server.NewSessionsHandler(cfg, mgr))
	mux.Handle("/", server.NewUIHandler(uiFS))

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("conduit listening on http://%s", addr)
	log.Printf("auth token: %s", cfg.Token)
	if cfg.Bind == "127.0.0.1" {
		log.Println("bound to localhost — expose with: cloudflared tunnel --url http://localhost:" + fmt.Sprint(cfg.Port))
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

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
