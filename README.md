<div align="center">

<h1>Conduit</h1>

<p><strong>Browser-based terminal with named persistent sessions.</strong></p>

<p>
  <em>Single self-contained Go binary. Open a URL, sign in, you have your shell — on any phone, tablet, or laptop.</em>
</p>

<p>
  <a href="docs/INSTALLATION.md">Installation</a> &bull;
  <a href="docs/CONFIGURATION.md">Configuration</a> &bull;
  <a href="docs/DEPLOYMENT.md">Deployment</a> &bull;
  <a href="docs/API.md">API Reference</a> &bull;
  <a href="docs/ARCHITECTURE.md">Architecture</a> &bull;
  <a href="docs/SECURITY.md">Security</a> &bull;
  <a href="docs/PUBLIC_HARDENING_CHECKLIST.md">Public Hardening</a> &bull;
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

<p>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/Node-20+-339933?logo=node.js&logoColor=white" alt="Node 20+">
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey" alt="Platform">
</p>

</div>

---

> **conduit** `/ˈkɒn.djuː.ɪt/` — a channel for conveying something between two places.

Conduit is **one binary** that runs a local shell server and a browser UI together. No agent install on your phone. No SSH key dance. No client to update. Sign in with a token, and you get the same terminal you'd get sitting at the machine — with tabs, splits, file transfer, and sessions that survive every reconnect.

```
┌──────────────────┐                     ┌──────────────────────────────┐
│  Phone / Laptop  │  HTTPS              │  conduit.exe (single Go bin) │
│   (any browser)  │ ──── tunnel ────▶   │  ├─ embedded TS / xterm UI   │
│                  │                     │  ├─ /ws → PTY (ConPTY/PTY)   │
└──────────────────┘                     │  └─ /api → sessions, files,  │
                                         │           shares, presets    │
                                         └──────────────────────────────┘
```

## Quick Start

### 1. Build

```powershell
git clone https://github.com/mariesqu/conduit.git
cd conduit
./build.ps1           # Windows — produces ./conduit.exe
```

```bash
make build            # macOS / Linux / Git Bash — produces ./conduit
```

> Need `make`? Install via `choco install make` on Windows, or use `./build.ps1`.

### 2. Run

```bash
./conduit.exe
```

On first launch, Conduit:

- Generates a random token and writes `conduit.config.json`
- Picks an available shell (PowerShell, cmd, WSL, Git Bash, bash, zsh)
- Prints a **scannable QR code** with the URL + token embedded
- Logs the public URL if you turn on the auto-tunnel (see below)

```
conduit listening on http://127.0.0.1:7180
access URL:        http://127.0.0.1:7180/?token=4f2a8b6c19...
auth token:        4f2a8b6c19...

Scan from your phone to sign in:
█████████████████████████████████
██ ▄▄▄▄▄ █▀ █▄▄▀▄▀ ▀▀▀▄▄█ ▄▄▄▄▄ ██
██ █   █ █▀▄▀█▄ ▀█▄▀▀▄▀▀█ █   █ ██
...
```

### 3. Sign in

Open the URL in any browser, or scan the QR from your phone.

## Features

| Capability | What you get |
|------------|-------------|
| **Single binary** | One `conduit.exe`. No Node, no Python, no Docker. UI is embedded via `go:embed`. |
| **Named persistent sessions** | PTY sessions live in the server, independent of the browser. Close the tab, reattach later from any device — replay buffer included. |
| **Mirror mode** | Attach the same session from multiple devices at once. Everyone sees the same output; anyone can type. |
| **Tab + pane splits** | Multiple tabs per browser. Within a tab, split panes horizontally or vertically (Ctrl+Shift+H/V). |
| **Share links** | Generate time-limited URLs (viewer or writer) to give someone else access to one session. Server enforces read-only on the wire. |
| **Mobile-first** | Touch-friendly keyboard toolbar (ESC, TAB, CTRL+key, arrows, modifiers). PWA install. Safe-area insets for iOS. |
| **File transfer** | Drag a file onto the UI to upload to a configured root. Browse and download via the Files panel. Path-traversal blocked. |
| **Scrollback search** | Ctrl-F to search the active session's scrollback, live highlight, next/prev navigation. |
| **QR sign-in** | Startup QR encodes URL + token; one scan from your phone, you're in. |
| **Cloudflare auto-tunnel** | Set `"tunnel": "auto"` — Conduit spawns `cloudflared` if installed and uses the public URL on the QR. |
| **Session presets** | Config-defined workspaces: launch N named sessions, each pre-running its own command. |
| **Themes & font sizing** | Dark/light + 12/14/16/18px, persisted to localStorage. |

Full feature reference → [docs/CONFIGURATION.md](docs/CONFIGURATION.md)

## Shells detected automatically

| OS | Default candidates |
|----|--------------------|
| Windows | `pwsh.exe`, `powershell.exe`, `cmd.exe`, `wsl.exe`, `bash.exe` (Git Bash) |
| Unix | `bash`, `zsh`, `sh` |

Restrict via `allowed_shells` in config.

## Exposing your machine to the internet

Conduit binds to `127.0.0.1` by default. Three documented paths to make it reachable from your phone over cellular:

### Option A — Cloudflare quick tunnel (recommended, zero-setup)

```jsonc
// conduit.config.json
{ "tunnel": "auto" }
```

If `cloudflared` is in PATH, Conduit spawns a quick tunnel on boot and prints/QR-encodes the `*.trycloudflare.com` URL. Install via `winget install cloudflare.cloudflared` or [the official docs](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/).

### Option B — Tailscale (private mesh)

```bash
tailscale funnel --bg 7180
```

Conduit prints this hint automatically when it detects Tailscale.

### Option C — LAN only

```jsonc
{ "bind": "0.0.0.0", "port": 7180 }
```

Conduit auto-detects your LAN IP and uses it in the printed URL/QR.

Full deployment guide → [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)

## How named sessions work

Sessions are owned by the **server**, not the browser. Three operations:

| Action | What happens |
|--------|--------------|
| Click `+` and start a new session | A new shell process is spawned on the server and named. |
| Click `×` on a tab | **Detaches.** The WebSocket closes; the shell keeps running. |
| Open Sessions panel → Kill | Terminates the shell process. |
| Reload the page | Open tabs auto-restore by reattaching to any sessions still alive. |
| Open the same session from two devices | **Mirror.** Both see output; both can type. |

Replay buffer is 256 KB per session — roughly 2,000 lines of typical output, so reattaching on a different device gives you the recent scrollback you expect.

> **Heads-up:** sessions you open in Windows Terminal locally are **not** visible to Conduit — only sessions Conduit itself spawned. Open your portable shells through the Conduit UI even when you're at your own laptop.

## Security

- **Token auth** on every endpoint, constant-time comparison.
- **Path-traversal blocked** on file API (rejects `..`, absolute paths, drive letters).
- **WS ping/pong** every 30s with read deadline so Cloudflare doesn't time out the connection and dead clients get reaped.
- **Configurable session cap** (`max_sessions`, default 64) bounds DoS from authenticated users.
- **Per-file upload size cap** (`max_upload_mb`, default 50).
- **Viewer-mode shares** enforce read-only on the server — a malicious client can't escalate by sending crafted JSON.
- **Crypto/rand failures panic** rather than silently generating predictable IDs.

Full threat model → [docs/SECURITY.md](docs/SECURITY.md)

## Configuration reference

```jsonc
{
  "bind": "127.0.0.1",         // "0.0.0.0" for LAN access
  "port": 7180,
  "token": "",                  // auto-generated on first run
  "allowed_shells": [],         // empty == all detected; ["pwsh","wsl"] to whitelist
  "max_sessions": 64,
  "files_root": "",             // empty == ~/Conduit-Files
  "max_upload_mb": 50,
  "tunnel": "off",              // "off" | "auto" | "cloudflared"
  "presets_locked": false,      // true: create preset sessions, skip dir/command injection\n  "trust_proxy_headers": false, // trust X-Forwarded-* for absolute share URLs (trusted proxies only)\n  "presets": [                  // optional named bundles
    {
      "name": "dev",
      "description": "Local dev stack",
      "sessions": [
        { "name": "web", "shell": "pwsh", "dir": "C:\\proj\\web", "command": "npm run dev" },
        { "name": "api", "shell": "pwsh", "dir": "C:\\proj\\api", "command": "dotnet watch run" }
      ]
    }
  ]
}
```

Full reference → [docs/CONFIGURATION.md](docs/CONFIGURATION.md)

## Project layout

```
conduit/
├── main.go                      # HTTP routing, go:embed, graceful shutdown, QR
├── server/
│   ├── config.go                # JSON config + auto-token
│   ├── session.go               # PTY lifecycle, 256KB replay buffer, multi-attach
│   ├── ws.go                    # WebSocket bridge, create/attach, viewer enforcement
│   ├── shells.go                # cross-platform shell detection
│   ├── sessions_api.go          # GET/DELETE /api/sessions
│   ├── shares_api.go            # POST /api/sessions/{name}/share + listing/revoke
│   ├── shares.go                # ShareManager + TTL sweeper
│   ├── files.go                 # path-safe upload/download/list
│   ├── presets_api.go           # GET/POST /api/presets
│   ├── tunnel.go                # cloudflared auto-spawn
│   ├── qr.go                    # ANSI SGR QR renderer
│   └── netaddr.go               # LAN IP detection
└── ui/
    ├── index.html
    ├── vite.config.ts
    ├── src/
    │   ├── main.ts              # App shell, dialogs, drag-drop, keyboard
    │   ├── terminal.ts          # xterm.js + Fit + Search + WebSocket
    │   ├── layout.ts            # binary layout tree for panes
    │   ├── toolbar.ts           # mobile keyboard toolbar
    │   ├── toast.ts             # auto-dismissing notifications
    │   ├── api.ts               # REST + WS URL helpers
    │   └── style.css            # themes, layout, responsive
    └── public/                  # PWA manifest, icons, service worker
```

Detailed walkthrough → [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## Development

```bash
make dev               # prints instructions to run Vite + Go in two terminals
./build.ps1 dev        # Windows — runs both concurrently
```

The Vite dev server (`:5173`) proxies `/api` and `/ws` to the Go server (`:7180`), so HMR works for UI changes while the real backend handles sessions.

Full dev guide → [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)

## Comparison with alternatives

| Tool | Single binary | Persistent sessions | Multi-attach mirror | Mobile-first | Share links | File transfer |
|------|:-:|:-:|:-:|:-:|:-:|:-:|
| **Conduit** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| ttyd | ✅ | ❌ | ❌ | ⚠️ | ❌ | ❌ |
| wetty | ❌ (Node) | ❌ | ❌ | ⚠️ | ❌ | ❌ |
| tmux + SSH | ✅ (no UI) | ✅ | ✅ | ❌ (needs client) | ❌ | scp |
| Termius | ❌ (paid app) | via SSH/tmux | ❌ | ✅ | ❌ | ✅ |

## Roadmap

- Drag-to-resize pane splits
- Image / sixel preview support
- Per-session log files (audit + searchable history)
- Webhooks on session events (notify when `make` finishes)
- WebAuthn / passkey auth alongside token

## Contributing

Issues, ideas, and PRs welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md). For security issues, see [SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE).



