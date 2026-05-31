# Security Model

## Assets

- Host shell execution surface
- Session input/output data
- Files within `files_root`

## Threats and controls

- Token theft/replay: token required on all protected routes; constant-time compare
- **Token in URL → proxy/history leakage**: the long-lived token is exchanged (header-authenticated) for a **short-lived ticket** (`/api/ticket`, ~30s) for the two URL-only paths — the WebSocket upgrade and download links. The permanent token never appears in a loggable URL during normal use.
- **Token compromise / revocation**: `POST /api/token/rotate` (Settings → **Rotate**) issues a new token, persists it, and invalidates every other client.
- **Cleartext exposure**: a non-loopback bind over plain HTTP is **refused at startup** unless `tls_cert`/`tls_key` are set or `allow_insecure=true`. Direct HTTPS is supported via `tls_cert`/`tls_key`.
- **Response hardening**: every response carries `Content-Security-Policy` (`script-src 'self'`), `Referrer-Policy: no-referrer` (so the token can't leak via `Referer`), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and HSTS.
- WS cross-site hijacking: `CheckOrigin` now enforces same-origin (plus `allowed_origins`); not strictly required given bearer-token auth, but blocks the DNS-rebinding edge.
- Credential flooding: per-IP rate limit on `/api/auth`.
- WS dead connections: ping/pong and read deadlines
- Path traversal: `SafePath` rejects `..`, absolute paths, drive letters, **and resolves symlinks** so an in-root symlink can't escape the root
- Upload DoS: body and per-file size limits
- Session DoS: `max_sessions` cap
- Share privilege escalation: viewer mode enforced server-side
- Share persistence after kill: shares revoked when session removed
- Randomness failure: panic on `crypto/rand` errors
- Proxy header spoofing: `trust_proxy_headers=false` by default
- Preset injection risk: `presets_locked=true` disables command/dir auto-injection; control characters (CR/LF) are stripped from preset values regardless
- Information disclosure: file-API errors return generic messages; details are logged server-side only
- Audit trail: failed auth, share creation/attach (with client IP), session creation, and token rotation are logged

## Accepted risks

- WebSocket/download still accept the raw `?token=` for non-browser clients (curl, scripts). The UI never uses it; prefer tickets.
- A reverse proxy that rewrites the `Host` header will fail the same-origin WebSocket check — set `trust_proxy_headers=true` (so `X-Forwarded-Host` is honored) or list the public host in `allowed_origins`.
- `style-src` allows `'unsafe-inline'` because xterm.js injects styles at runtime; scripts remain locked to `'self'`.

## Hardening recommendations

- Keep default bind on localhost
- Prefer Cloudflare/Tailscale/TLS over open internet bind
- Rotate the token from Settings after sharing a device or suspecting leakage
- Restrict `allowed_shells`
- Reduce `max_sessions` and `max_upload_mb` for public instances
- Enable `presets_locked` in shared environments
