# Security Model

## Assets

- Host shell execution surface
- Session input/output data
- Files within `files_root`

## Threats and controls

- Token theft/replay: token required on all protected routes; constant-time compare
- WS dead connections: ping/pong and read deadlines
- Path traversal: `SafePath` rejects `..`, absolute paths, drive letters
- Upload DoS: body and per-file size limits
- Session DoS: `max_sessions` cap
- Share privilege escalation: viewer mode enforced server-side
- Share persistence after kill: shares revoked when session removed
- Randomness failure: panic on `crypto/rand` errors
- Proxy header spoofing: `trust_proxy_headers=false` by default
- Preset injection risk: `presets_locked=true` disables command/dir auto-injection

## Accepted risks

- WebSocket query token support is intentional browser compatibility tradeoff
- `CheckOrigin` allows cross-origin upgrades because token auth is authoritative

## Hardening recommendations

- Keep default bind on localhost
- Prefer Cloudflare/Tailscale/TLS over open internet bind
- Rotate token regularly
- Restrict `allowed_shells`
- Reduce `max_sessions` and `max_upload_mb` for public instances
- Enable `presets_locked` in shared environments
