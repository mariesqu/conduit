# Architecture

## Overview

Conduit is a single Go binary that embeds a Vite+TypeScript UI and exposes REST + WebSocket APIs.

## Server modules

- `main.go`: startup, route wiring, graceful shutdown, URL/QR output
- `server/config.go`: config loading/defaulting/token generation
- `server/session.go`: PTY lifecycle, replay buffer, multi-attach broadcast
- `server/ws.go`: WS handshake and data/control protocol
- `server/shells.go`: shell detection and auth helpers
- `server/shares.go`: share token lifecycle + expiry sweeper
- `server/shares_api.go`: share REST endpoints
- `server/files.go`: safe file APIs under rooted directory
- `server/presets_api.go`: preset list + launch behavior
- `server/tunnel.go`: cloudflared detection/spawn
- `server/qr.go`: terminal QR rendering
- `server/netaddr.go`: local/LAN host resolution
- `server/absurl.go`: trusted/untrusted absolute URL construction

## UI modules

- `ui/src/main.ts`: app shell and state orchestration
- `ui/src/terminal.ts`: xterm integration + WS bridge
- `ui/src/layout.ts`: binary split pane tree
- `ui/src/api.ts`: REST/WS client helpers
- `ui/src/toolbar.ts`: mobile keyboard controls
- `ui/src/toast.ts`: notifications

## WS lifecycle

```text
connect -> auth (token/share) -> ready -> replay buffer -> live stream
```

## Session fan-out model

```text
PTY output -> append replay buffer -> broadcast to all attachments
```
