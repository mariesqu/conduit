# Deployment

## Cloudflare quick tunnel

Set config:

```json
{"tunnel":"auto"}
```

Install and run:

```powershell
winget install cloudflare.cloudflared
.\conduit.exe
```

## Cloudflare named tunnel

`~/.cloudflared/config.yml` example:

```yaml
tunnel: conduit
credentials-file: /home/user/.cloudflared/<id>.json
ingress:
  - hostname: conduit.example.com
    service: http://127.0.0.1:7180
  - service: http_status:404
```

## Tailscale funnel

```bash
tailscale funnel --bg 7180
./conduit
```

## Bare LAN

```json
{"bind":"0.0.0.0","port":7180}
```

## Windows service (NSSM)

```powershell
winget install NSSM.NSSM
nssm install Conduit "C:\Conduit\conduit.exe"
nssm set Conduit AppDirectory "C:\Conduit"
nssm start Conduit
```

## Linux systemd

`/etc/systemd/system/conduit.service`:

```ini
[Unit]
Description=Conduit
After=network.target

[Service]
Type=simple
User=conduit
WorkingDirectory=/opt/conduit
ExecStart=/opt/conduit/conduit
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
