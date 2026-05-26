# Conduit

A single self-contained Go binary that serves a browser-based terminal. PTY-backed shells (PowerShell, cmd, WSL, bash) over WebSocket, with an embedded xterm.js UI. Works on desktop and mobile browsers.

```
┌───────────────────┐
│   Mobile/Desktop  │  HTTPS (Cloudflare Tunnel)
│      Browser      │ ─────────────────────────┐
└───────────────────┘                          │
                                               ▼
                                  ┌──────────────────────────┐
                                  │   conduit.exe (Go)       │
                                  │   ├─ embedded UI (TS)    │
                                  │   ├─ /ws  → PTY session  │
                                  │   └─ /api → shells, auth │
                                  └──────────────────────────┘
```

## Features

- **Single binary** — `conduit.exe`, no runtime files needed (UI is embedded via `go:embed`)
- **Multiple shells** — auto-detects `pwsh`, `powershell`, `cmd`, `wsl`, Git Bash on Windows
- **Named persistent sessions** — sessions live in the server, independent of any browser. Detach from one device, reattach from another, scrollback intact.
- **Mirror mode** — attach the same session from multiple devices simultaneously; everyone sees the same output and can type.
- **Tabs** — open multiple concurrent sessions; tabs auto-restore on page refresh if their server session is still alive.
- **Mobile-first** — touch-friendly keyboard toolbar (ESC, TAB, CTRL+key, arrows)
- **Themes** — dark/light, font-size picker, persisted to localStorage
- **Auth** — secret token; required for login screen, REST, and WebSocket handshake
- **Cloudflare-ready** — bound to localhost by default; expose via `cloudflared`

## Requirements

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24+ | Windows ConPTY (Windows 10 1809+) or any Unix |
| Node | 20+ | for building the UI |
| npm | 10+ | bundled with Node |
| `make` | optional | use `./build.ps1` on Windows if `make` isn't installed |

## Quick start

```powershell
# Windows
./build.ps1 build      # build UI + compile single binary
./conduit.exe          # generates conduit.config.json with a random token on first run
```

```bash
# Linux / macOS / Git Bash
make build
./conduit.exe          # or ./conduit
```

On first launch, Conduit prints something like:

```
conduit listening on http://127.0.0.1:7180
auth token: 4f2a8b6c19...
bound to localhost — expose with: cloudflared tunnel --url http://localhost:7180
```

Open the URL in any browser, paste the token, and you're in.

## Configuration

The first run writes `conduit.config.json` next to the binary. Edit and restart to change settings.

```jsonc
{
  "bind": "127.0.0.1",        // "0.0.0.0" for LAN access
  "port": 7180,
  "token": "…",               // auto-generated; replace with your own if you want
  "allowed_shells": []        // empty == allow all detected shells, e.g. ["powershell","wsl"]
}
```

Use `-config path/to/file.json` to point at a custom location.

## Exposing remotely

### Cloudflare Tunnel (quick mode — temporary URL)

```bash
cloudflared tunnel --url http://localhost:7180
```

Cloudflare prints a `*.trycloudflare.com` URL. WebSocket upgrades work out of the box on Cloudflare's edge.

### Cloudflare Tunnel (named — persistent URL)

```bash
cloudflared tunnel login
cloudflared tunnel create conduit
cloudflared tunnel route dns conduit term.example.com
```

`~/.cloudflared/config.yml`:

```yaml
tunnel: <tunnel-uuid>
credentials-file: <path-to-cred>.json
ingress:
  - hostname: term.example.com
    service: http://localhost:7180
  - service: http_status:404
```

Then:

```bash
cloudflared tunnel run conduit
```

### LAN mode (no tunnel)

Set `"bind": "0.0.0.0"` and open `http://<machine-ip>:7180` from any device on the network. Combine with the token auth — never expose without it.

## Development

```powershell
./build.ps1 dev        # starts Vite on :5173 (proxies /api and /ws to Go on :7180)
                       # and `go run .` in parallel
```

Visit `http://localhost:5173` — UI is served by Vite with HMR, but talks to the real Go server through the proxy.

```bash
make dev               # prints instructions to run Vite + Go in two terminals
```

## Project layout

```
conduit/
├── main.go                      # HTTP routing, embedded UI, graceful shutdown
├── server/
│   ├── config.go                # JSON config, token generation
│   ├── shells.go                # detection of available shells
│   ├── session.go               # PTY lifecycle (go-pty / ConPTY), buffer, multi-attach
│   ├── sessions_api.go          # REST: GET /sessions, DELETE /sessions/{name}
│   └── ws.go                    # WebSocket bridge, create/attach handshake, SPA fallback
├── ui/
│   ├── index.html
│   ├── vite.config.ts           # Vite + dev proxy to Go server
│   ├── package.json
│   ├── src/
│   │   ├── main.ts              # tab manager, login, settings, app shell
│   │   ├── terminal.ts          # xterm.js + FitAddon + WebSocket I/O
│   │   ├── toolbar.ts           # mobile keyboard toolbar
│   │   ├── api.ts               # /api/auth, /api/shells, ws URL
│   │   └── style.css            # themes, layout, responsive
│   └── dist/                    # built UI — embedded into binary at compile time
├── Makefile                     # build, dev, clean, vendor, tidy
├── build.ps1                    # Windows equivalent of the Makefile
├── conduit.config.example.json
└── README.md
```

## Named sessions

Sessions are created and named on the server. They live independent of any browser tab — close the browser, the session keeps running. Open a new tab from another device, attach to the same name, and you're back where you left off (with up to ~256 KB of replay buffer).

| Action | What happens |
|--------|--------------|
| Click `+` and start a new session | A new shell process is spawned on the server and named (you can pick the name or let it auto-generate, e.g. `cmd-a3f2`). |
| Click the `×` on a tab | **Detaches** — the WebSocket closes but the shell keeps running. Reattach anytime. |
| Click the `☰` sessions icon, then `Kill` | Terminates the shell process and removes the session from the server. |
| Open the same session from two devices | **Mirror mode** — both see the same output, either can type. |
| Refresh the page | Open tabs auto-restore by reattaching to their server sessions. |

### Limits

- 256 KB replay buffer per session — covers about 2,000 lines of typical output.
- Sessions you start in Windows Terminal locally are **not** visible to Conduit — Conduit only manages sessions it spawned itself. Use Conduit in the browser (even on the same machine) for sessions you want to be portable.

## REST API

All endpoints require `X-Auth-Token: <token>` header (or `?token=<token>` for `/ws`).

| Method | Path | Body / params | Response |
|--------|------|---------------|----------|
| POST | `/api/auth` | `{"token":"…"}` | 200 / 401 |
| GET | `/api/shells` | — | `[{name,path,args}]` |
| GET | `/api/sessions` | — | `[{name, shell, created_at, attached, alive}]` |
| DELETE | `/api/sessions/{name}` | — | 204 / 404 |

## WebSocket protocol

Client opens `wss://host/ws?token=<token>`. First message picks create-vs-attach:

| Direction | Frame type | Payload |
|-----------|------------|---------|
| → server | text (JSON) | `{"type":"create","shell":"powershell","name":"work","cols":120,"rows":30}` |
| → server | text (JSON) | `{"type":"attach","name":"work","cols":120,"rows":30}` |
| ← server | text (JSON) | `{"type":"ready","name":"work","shell":"powershell","created":true}` |
| ← server | text (JSON) | `{"type":"error","message":"…"}` or `{"type":"ended","reason":"…"}` |
| → server | binary | raw keystrokes for PTY stdin |
| → server | text (JSON) | `{"type":"resize","cols":N,"rows":N}`, `{"type":"detach"}`, or `{"type":"kill"}` |
| ← server | binary | raw PTY stdout/stderr (replay buffer sent first on attach, then live) |

Binary frames are used for the hot path (per-byte I/O) to avoid JSON overhead. `detach` closes the WebSocket but keeps the server session alive; `kill` terminates the shell process.

## Security notes

- **Always use a token.** The token is generated on first run; rotate by editing `conduit.config.json` and restarting.
- **Bind to localhost** unless you're on a trusted LAN — combine with a tunnel for remote access.
- **HTTPS via the tunnel.** Conduit itself serves plain HTTP; terminate TLS at Cloudflare (or any reverse proxy).
- **Auth is checked** on `/api/*` and on the WebSocket handshake. There is no anonymous endpoint.
- **One session per WebSocket.** Closing the WS kills the PTY; closing the PTY (shell exits) closes the WS.

## License

MIT — see [LICENSE](LICENSE).
