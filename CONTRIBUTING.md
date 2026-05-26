# Contributing to Conduit

Thanks for your interest in making Conduit better. This document covers everything you need to ship a change.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Ways to Contribute](#ways-to-contribute)
- [Local Development](#local-development)
- [Smoke tests](#smoke-tests)
- [Conventions](#conventions)
- [Pull Request Checklist](#pull-request-checklist)
- [Reporting Bugs](#reporting-bugs)
- [Security Issues](#security-issues)

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By participating you agree to its terms.

## Ways to Contribute

- **Report a bug** — open an Issue with the bug-report form
- **Propose a feature** — open an Issue with the feature-request form (please check the [Roadmap](README.md#roadmap) first)
- **Improve docs** — typos, missing context, better diagrams all welcome
- **Send a PR** — see [Pull Request Checklist](#pull-request-checklist) below
- **Triage** — reproduce open issues, add helpful detail

Good first issues are tagged [`good first issue`](https://github.com/mariesqu/conduit/labels/good%20first%20issue).

## Local Development

### Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24+ | ConPTY needs Windows 10 1809+ |
| Node | 20+ | for building the embedded UI |
| npm | 10+ | bundled with Node |
| `make` | optional | use `./build.ps1` on Windows if absent |

### One-shot build

```bash
make build              # produces ./conduit (Unix) or ./conduit.exe (Windows)
./build.ps1 build       # Windows equivalent
```

### Dev loop with HMR

```bash
make dev                # prints instructions for two-terminal dev
./build.ps1 dev         # Windows — runs Vite + Go concurrently
```

The Vite dev server runs on `:5173` and proxies `/api` and `/ws` to the Go server on `:7180`, so frontend changes hot-reload while the backend handles real sessions.

## Smoke tests

After a change, the minimum verification is:

```bash
go vet ./...
go build -o conduit.exe .
cd ui && npm run build && cd ..
go build -trimpath -ldflags "-s -w" -o conduit.exe .   # re-embed the new UI
```

For larger changes, exercise the end-to-end paths the relevant feature touches — see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#smoke-tests) for the curl/node smoke recipes used in CI.

## Conventions

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add Ctrl-F scrollback search
fix(server): constant-time token comparison
docs: clarify cloudflared install on Windows
refactor(ui): hoist layout tree into its own module
```

Allowed types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `chore`, `ci`.

### Go

- `go vet ./...` must pass.
- Format with `gofmt` (most editors do this on save).
- Errors that the caller can't reasonably handle should be returned, not logged-and-swallowed.
- New exported functions need a one-line `// Name explains …` doc comment.
- For concurrent code, **annotate the locking contract** at the struct field level.

### TypeScript

- `tsc --noEmit` must pass (run by `npm run build` automatically).
- `strict: true` is on — no implicit `any`.
- Prefer `const`-asserted objects + discriminated unions over class hierarchies.
- DOM access goes through `querySelector<T>` with the type set.
- No new dependencies without discussing in an issue first.

### CSS

- Custom properties in `:root` + `[data-theme='light']` only. Don't hardcode colors elsewhere.
- Use BEM-ish naming: `.block`, `.block__elem`, `.block--modifier`.
- Mobile considerations: respect `env(safe-area-inset-*)`.

### Security

Anything touching auth, file paths, sessions, or shares should:

- Reuse `tokenEqual` (constant-time) for secret comparisons
- Use `FileService.SafePath` (or its caller-side check) for any path coming from a client
- Treat the share-token path as **untrusted client mode** — server is the sole authority on `viewer` vs `writer`

See [docs/SECURITY.md](docs/SECURITY.md) for the threat model.

## Pull Request Checklist

Before opening a PR:

- [ ] Description explains **why** the change is needed (not just what)
- [ ] One logical change per PR (separate "fix a bug" from "while I'm here, refactor X")
- [ ] `go vet ./...` clean
- [ ] `cd ui && npm run build` clean (TS strict + Vite production build)
- [ ] Updated `README.md` or `docs/*` if behavior, config, or API changed
- [ ] New REST endpoint or WS message → updated [`docs/API.md`](docs/API.md)
- [ ] New config field → updated `conduit.config.example.json` + [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md)
- [ ] Manual test of the changed path (state which device/browser if it's UI-visible)
- [ ] Commit messages follow Conventional Commits

### PR template

The repo's [`.github/pull_request_template.md`](.github/pull_request_template.md) covers this — fill out every section.

## Reporting Bugs

Use the [Bug Report](.github/ISSUE_TEMPLATE/bug_report.yml) issue form. The more of these you can include, the faster the fix:

- Conduit version (`./conduit --version` if it exists, or git commit)
- OS + browser
- Minimal config to reproduce
- Server log (with `conduit.config.json` token redacted)
- Browser console errors

## Security Issues

**Do not open a public issue for security bugs.** See [SECURITY.md](SECURITY.md) for the disclosure process.

## License

By contributing, you agree your contributions will be licensed under the MIT License (see [LICENSE](LICENSE)).


