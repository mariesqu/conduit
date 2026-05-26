package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Tunnel represents an externally-spawned reverse tunnel (currently
// just cloudflared quick tunnels).
type Tunnel struct {
	Provider string // "cloudflared"
	URL      string // public URL, e.g. https://foo-bar-baz.trycloudflare.com
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// StartCloudflaredQuick attempts to spawn `cloudflared tunnel --url
// http://127.0.0.1:PORT` and blocks until cloudflared prints a
// trycloudflare.com URL (or the timeout elapses).
//
// Returns ErrNotFound if cloudflared is not in PATH.
func StartCloudflaredQuick(localPort int, timeout time.Duration) (*Tunnel, error) {
	binPath, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, ErrNotFound
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath,
		"tunnel", "--no-autoupdate",
		"--url", fmt.Sprintf("http://127.0.0.1:%d", localPort),
	)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}

	t := &Tunnel{
		Provider: "cloudflared",
		cmd:      cmd,
		cancel:   cancel,
	}

	// Scan both streams for the URL. cloudflared usually writes it to
	// stderr in a boxed banner, but newer versions sometimes use stdout.
	urlCh := make(chan string, 1)
	t.wg.Add(2)
	go t.scanForURL(stdout, urlCh)
	go t.scanForURL(stderr, urlCh)

	deadline := time.After(timeout)
	select {
	case u := <-urlCh:
		t.URL = u
		return t, nil
	case <-deadline:
		t.Stop()
		return nil, fmt.Errorf("cloudflared timed out after %s without printing a URL", timeout)
	}
}

// ErrNotFound is returned when cloudflared (or another tunnel binary)
// isn't installed.
var ErrNotFound = errors.New("tunnel binary not found in PATH")

// DetectTailscale returns the tailscale binary path if installed and
// `tailscale status` succeeds; empty string otherwise. Used only to
// print an informational hint — no auto-spawn.
func DetectTailscale() string {
	p, err := exec.LookPath("tailscale")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, p, "status", "--json").Run(); err != nil {
		return ""
	}
	return p
}

// Stop terminates the tunnel process and waits for cleanup.
func (t *Tunnel) Stop() {
	if t == nil || t.cancel == nil {
		return
	}
	t.cancel()
	_ = t.cmd.Wait()
	t.wg.Wait()
}

var trycloudflareRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

func (t *Tunnel) scanForURL(r io.Reader, out chan<- string) {
	defer t.wg.Done()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := trycloudflareRe.FindString(line); m != "" {
			// Non-blocking send: only the first scanner wins.
			select {
			case out <- strings.TrimSpace(m):
			default:
			}
		}
	}
}
