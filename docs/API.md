# API Reference

## Authentication

All protected endpoints require the main auth token.

| Surface | How to pass the token |
|---------|------------------------|
| REST `/api/*` (except download) | `X-Auth-Token: <token>` header **only** |
| `GET /api/files/download` | Header **or** `?token=<token>` query (so plain `<a download>` links work) |
| WebSocket `/ws` | `?token=<MAIN>` or `?share=<SHARE_TOKEN>` query (browsers can't set custom headers on the WS handshake) |

The query-string fallback is intentionally narrow — keeping it off the REST path keeps tokens out of access logs and referer headers. Comparison is constant-time via `crypto/subtle`.

## REST

### `POST /api/auth`

Request:

```json
{"token":"..."}
```

### `GET /api/shells`

Returns available shells.

### `GET /api/sessions`

Returns session list.

### `DELETE /api/sessions/{name}`

Kills session.

### `POST /api/sessions/{name}/share`

Request:

```json
{"mode":"viewer|writer","ttl_seconds":3600}
```

Response includes both:

- `url` (relative)
- `absolute_url` (built from request host/proto, optionally trusted proxy headers)

### `GET /api/shares`

Returns active shares.

### `DELETE /api/shares/{token}`

Revokes share.

### `POST /api/files`

Multipart upload, optional `?dir=`.

### `GET /api/files/download?path=`

Downloads file bytes.

### `GET /api/files/list?dir=`

Lists files and directories.

### `GET /api/presets`

Lists configured presets.

### `POST /api/presets/{name}/launch`

Launches preset sessions.

When `presets_locked=true`, launch still creates/attaches sessions but does not inject `dir`/`command` text.

## WebSocket

Connect:

- `ws(s)://host/ws?token=<MAIN_TOKEN>`
- `ws(s)://host/ws?share=<SHARE_TOKEN>`

Main-token first text frame:

```json
{"type":"create","shell":"powershell","name":"dev","cols":120,"rows":40}
```

or:

```json
{"type":"attach","name":"dev","cols":120,"rows":40}
```

Share-token flow auto-attaches and returns mode in `ready`.

Viewer mode ignores write/resize/kill input server-side.

Heartbeat:

- Ping every 30s
- Read deadline 70s
