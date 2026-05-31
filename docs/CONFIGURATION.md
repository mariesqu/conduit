# Configuration

Conduit uses `conduit.config.json`.

```json
{
  "bind": "127.0.0.1",
  "port": 7180,
  "token": "",
  "allowed_shells": [],
  "max_sessions": 64,
  "files_root": "",
  "max_upload_mb": 50,
  "tunnel": "off",
  "tls_cert": "",
  "tls_key": "",
  "allow_insecure": false,
  "allowed_origins": [],
  "presets_locked": false,
  "trust_proxy_headers": false,
  "presets": []
}
```

## Field reference

| Field | Default | Description |
|---|---:|---|
| `bind` | `127.0.0.1` | Bind address. A non-loopback bind (e.g. `0.0.0.0`) over plain HTTP is **refused** unless `tls_cert`/`tls_key` are set or `allow_insecure` is `true` — see [SECURITY.md](SECURITY.md). |
| `port` | `7180` | Listen port. |
| `token` | auto | Main auth token. Rotate it from the in-app Settings → **Rotate** (invalidates every other signed-in client). |
| `allowed_shells` | `[]` | Empty = all detected shells. |
| `max_sessions` | `64` | Maximum live sessions. |
| `files_root` | `~/Conduit-Files` | Root for file APIs when blank. |
| `max_upload_mb` | `50` | Per-file upload limit (MB). |
| `tunnel` | `off` | `off`, `auto`, or `cloudflared`. |
| `tls_cert` | `""` | Path to a PEM certificate. Set with `tls_key` to serve HTTPS directly (no proxy needed). |
| `tls_key` | `""` | Path to the matching PEM private key. |
| `allow_insecure` | `false` | Permit a non-loopback bind over plain HTTP. **Off by default** — turning it on serves the token and every keystroke in cleartext. Only enable on an already-encrypted network (e.g. WireGuard/Tailscale overlay). |
| `allowed_origins` | `[]` | Extra `Origin` hosts accepted on the WebSocket upgrade. Same-origin is always allowed; use this only for split-origin / proxy setups that rewrite `Host`. |
| `presets_locked` | `false` | If true, preset launch creates sessions but skips `dir` and `command` injection. |
| `trust_proxy_headers` | `false` | If true, `X-Forwarded-Proto`/`-Host`/`-For` are honored for absolute share URLs, WebSocket origin matching, and client-IP logging. Enable only behind a trusted proxy. |
| `presets` | `[]` | Named multi-session launch bundles. |

## Common configurations

### Localhost-only

```json
{"bind":"127.0.0.1","port":7180,"tunnel":"off"}
```

### LAN server (HTTPS)

Serving on a reachable interface requires TLS (or an explicit insecure override):

```json
{"bind":"0.0.0.0","port":7180,"tls_cert":"./conduit.crt","tls_key":"./conduit.key"}
```

### LAN server over an already-encrypted overlay

If the network itself is encrypted (WireGuard, Tailscale), you may opt out of TLS:

```json
{"bind":"0.0.0.0","port":7180,"allow_insecure":true}
```

### Cloudflare quick tunnel

Keep the bind on loopback; the tunnel provides public TLS:

```json
{"bind":"127.0.0.1","port":7180,"tunnel":"auto","trust_proxy_headers":true}
```

### Locked-down shell allowlist

```json
{"allowed_shells":["powershell"],"max_sessions":16,"presets_locked":true}
```
