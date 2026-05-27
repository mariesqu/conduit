# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
