# AGENTS.md

## Project Overview

Dockify is a self-hosted Docker app deployment platform. Single Go binary with embedded web UI, SQLite database, SSH-based worker management, Caddy reverse proxy integration, and Cloudflare DNS automation.

## Tech Stack

- **Language:** Go 1.25+
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGo)
- **Router:** `github.com/go-chi/chi/v5`
- **SSH:** `golang.org/x/crypto/ssh`
- **Web UI:** Go `html/template` + HTMX (embedded via `embed`, no build step) — fully custom CSS, no framework
- **Docker:** Multi-stage build, published to `ghcr.io/coderbuzz/dockify`

## Build

```bash
go build -o dockify ./cmd/dockify
./dockify serve        # start server
./dockify version      # print version
```

## Project Structure

```
cmd/dockify/main.go           # entry point, wires dependencies
internal/
  config/config.go            # env var config
  db/db.go + schema.sql       # SQLite setup + schema
  ssh/client.go               # SSH client (connect, exec, write file)
  server/                     # VM/Server CRUD, resource monitoring
  app/                        # App CRUD, deploy/undeploy/redeploy/rollback
  caddy/client.go             # Caddy Admin API via SSH
  cloudflare/client.go        # Cloudflare DNS API v4
  webhook/handler.go          # GitHub + GitLab webhook handler
  scheduler/scheduler.go      # Auto-select least-loaded VM
  http/                       # Chi router, templates (embedded), renderer
    templates/                # HTML templates (custom CSS + HTMX)
scripts/
  install.sh                  # One-liner install script
  setup-worker.sh             # Generate SSH key on worker, output for Dockify UI
  update.sh                   # Auto-detect mode, download & restart latest
  release.sh                  # Version bump + tag helper
Dockerfile                    # Multi-stage Docker build
docker-compose.yml            # Dockify + Caddy reverse proxy
.github/workflows/build.yml   # CI: vet, test, build binary + Docker, release on tag
```

## UI Style Guide

### Typography
- **Font:** `"Berkeley Mono", "IBM Plex Mono", ui-monospace, monospace`
- **Base size:** 15px
- **Line height:** 1.5
- **Headings:** weight 600, sizes: h1=1.35em, h2=1.1em

### Color System
All colors are defined as CSS custom properties in `:root` (light) and `html.dark` (dark) in `internal/http/templates/layout.html`.

| Token | Light | Dark |
|---|---|---|
| `--bg` | `#f5f5f5` | `#0d0d0d` |
| `--bg-elevated` | `#fff` | `#141414` |
| `--bg-card` | `#fff` | `#1a1a1a` |
| `--border` | `#d4d4d4` | `#2a2a2a` |
| `--text` | `#1a1a1a` | `#cfcecd` |
| `--text-muted` | `#666` | `#888` |
| `--link` | `#2563eb` | `#8ab4f8` |

Status colors: `--green`, `--red`, `--yellow`, `--orange` (both themes).

### Dark/Light Mode
- Default: light (`<html>` without class)
- Dark: `<html class="dark">`
- Toggle button in nav uses `localStorage('dockify-theme')`
- Icons: ☀ (light mode toggle button), ☾ (dark mode toggle button)

### Component Patterns

**Nav:** `<div class="top-nav">` with `.logo` + `.nav-links`. Compact, bottom border separator.

**Cards:** `<div class="card">` — border `1px solid var(--border)`, border-radius 6px, background `var(--bg-card)`.
  - Card title: `<h3>` with uppercase, muted, letter-spaced.

**Buttons:** `<button class="btn">` or `<a class="btn">`.
  - `btn-primary`: for primary actions (filled accent)
  - `btn-ghost`: for secondary actions (transparent, muted text)
  - `btn-red`: danger/delete actions (red text, red border on hover)

**Tables:** `<table>` with `<thead>` (uppercase th) and `<tbody>`.
  - Row hover: subtle background.

**Forms:** `<form method="POST" action="relative-path">` (never use `hx-boost`; use normal POST for forms).
  - Labels wrap inputs: `<label>Text<input></label>`
  - Grid layout: `<div class="grid">` for 2-column form rows
  - Form grid adds `.7em` margin-bottom automatically
  - Inline labels inside grids use `margin-bottom: 0`
  - Legend: uppercase, muted, no border-bottom
  - Radio: `display:inline-flex;align-items:center` on `<label>`, `input[type=radio]` has `margin: 0 .7em 0 0`
  - Error messages: `<div class="card" style="color:var(--red)">`

**Badges:** `<span class="badge badge-{status}">`. Statuses: online/offline/pending/error, running/stopped/created/deploying/failed/success.

**Breadcrumb:** `<div class="breadcrumb">` — links separated by `/`.

**Empty states:** `<div class="empty-state">` centered text + primary button.

**Logs viewer:** `<div class="log-container">` with `.log-toolbar` (buttons) + `.log-content` (pre, monospace).

**Stats dashboard:** `.stat` cards in `.grid` — number `h2`, label `small`.

### CSS Conventions
- All styles are in `internal/http/templates/layout.html` (single `<style>` block).
- No Pico CSS, no CSS framework — fully custom.
- Class naming: lowercase with hyphens.
- Use CSS variables (custom properties) for theming.
- Responsive: single `@media (max-width: 600px)` breakpoint.

## How to Release a New Version

### Option 1: Interactive script (recommended)

```bash
./scripts/release.sh patch    # bump 0.1.0 -> 0.1.1
./scripts/release.sh minor    # bump 0.1.1 -> 0.2.0
./scripts/release.sh major    # bump 0.2.0 -> 1.0.0
./scripts/release.sh 1.5.0    # set exact version
```

The script will:
1. Detect the latest git tag
2. Calculate the next version
3. Create a git tag
4. Push the tag to origin

### Option 2: Manual

```bash
git tag v0.2.0 && git push origin v0.2.0
```

### CI Automation

Pushing a `v*` tag triggers `.github/workflows/build.yml` which automatically:
1. Builds the Go binary for linux/amd64 with the version injected via ldflags
2. Creates a GitHub Release with the binary as an asset (`dockify-linux-amd64`)
3. Pushes a Docker image tagged with the version to `ghcr.io/coderbuzz/dockify`

No manual GitHub UI interaction needed. The install script auto-detects the latest release.

## Development Workflow

This project uses [Air](https://github.com/air-verse/air) for live-reload during development.

### Flow
1. **User** runs `air` in terminal (keeps it running)
2. **opencode** edits code/saves files
3. **Air** auto-detects changes → rebuilds Go binary → restarts server
4. **User** refreshes browser (Cmd+Shift+R) at `http://localhost:8080` to review
5. Repeat steps 2-4 until feature is complete
6. Only then release: `./scripts/release.sh patch`

### How Air works
- Watches `.go` files in `cmd/` and `internal/` + `.html` files in `internal/http/templates/`
- On successful build: kills old process, starts new one
- On build error: **old server keeps running** (thanks to `stop_on_error = false`), error shown in terminal
- Data stored in `./data/` (SQLite DB), credentials in `.env` (both gitignored)
- Config: `.air.toml` at project root

### Key development commands
```bash
# Development
go build ./...           # build all packages
go vet ./...             # lint
go mod tidy              # clean dependencies

# Run locally (auto-reload)
air                      # http://localhost:8080

# Run once (no auto-reload)
DOCKIFY_DATA_DIR=./data go run ./cmd/dockify serve

# Verify templates parse correctly
go test ./internal/http/... -run TestTemplates

# Build and run with Docker
docker build -t dockify .
docker run -p 8080:8080 -v $(pwd)/data:/var/lib/dockify dockify
```

## Git Workflow

### Branch Strategy
- `main` — stable, production-ready. Hanya diisi dari merge PR.
- `feat/*` — fitur baru
- `fix/*` — perbaikan bug

### Daily Flow

```
main ───── feat/new-feature
  │              │
  ├─ buat branch ┘
  │              ├─ kerja, commit (wip boleh), push
  │              ├─ switch PC → pull, lanjut, push
  │              │  (ulang sampai selesai)
  │              │
  │         selesai
  │              ├─ push final
  │              ├─ gh pr create --fill
  │              ├─ user: "merge"
  │              ├─ gh pr merge --merge --delete-branch
  │              └─ git branch -d feat/x
  │              │
  ├─ merge ──────┘
  │
  ├─ ./scripts/release.sh patch
  └─ push tag
```

### Step-by-step

1. **opencode start task:** `git checkout main && git pull && git checkout -b feat/x`
2. **opencode kerja:** edit → commit → push (bisa WIP)
3. **Switch PC:** commit + push dulu di PC lama; `git pull` di PC baru, `git checkout feat/x`
4. **Selesai:** push final → `gh pr create --fill`
5. **User review:** user bilang "merge" kalau OK
6. **opencode merge:** `gh pr merge --merge --delete-branch && git branch -d feat/x && git checkout main`
7. **Release (dari main):** `./scripts/release.sh patch`

### Commit Messages
```
feat: add dark mode toggle
fix: handle empty server list
refactor: extract deploy logic
docs: update README
wip: partial work (feature branch only, never on main)
```

### Rules
- Tidak pernah commit/amend langsung di `main`
- Tidak pernah force push
- Tidak commit `.env`, token, atau secret
- Branch remote auto-hapus setelah merge, branch local dihapus manual
- Push dulu sebelum switch PC (biar bisa lanjut di perangkat lain)

## Environment Variables

| Variable | Default | Required |
|---|---|---|
| `DOCKIFY_HOST` | `0.0.0.0` | No |
| `DOCKIFY_PORT` | `8080` | No |
| `DOCKIFY_DATA_DIR` | `/var/lib/dockify` | No |
| `DOCKIFY_SSH_KEY_DIR` | `/var/lib/dockify/keys` | No |
| `CLOUDFLARE_API_TOKEN` | - | For DNS |
| `CLOUDFLARE_ZONE_ID` | - | For DNS |

## API Endpoints

### Servers
```
GET    /api/servers              List all servers
POST   /api/servers              Add a new server
GET    /api/servers/:id          Get server details
DELETE /api/servers/:id          Remove server
POST   /api/servers/:id/init     Initialize worker (install Docker + Caddy)
```

### Apps
```
GET    /api/apps                 List all apps
POST   /api/apps                 Create and deploy app (server_id=0 for auto-select)
GET    /api/apps/:id             Get app details
DELETE /api/apps/:id             Undeploy and remove app
POST   /api/apps/:id/redeploy    Redeploy app
POST   /api/apps/:id/rollback    Rollback to last successful deployment
GET    /api/apps/:id/deployments Deployment history
GET    /api/apps/:id/logs        Stream app logs (SSH docker compose logs)
```

### Webhooks
```
POST   /api/webhook/github      GitHub push webhook
POST   /api/webhook/gitlab      GitLab push webhook
```

## Deploy Flow

When an app is deployed:
1. Select server (manual or auto-scheduler)
2. SSH → write `docker-compose.yml` to `/opt/dockify/apps/:name/`
3. SSH → `docker compose up -d`
4. Inject Caddy route via Admin API
5. Create Cloudflare DNS A record (if configured)
6. Save compose snapshot for rollback
7. Record deployment in history

## Worker Init Flow

`POST /api/servers/:id/init` triggers:
1. SSH connect + verify
2. Install Docker via `get.docker.com`
3. Create `dockify` Docker network
4. Deploy Caddy container (ports 80/443 + Admin API localhost:2019)
