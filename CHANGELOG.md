# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Windows: runs as a **system-tray app with no console window** (release
  builds link with `-H=windowsgui`). Tray menu: open in browser, copy access
  URL / token, show QR, rotate token, start-with-Windows toggle, open log,
  quit. `--console` flag restores the foreground/QR behavior for debugging.
  Tray-mode logs go to `%LOCALAPPDATA%\Conduit\conduit.log`.
- Short-lived tickets (`POST /api/ticket`) so the long-lived token no longer
  travels in WebSocket or download URLs (keeps it out of proxy/access logs)
- Token rotation (`POST /api/token/rotate`, Settings → **Rotate**) — a revoke
  path that signs out every other client
- Direct HTTPS via `tls_cert`/`tls_key`
- WebSocket `Origin` enforcement with optional `allowed_origins` allowlist
- Per-IP rate limiting on the credential endpoints
- Audit logging of auth failures, share create/attach, session creation, and
  token rotation (with client IP)
- Security tests for tickets, rate limiter, origin check, preset sanitization,
  and token rotation

### Changed

- A non-loopback bind over plain HTTP is now **refused at startup** unless
  `tls_cert`/`tls_key` are set or `allow_insecure=true` (secure by default)
- Access token is masked in Settings (reveal/copy on demand)

### Security

- Security response headers on every response: CSP (`script-src 'self'`),
  `Referrer-Policy: no-referrer`, `nosniff`, `X-Frame-Options: DENY`, HSTS
- `SafePath` now resolves symlinks so an in-root symlink can't escape the root
- Control characters stripped from preset `dir`/`command`; tighter shell quoting
- File-API errors return generic messages; details logged server-side only

### Fixed

- Terminal rendered as a tiny unreadable column until the browser window was
  resized — and stayed broken on mobile, which has no resize gesture. The fit
  now runs after layout (and never on a zero-size pane), so it's correct on
  first paint.
- Mobile: terminal scrollback is now touch-scrollable (the page's
  `overscroll-behavior` was swallowing the gesture)

## [0.1.0] - 2026-05-26

### Added

- Named persistent sessions decoupled from WebSocket lifecycle
- Session replay buffer (256 KB), multi-attach mirror, detach/kill semantics
- QR sign-in and magic link token bootstrap
- Time-limited share links with viewer/writer modes
- Share create response now includes `absolute_url` alongside relative `url`
- File upload/download with rooted path traversal protection
- Ctrl-F scrollback search in xterm
- PWA manifest/icons/service worker
- Cloudflared quick tunnel detection/spawn and Tailscale hinting
- Session presets and pane splitting
- Security-critical unit tests (54 subtests) covering SafePath, token
  comparison, share TTL/revoke, name regex, and AbsoluteURL hardening

### Security

- Constant-time token comparison
- WS ping/pong + read deadlines
- WS message size cap (64 KiB) via `SetReadLimit`
- REST endpoints restricted to header-only auth (`?token=` retained only
  for `/ws` and `/api/files/download` where headers can't be set)
- Session/process cleanup hardening (`cmd.Wait()`)
- Terminal dimension clamping
- JSON/body size bounds
- Panic on `crypto/rand` failure
- Session and upload limits
- Optional proxy-header trust gate for absolute share URLs
  (`trust_proxy_headers`); when on, X-Forwarded-Proto/Host are
  validated against scheme + host charset rules so misconfigured
  proxies can't inject `javascript:` schemes, credentials, paths, or
  CRLF header-injection into share URLs
- Optional preset command-injection lockout (`presets_locked`)
- Windows DACL tightening of `conduit.config.json` to current user
  via `golang.org/x/sys/windows.SetNamedSecurityInfo` (no-op on Unix
  where `0o600` already enforces owner-only access)

### Documentation

- Added install/config/API/deployment/architecture/security/development guides
- Added contribution templates and workflow docs

[Unreleased]: https://github.com/mariesqu/conduit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mariesqu/conduit/releases/tag/v0.1.0
