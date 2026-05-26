# Development

## Build targets

- `make build` / `./build.ps1 build`
- `make dev` / `./build.ps1 dev`
- `make clean` / `./build.ps1 clean`
- `make tidy` / `./build.ps1 tidy`
- `make vendor` / `./build.ps1 vendor`

## Validation loop

```bash
go vet ./...
go test ./server/...
cd ui && npm run build
```

## Debugging tips

- Run server in foreground for logs
- Use browser DevTools for `/api/*` and `/ws`
- Confirm service-worker behavior in production build

## Adding REST endpoints

1. Add route in server package
2. Enforce auth
3. Add request/response validation
4. Update `docs/API.md`

## Adding WS message types

1. Extend `clientMsg` handling in `server/ws.go`
2. Keep server-side authorization authoritative
3. Update UI protocol handlers
4. Update `docs/API.md`
