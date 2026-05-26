# API Reference

All endpoints require auth via `X-Auth-Token` header or `?token=` query.

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
