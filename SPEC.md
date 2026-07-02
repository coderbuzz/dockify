# Dockify Build Spec

## Project Overview

Dockify is a self-hosted Docker app deployment platform — single Go binary, SQLite database, SSH-based worker management, embedded Web UI (Go `html/template` + HTMX), Caddy reverse proxy integration, and Cloudflare DNS automation.

## Tech Stack

| Layer | Choice | Details |
|---|---|---|
| Language | Go 1.23+ | Single binary, cross-compile |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGo, embedded |
| Router | `github.com/go-chi/chi/v5` | Middleware, groups, path params |
| SSH | `golang.org/x/crypto/ssh` | Std SSH client, PTY sessions |
| Web UI | Go `html/template` + HTMX | Embedded via `embed`, no JS framework, no build step |
| CSS | Fully custom (no framework) | CSS variables, dark/light mode, single `<style>` block |
| Terminal | xterm.js + WebSocket | Interactive SSH console in browser |
| Docker | Multi-stage build | Published to `ghcr.io/coderbuzz/dockify` |

## Project Structure

```
cmd/dockify/main.go              # Entry point, wires dependencies
internal/
  config/config.go               # Environment variable config (11 vars)
  db/db.go + schema.sql          # SQLite setup, 8 tables
  ssh/
    client.go                    # SSH client: connect, exec, write file, PTY
    mock.go                      # Mock SSH for dev mode (DOCKIFY_DEV_MOCK=true)
  server/
    service.go                   # Server CRUD (list, get, create, update, delete)
    handler.go                   # JSON API handlers
    web_handler.go               # Web UI handlers (pages + forms)
    monitor.go                   # Background resource polling (every 60s)
  app/
    service.go                   # App CRUD + deploy/undeploy/redeploy/rollback/stop/start
    handler.go                   # JSON API handlers (apps, deployments, secrets, files, logs)
    web_handler.go               # Web UI handlers (pages + forms)
    scheduler.go                 # Auto-select least-loaded server
    logs.go                      # SSH log streaming
    backup.go                    # Compose file write/read on worker
  caddy/client.go                # Caddy Admin API client (route CRUD via SSH tunnel)
  cloudflare/client.go           # Cloudflare DNS API v4 (list, create, upsert records)
  webhook/handler.go             # GitHub + GitLab webhook receiver, HMAC validation
  settings/
    handler.go                   # Settings page, webhook secret CRUD, update check/run
    updater.go                   # GitHub Releases checker, systemd-run self-update
  backup/
    handler.go                   # Export/import pages, YAML generation/parsing
    encryption.go                # AES-GCM encrypt/decrypt with PBKDF2 passphrase
  http/
    router.go                    # Chi router, all routes, middleware
    auth.go                      # Session auth, login/logout, middleware
    console.go                   # WebSocket upgrade, PTY relay for SSH console
    renderer.go                  # Template rendering with layout
    templates/                   # HTML templates (all pages, layout, partials)
      layout.html                # Base layout: nav, head, all CSS (single <style> block)
      dashboard.html             # Dashboard with stats cards + server/app summary tables
      server_list.html           # Server list
      server_add.html            # Add server form
      server_detail.html         # Server detail: info, resources card, SSH console, actions
      server_edit.html           # Edit server form
      servers_resources_card.html# HTMX partial: resource usage with human-readable values
      app_list.html              # App list
      app_add.html               # Deploy app form (simple/advanced toggle)
      app_detail.html            # App detail: info, compose, secrets editor, files, logs, deployments
      app_edit.html              # Edit app form (pre-fills from simple fields or compose)
      settings.html              # Settings: webhook secret, update check/run, export/import links
      about.html                 # About page: version, description, sponsor link
      export.html                # Export page: passphrase generator, download YAML
      import.html                # Import page: file upload, mode selector (merge/replace)
      login.html                 # Login page
scripts/
  install.sh                     # One-liner install (3 modes: Docker Compose / Binary / Binary + Caddy)
  setup-worker.sh                # Generate SSH key on worker, output private key
  update.sh                      # Auto-detect install mode, download & restart latest
  release.sh                     # Version bump + tag helper
Dockerfile                       # Multi-stage Docker build
docker-compose.yml               # Dockify + Caddy reverse proxy (mode 1)
.air.toml                        # Air live-reload config
.github/workflows/build.yml      # CI: vet, test, build binary + Docker, release on v* tag
```

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `DOCKIFY_HOST` | `0.0.0.0` | No | Network interface to bind |
| `DOCKIFY_PORT` | `8080` | No | HTTP port |
| `DOCKIFY_DATA_DIR` | `/var/lib/dockify` | No | SQLite DB + SSH keys storage |
| `DOCKIFY_SSH_KEY_DIR` | `$DATA_DIR/keys` | No | Per-server SSH private key files |
| `DOMAIN` | — | Mode 1 | Domain for Caddy reverse proxy (auto HTTPS) |
| `DOCKIFY_ADMIN_USER` | `admin` | No | Web UI login username |
| `DOCKIFY_ADMIN_PASSWORD` | — | No | Web UI password. Not set = no authentication |
| `DOCKIFY_DEV_MOCK` | `false` | No | Enable mock SSH client for local development |
| `DOCKIFY_BASE_PATH` | — | No | URL prefix when behind reverse proxy (e.g. `/proxy/9898`) |
| `CLOUDFLARE_API_TOKEN` | — | No | Cloudflare API token for DNS automation (Zone:DNS:Edit scope) |
| `CLOUDFLARE_ZONE_ID` | — | No | Cloudflare zone ID for the domain |

## Database Schema (8 Tables)

### servers

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `name` | TEXT NOT NULL | Display name |
| `host` | TEXT NOT NULL | IP or hostname |
| `port` | INTEGER DEFAULT 22 | SSH port |
| `user` | TEXT DEFAULT 'root' | SSH user |
| `ssh_key` | TEXT NOT NULL | SSH private key content |
| `status` | TEXT DEFAULT 'pending' | pending, online, offline, error, initializing |
| `cpu_cores` | INTEGER | Total CPU cores |
| `ram_mb` | INTEGER | Total RAM in MB |
| `disk_gb` | INTEGER | Total disk in GB |
| `cpu_usage` | REAL | CPU usage % (0.0 - 100.0) |
| `ram_usage` | REAL | RAM usage % (0.0 - 100.0) |
| `disk_usage` | REAL | Disk usage % (0.0 - 100.0) |
| `resources_updated_at` | DATETIME | Last resource refresh timestamp |
| `created_at` | DATETIME | Creation timestamp |
| `updated_at` | DATETIME | Last update timestamp |

### apps

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `name` | TEXT NOT NULL | Display name |
| `server_id` | INTEGER FK→servers | Target worker VM |
| `domain` | TEXT DEFAULT '' | Public domain (Caddy + Cloudflare) |
| `port` | INTEGER DEFAULT 0 | Internal container port |
| `compose` | TEXT NOT NULL | docker-compose.yml content |
| `git_repo` | TEXT | Git repo URL for webhook CI/CD |
| `git_branch` | TEXT DEFAULT 'main' | Git branch to track |
| `auth_user` | TEXT DEFAULT '' | HTTP basic auth username |
| `auth_pass` | TEXT DEFAULT '' | HTTP basic auth password (bcrypt hashed) |
| `status` | TEXT DEFAULT 'created' | created, deploying, running, stopped, failed |
| `compose_mode` | TEXT DEFAULT 'advanced' | simple or advanced |
| `created_at` | DATETIME | Creation timestamp |
| `updated_at` | DATETIME | Last update timestamp |

### deployments

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `app_id` | INTEGER FK→apps | Parent app |
| `server_id` | INTEGER FK→servers | Target server at deploy time |
| `status` | TEXT | success, failed |
| `log` | TEXT | Deployment log output |
| `commit_sha` | TEXT | Git commit SHA (webhook deploys) |
| `compose_snapshot` | TEXT | Compose content at deploy time (for rollback) |
| `created_at` | DATETIME | Deployment timestamp |

### routes

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `app_id` | INTEGER FK→apps | Parent app |
| `server_id` | INTEGER FK→servers | Worker VM with Caddy |
| `domain` | TEXT NOT NULL | Public domain |
| `target` | TEXT NOT NULL | container_name:port |
| `status` | TEXT DEFAULT 'active' | active, removed |
| `created_at` | DATETIME | Creation timestamp |

### dns_records

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `app_id` | INTEGER FK→apps | Parent app |
| `server_id` | INTEGER FK→servers | Worker VM |
| `zone_id` | TEXT NOT NULL | Cloudflare zone ID |
| `record_id` | TEXT NOT NULL | Cloudflare record ID |
| `name` | TEXT NOT NULL | Subdomain |
| `type` | TEXT DEFAULT 'A' | Record type |
| `content` | TEXT NOT NULL | Worker IP address |
| `proxied` | INTEGER DEFAULT 0 | Cloudflare proxy (orange cloud) |
| `created_at` | DATETIME | Creation timestamp |

### app_secrets

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `app_id` | INTEGER FK→apps ON DELETE CASCADE | Parent app |
| `key` | TEXT NOT NULL | Secret name |
| `value` | TEXT NOT NULL | Secret value |
| UNIQUE(app_id, key) | | One value per key per app |

### app_files

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `app_id` | INTEGER FK→apps ON DELETE CASCADE | Parent app |
| `path` | TEXT NOT NULL | File path relative to app directory |
| `content` | TEXT NOT NULL | File content |
| UNIQUE(app_id, path) | | One file per path per app |

### settings

| Column | Type | Description |
|---|---|---|
| `key` | TEXT PK | Setting name |
| `value` | TEXT NOT NULL | Setting value |
| `updated_at` | DATETIME | Last update timestamp |

## API Endpoints

### Public (no auth)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check → `ok` |
| GET/POST | `/login` | Login page / form submit |
| POST | `/logout` | Clear session, redirect to login |
| POST | `/api/webhook/github` | GitHub push webhook (HMAC-SHA256) |
| POST | `/api/webhook/gitlab` | GitLab push webhook (token) |
| GET | `/api/settings/update/check` | Check latest GitHub Release version |
| GET | `/static/*` | Static files (`web/static/`) |

### Protected — Server API (JSON)

| Method | Path | Description |
|---|---|---|
| GET | `/api/servers` | List servers |
| POST | `/api/servers` | Create server |
| GET | `/api/servers/:id` | Get server details |
| PATCH | `/api/servers/:id` | Partial update (host, port, user, ssh_key) |
| DELETE | `/api/servers/:id` | Delete server |
| POST | `/api/servers/:id/init` | Initialize worker (Docker + Caddy) |
| POST | `/api/servers/:id/refresh` | Refresh resource metrics |
| GET | `/api/servers/:id/console` | WebSocket upgrade → SSH terminal |

### Protected — Server Web UI

| Method | Path | Description |
|---|---|---|
| GET | `/servers` | Server list page |
| GET | `/servers/add` | Add server form |
| POST | `/servers/add` | Submit add server form |
| GET | `/servers/:id` | Server detail page |
| GET | `/servers/:id/edit` | Edit server form |
| POST | `/servers/:id/edit` | Submit edit server form |
| POST | `/servers/:id/init` | Initialize worker (web form) |
| GET | `/servers/:id/resources` | Resource card (HTMX partial) |
| POST | `/servers/:id/refresh` | Refresh resources (web form) |
| POST/DELETE | `/servers/:id/delete` | Delete server (web form) |

### Protected — App API (JSON)

| Method | Path | Description |
|---|---|---|
| GET | `/api/apps` | List apps |
| POST | `/api/apps` | Create and deploy app |
| GET | `/api/apps/:id` | Get app details |
| DELETE | `/api/apps/:id` | Undeploy and remove app |
| POST | `/api/apps/:id/redeploy` | Redeploy app |
| POST | `/api/apps/:id/rollback` | Rollback to last successful deployment |
| POST | `/api/apps/:id/stop` | Stop app (docker compose stop + remove Caddy route) |
| POST | `/api/apps/:id/start` | Start app (docker compose start + add Caddy route + DNS) |
| GET | `/api/apps/:id/deployments` | List deployment history (last 20) |
| GET | `/api/apps/:id/logs` | Stream logs (`docker compose logs --tail=N`) |
| GET | `/api/apps/:id/secrets` | List secrets |
| POST | `/api/apps/:id/secrets` | Set secret `{key, value}` |
| DELETE | `/api/apps/:id/secrets/:key` | Delete secret |
| GET | `/api/apps/:id/files` | List config files |
| POST | `/api/apps/:id/files` | Set config file `{path, content}` |
| DELETE | `/api/apps/:id/files/:path` | Delete config file |
| GET | `/api/deployments/:id` | Get single deployment detail |

### Protected — App Web UI

| Method | Path | Description |
|---|---|---|
| GET | `/apps` | App list page |
| GET | `/apps/add` | Deploy app form |
| POST | `/apps/add` | Submit deploy app form |
| GET | `/apps/:id` | App detail page |
| POST/DELETE | `/apps/:id/undeploy` | Undeploy app (web form) |
| POST | `/apps/:id/redeploy` | Redeploy app (web form) |
| POST | `/apps/:id/rollback` | Rollback app (web form) |
| POST | `/apps/:id/stop` | Stop app (web form) |
| POST | `/apps/:id/start` | Start app (web form) |
| GET | `/apps/:id/edit` | Edit app form page |
| POST | `/apps/:id/edit` | Submit edit app form |

### Protected — Settings, Backup, Pages

| Method | Path | Description |
|---|---|---|
| GET | `/` | Dashboard: stats, servers table, apps table |
| GET | `/settings` | Settings page |
| GET | `/about` | About page (version, sponsor link) |
| GET | `/export` | Export page (passphrase generator, download) |
| GET | `/import` | Import page (file upload, merge/replace) |
| GET | `/api/settings/webhook-secret` | Get webhook secret |
| POST | `/api/settings/webhook-secret/roll` | Regenerate webhook secret |
| POST | `/api/settings/webhook-secret/disable` | Disable webhook secret checking |
| POST | `/api/settings/webhook-secret/enable` | Re-enable webhook secret checking |
| POST | `/api/settings/update/run` | Trigger self-update via systemd-run |
| POST | `/api/backup/export` | Download YAML export |
| POST | `/api/backup/import` | Upload YAML import (multipart: `file` + `mode`) |

## Deploy Flow

When an app is deployed (create/redeploy):

1. Parse and validate compose (simple mode: generate compose from image + env vars + volumes)
2. Select target server (manual or auto-scheduler — `server_id=0` = auto-select)
3. Ensure `dockify` Docker network exists on worker
4. SSH → write `docker-compose.yml` to `/opt/dockify/apps/app-{id}/`
5. Write app secrets as `.env` file to `/opt/dockify/apps/app-{id}/.env`
6. Write config files to `/opt/dockify/apps/app-{id}/{path}`
7. SSH → `docker compose up -d` (auto-detects `docker-compose` vs `docker compose`)
8. Inject Caddy route via Admin API (`POST /config/apps/http/servers/srv0/routes`)
9. If HTTP basic auth set: include bcrypt `basic_auth` handler in route
10. Create Cloudflare DNS A record (if configured, skips duplicates, upserts on IP change)
11. Save compose snapshot in deployment record (for rollback)
12. Record deployment with status, log, commit SHA (if Git-triggered)
13. Update app status → running

## Worker Init Flow

When a server is initialized (`POST /api/servers/:id/init`):

1. SSH connect + verify auth
2. Install Docker via `get.docker.com` (if not present)
3. Create `dockify` Docker network
4. Deploy Caddy container (ports 80/443 + Admin API on localhost:2019)
5. Collect CPU cores (`nproc`), RAM total (`free -m`), disk total (`df -BG`)
6. Collect CPU usage (`/proc/stat`), RAM usage (`free -m`), disk usage (`df -BG`)
7. Status → online

Init is idempotent — re-running skips components that already exist (Caddy container, Docker, network).

## Git Webhook Flow

1. Receive push event at `POST /api/webhook/github` or `/api/webhook/gitlab`
2. If webhook secret is enabled: validate HMAC-SHA256 (GitHub) or plain token (GitLab); reject 401 if mismatch
3. If not a push event → return 200 `"ignored"`
4. Parse: `ref` → branch, `after` → commit SHA, `clone_url`/`git_http_url` → repo URL
5. Find **all** matching apps by `git_repo` + `git_branch`
6. For each match: trigger `deployWithCommit(app.ID, commitSHA)` — same deploy flow as UI
7. Record each deployment with commit SHA

## Backup & Restore Flow

### Export

1. Query all servers (name, host, port, user — **no SSH keys**)
2. Query all apps (name, domain, port, compose, git_repo, git_branch, auth_user, auth_pass, compose_mode, server name mapping)
3. Query all app secrets and config files
4. If passphrase provided: encrypt secrets, auth_pass, and file contents with AES-GCM (PBKDF2 key derivation, 600,000 iterations, `enc:` prefix)
5. Generate YAML document
6. Download as `dockify-config.yaml`

### Import

1. Parse YAML file
2. If passphrase provided: attempt decryption of all `enc:` values to validate passphrase
3. In **merge** mode (default): skip entries that already exist by same name
4. In **replace** mode: delete all existing servers and apps, then import
5. Create servers (user must re-enter SSH keys after import)
6. Create apps (status = created, ready for deploy)
7. Import secrets and config files
8. Redirect to servers page

## Auto-Scheduler Logic

When no server is specified for deployment (`server_id=0`):

1. Query all online servers
2. Calculate load score for each: `score = (cpu_usage × 0.5) + (ram_usage × 0.5)`
3. Pick the server with the lowest score
4. Return error if no online servers available

Resource metrics are refreshed every 60 seconds by the background monitor goroutine.

## Resource Monitoring

The `Monitor.Run()` goroutine runs every 60 seconds:

- Queries all non-pending servers
- SSH exec to collect:
  - **CPU cores:** `nproc`
  - **CPU usage:** parse `/proc/stat` — `(user+system) × 100 / (user+system+idle)` — 5s sample interval
  - **RAM total:** `free -m` → Mem total
  - **RAM usage:** `free -m` → `(used−buffers−cache) / total × 100`
  - **Disk total:** `df -BG /` → Size
  - **Disk usage:** `df -BG /` → Use% as-is
- Updates server record with metrics + timestamp
- Human-readable display on resource card (e.g. "3200 MB used, 4800 MB free")

Server resource card supports HTMX partial refresh (`GET /servers/:id/resources`).

## UI Style Guide

All styles are in `internal/http/templates/layout.html` as a single `<style>` block. No CSS framework — fully custom.

### Typography
- **Font stack:** `"Berkeley Mono", "IBM Plex Mono", ui-monospace, "SF Mono", "Cascadia Code", "Fira Code", monospace`
- **Base size:** 15px, line-height: 1.5
- **Headings:** weight 600; h1 = 1.35em, h2 = 1.1em

### Color System

| Token | Light (`:root`) | Dark (`html.dark`) | Usage |
|---|---|---|---|
| `--bg` | `#f5f5f5` | `#0d0d0d` | Page background |
| `--bg-elevated` | `#fff` | `#141414` | Raised surface |
| `--bg-card` | `#fff` | `#1a1a1a` | Card background |
| `--border` | `#d4d4d4` | `#2a2a2a` | Borders |
| `--text` | `#1a1a1a` | `#cfcecd` | Body text |
| `--text-muted` | `#666` | `#888` | Secondary text |
| `--link` | `#2563eb` | `#8ab4f8` | Links |
| `--green` | `#2d7a3a` | `#3d8b4a` | Success / online / running |
| `--red` | `#9a2a2a` | `#b33a3a` | Error / offline / failed |
| `--yellow` | `#8a7420` | `#b8942e` | Warning / pending |
| `--orange` | `#8a6020` | `#b8702e` | Info / deploying |

### Dark/Light Mode
- Default: light mode (`<html>` without class)
- Dark: `<html class="dark">`
- Toggle button in nav uses `localStorage('dockify-theme')`, defaults to OS preference
- Icons: ☀ (light mode), ☾ (dark mode)

### Component Patterns

**Nav:** `<div class="top-nav">` — `.logo` (bold), `.nav-links` (flex). Compact, bottom border separator.

**Cards:** `<div class="card">` — `border: 1px solid var(--border)`, `border-radius: 6px`, `background: var(--bg-card)`. Card title: `<h3>` with uppercase, muted, letter-spaced.

**Buttons:** `<button class="btn">` or `<a class="btn">`
- `btn-primary` — filled accent, white text
- `btn-ghost` — transparent, muted text
- `btn-red` — red text, red border on hover (danger actions)

**Tables:** `<table>` — `<thead>` (uppercase th), row hover: subtle background.

**Forms:** `<form method="POST" action="relative-path">` — never `hx-boost` for forms.
- Labels wrap inputs: `<label>Text<input></label>`
- Grid layout: `<div class="grid">` for 2-column form rows
- Form grid adds 0.7em margin-bottom automatically
- Inline labels inside grids: `margin-bottom: 0`
- Legend: uppercase, muted, no border-bottom
- Radio: `display: inline-flex; align-items: center` on `<label>`, `input[type=radio]` has `margin: 0 .7em 0 0`
- Error messages: `<div class="card" style="color: var(--red)">`

**Badges:** `<span class="badge badge-{status}">`
- Server statuses: `online`, `offline`, `pending`, `error`, `initializing`
- App statuses: `running`, `stopped`, `created`, `deploying`, `failed`, `success`

**Breadcrumb:** `<div class="breadcrumb">` — links separated by `/`.

**Empty states:** `<div class="empty-state">` — centered text + primary CTA button.

**Logs viewer:** `<div class="log-container">` — `.log-toolbar` (tail 50/200/500 buttons) + `.log-content` (`<pre>` monospace). Lazy-load via HTMX (`hx-get` / `hx-target` / `hx-swap`).

**Stats dashboard:** `.stat` cards in `.grid` — number in `<h2>`, label in `<small>`.

**Confirm dialogs:** Destructive actions (undeploy, delete server, rollback) use `onclick="return confirm(...)"`.

### CSS Conventions
- Single `<style>` block in `layout.html` — no external CSS file
- Class naming: lowercase with hyphens (e.g. `.nav-links`, `.btn-primary`)
- CSS custom properties for all colors, no hardcoded values
- Responsive: single `@media (max-width: 600px)` breakpoint — grid collapses to 1 column

## Security Considerations

- Worker Caddy Admin API bound to `127.0.0.1:2019` — no external access
- Controller ↔ Worker communication only via SSH (encrypted)
- SSH private keys stored in `DOCKIFY_DATA_DIR/keys/` with `0600` permissions
- Cloudflare API token requires minimal scope: Zone:DNS:Edit
- App containers on internal `dockify` Docker network only — no public port exposure except via Caddy
- Webhook authentication: HMAC-SHA256 (GitHub), plain token (GitLab), auto-generated 64-char hex secret
- Session auth: cookie-based (`dockify_session`), 24-hour expiry, in-memory sessions
- HTTP Basic Auth for apps enforced at Caddy proxy level with bcrypt
- Backup encryption: AES-GCM with PBKDF2 passphrase (600,000 iterations, 256-bit key)
- Caddy auto-obtains Let's Encrypt certificates for all domains
