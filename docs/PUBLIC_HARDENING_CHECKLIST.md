# Public Repository Hardening Checklist

Use this checklist before each release and after major maintenance changes.

## 1) Secrets audit (critical)

- [ ] Confirm no runtime tokens/secrets are committed (`conduit.config.json` should not be tracked).
- [ ] Scan repo working tree:

```bash
git ls-files | xargs -I{} sh -c 'grep -nE "(token|secret|apikey|api_key|password)" "{}" || true'
```

PowerShell alternative:

```powershell
Get-ChildItem -Recurse -File | Select-String -Pattern "token|secret|apikey|api_key|password"
```

- [ ] If a secret was ever committed, rotate it and rewrite history if needed.

## 2) Branch protection baseline

Configure in GitHub Settings → Branches (`main`):

- [ ] Require pull request before merging
- [ ] Require status checks to pass before merging
- [ ] Require branch to be up to date before merging
- [ ] Restrict force pushes
- [ ] Restrict branch deletion

Recommended required checks:

- [ ] `build (windows-latest)`
- [ ] `build (ubuntu-latest)`
- [ ] `build (macos-latest)`

## 3) Dependency and vulnerability hygiene

- [ ] Dependabot enabled (`.github/dependabot.yml`)
- [ ] Dependabot alerts enabled in repository Security settings
- [ ] GitHub secret scanning enabled (if available for your plan)
- [ ] Review and merge security patches quickly

## 4) Security defaults verification

Before publishing docs/examples, confirm defaults remain secure:

- [ ] `bind` defaults to `127.0.0.1`
- [ ] `trust_proxy_headers` defaults to `false`
- [ ] `presets_locked` behavior documented for hardened multi-user deployments
- [ ] Token-auth flow documented with WS query-string risk/tradeoff

## 5) Release hygiene

- [ ] Update `CHANGELOG.md`
- [ ] Tag release (e.g., `v0.1.0`)
- [ ] Build binaries for all supported OS
- [ ] Publish checksums (SHA256)
- [ ] Create GitHub Release notes from changelog

## 6) Documentation sanity

- [ ] README links resolve
- [ ] docs/API.md matches current server behavior
- [ ] docs/CONFIGURATION.md includes all live config fields
- [ ] SECURITY.md and docs/SECURITY.md are consistent
