# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x     | ✅ |

## Reporting a Vulnerability

Please report vulnerabilities privately:

- GitHub Security Advisory (preferred)
- Email: <your-security-email@example.com>

Do not open public issues for security vulnerabilities.

## Response Targets

- Initial acknowledgement: within **72 hours**
- Fix or status update: within **7 days**

## Disclosure Policy

We use coordinated disclosure with a maximum **90-day** window unless extended by mutual agreement.

## Scope

### In scope

- Go server runtime and APIs
- Browser UI served by Conduit
- Build artifacts produced from this repository

### Out of scope

- Third-party dependency vulnerabilities unless caused by Conduit misuse
- Social engineering
- Physical access attacks

## Known Accepted Risk

Conduit supports token auth in WebSocket query strings (`/ws?token=` or `/ws?share=`) by design, because browser WebSocket APIs cannot set custom headers during handshake.

Mitigations:

- Prefer TLS transport (Cloudflare/Tailscale/reverse proxy)
- Token links are stripped from browser history after login
- Server-side token checks remain mandatory
