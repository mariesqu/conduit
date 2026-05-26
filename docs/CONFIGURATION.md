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
  "presets_locked": false,
  "trust_proxy_headers": false,
  "presets": []
}
```

## Field reference

| Field | Default | Description |
|---|---:|---|
| `bind` | `127.0.0.1` | Bind address (`0.0.0.0` for LAN). |
| `port` | `7180` | HTTP port. |
| `token` | auto | Main auth token. |
| `allowed_shells` | `[]` | Empty = all detected shells. |
| `max_sessions` | `64` | Maximum live sessions. |
| `files_root` | `~/Conduit-Files` | Root for file APIs when blank. |
| `max_upload_mb` | `50` | Per-file upload limit (MB). |
| `tunnel` | `off` | `off`, `auto`, or `cloudflared`. |
| `presets_locked` | `false` | If true, preset launch creates sessions but skips `dir` and `command` injection. |
| `trust_proxy_headers` | `false` | If true, absolute share URLs can use trusted forwarded headers. Enable only behind trusted proxy. |
| `presets` | `[]` | Named multi-session launch bundles. |

## Common configurations

### Localhost-only

```json
{"bind":"127.0.0.1","port":7180,"tunnel":"off"}
```

### LAN server

```json
{"bind":"0.0.0.0","port":7180,"tunnel":"off"}
```

### Cloudflare quick tunnel

```json
{"bind":"127.0.0.1","port":7180,"tunnel":"auto"}
```

### Locked-down shell allowlist

```json
{"allowed_shells":["powershell"],"max_sessions":16,"presets_locked":true}
```
