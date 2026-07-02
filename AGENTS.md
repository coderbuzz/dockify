# AGENTS.md

## Project Overview

Dockify is a self-hosted Docker app deployment platform — single Go binary, SQLite, SSH workers, Caddy + Cloudflare automation. Full project reference: `SPEC.md`.

## Build

```bash
go build -o dockify ./cmd/dockify
./dockify serve        # start server
./dockify version      # print version
```

## Development Workflow

This project uses [Air](https://github.com/air-verse/air) for live-reload.

```bash
air                      # http://localhost:8080, auto-rebuilds on save
```

Air watches `.go` files in `cmd/` and `internal/` + `.html` templates. On build error the **old server keeps running** (`stop_on_error = false`). Data stored in `./data/` (SQLite, gitignored). Credentials in `.env` (gitignored). Config: `.air.toml` at project root.

### Key Commands

```bash
go build ./...           # build all packages
go vet ./...             # lint
go mod tidy              # clean dependencies

# Run once (no auto-reload)
DOCKIFY_DATA_DIR=./data go run ./cmd/dockify serve

# Verify templates parse
go test ./internal/http/... -run TestTemplates

# Build and run with Docker
docker build -t dockify .
docker run -p 8080:8080 -v $(pwd)/data:/var/lib/dockify dockify
```

### Dev Mode

Set `DOCKIFY_DEV_MOCK=true` to use mock SSH client — no real VMs needed for UI development. Yellow "Dev Mock Mode" banner appears in the nav. Includes a mock interactive terminal for the SSH console.

## UI Style Guide

All styles live in `internal/http/templates/layout.html` (single `<style>` block). Full design tokens and component patterns in `SPEC.md`.

When editing templates or adding new UI:
- Always consult SPEC.md for the color system, component patterns, and CSS conventions
- No CSS frameworks — fully custom
- Class naming: lowercase with hyphens
- Use CSS variables (custom properties) for theming
- Dark mode via `<html class="dark">`, toggle persisted in `localStorage('dockify-theme')`
- Responsive: single `@media (max-width: 600px)` breakpoint

## Git Workflow

### Branch Strategy
- `main` — stable, production-ready. Only updated via merge PR.
- `feat/*` — new features
- `fix/*` — bug fixes

### Daily Flow
1. **Start task:** `git checkout main && git pull && git checkout -b feat/x`
2. **Work:** edit → commit → push (WIP commits allowed)
3. **Switch PC:** commit + push first; `git pull` on new PC, `git checkout feat/x`
4. **Done:** push final → `gh pr create --fill`
5. **User says "merge"** → `gh pr merge --merge --delete-branch && git branch -d feat/x && git checkout main`
6. **Release (from main):** `./scripts/release.sh patch`

### Commit Messages
```
feat: add dark mode toggle
fix: handle empty server list
refactor: extract deploy logic
docs: update README
wip: partial work (feature branch only, never on main)
```

### Rules
- Never commit/amend directly to `main`
- Never force push
- Never commit `.env`, tokens, or secrets
- Remote branch auto-deletes after merge; delete local branch manually

## Release

```bash
./scripts/release.sh patch    # bump 0.1.0 → 0.1.1
./scripts/release.sh minor    # bump 0.1.1 → 0.2.0
./scripts/release.sh major    # bump 0.2.0 → 1.0.0
./scripts/release.sh 1.5.0    # set exact version
```

Pushing a `v*` tag triggers CI which builds binary, creates GitHub Release, and pushes Docker image to `ghcr.io/coderbuzz/dockify`.
