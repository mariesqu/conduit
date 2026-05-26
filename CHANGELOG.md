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
- File upload/download with rooted path traversal protection
- Ctrl-F scrollback search in xterm
- PWA manifest/icons/service worker
- Cloudflared quick tunnel detection/spawn and Tailscale hinting
- Session presets and pane splitting

### Security

- Constant-time token comparison
- WS ping/pong + read deadlines
- Session/process cleanup hardening (`cmd.Wait()`)
- Terminal dimension clamping
- JSON/body size bounds
- Panic on `crypto/rand` failure
- Session and upload limits
- Optional proxy-header trust gate for absolute share URLs (`trust_proxy_headers`)
- Optional preset command-injection lockout (`presets_locked`)

### Documentation

- Added install/config/API/deployment/architecture/security/development guides
- Added contribution templates and workflow docs
