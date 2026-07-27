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
    stats.go                     # Stats collector: container stats, Caddy traffic, history
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
    stats_ws.go                  # WebSocket real-time resource stats (1s interval)
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
  setup-worker.sh                # Generate SSH key on worker, install Docker, output private key
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
| GET | `/api/servers/:id/stats/live` | WebSocket real-time resource stats (1s interval) |

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
3. Install Docker Compose plugin (if not present)
4. Create `dockify` Docker network
5. Deploy Caddy container (ports 80/443 + Admin API on localhost:2019)
6. Collect CPU cores (`nproc`), RAM total (`/proc/meminfo`), disk total (`df -BG`)
7. Collect CPU usage (`/proc/stat`), RAM usage (`/proc/meminfo`), disk usage (`df -BG`)
8. Status → online

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

1. Query all servers (name, host, port, user, SSH key)
2. Query all apps (name, domain, port, compose, git_repo, git_branch, auth_user, auth_pass, compose_mode, server name mapping)
3. Query all app secrets and config files
4. If passphrase provided: encrypt SSH keys, secrets, auth_pass, and file contents with AES-GCM (PBKDF2 key derivation, 600,000 iterations, `enc:` prefix)
5. Generate YAML document
6. Download as `dockify-config.yaml`

### Import

1. Parse YAML file
2. If passphrase provided: attempt decryption of all `enc:` values to validate passphrase (including SSH keys)
3. In **merge** mode (default): skip entries that already exist by same name
4. In **replace** mode: delete all existing servers and apps, then import
5. Create servers with SSH key from export (fallback to `"pending"` if key is empty — user must enter key manually)
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

There are two monitoring paths:

### Background Stats Collector (every 60s)

The `statsLoop()` goroutine runs every 60 seconds:

- Queries all non-pending servers
- SSH exec to collect:
  - **CPU cores:** `nproc`
  - **CPU usage:** two-sample `/proc/stat` — `(delta_total - delta_idle) / delta_total × 100` — 1s sample interval
  - **RAM total:** `awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo`
  - **RAM usage:** `awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} END{printf "%.1f", 100*(t-a)/t}' /proc/meminfo`
  - **Disk total:** `df -BG /` → Size
  - **Disk usage:** `df -BG /` → Use% as-is
- Updates server record with metrics + timestamp
- Human-readable display on resource card (e.g. "3200 MB used, 4800 MB free")

### Real-time WebSocket Stats (1s interval)

`GET /api/servers/:id/stats/live` — WebSocket endpoint that streams live CPU/RAM/disk metrics every 1 second. Uses the same `/proc/stat` and `/proc/meminfo` collection methods as the background collector. Client-side JS reconnects on disconnect.

## UI Style Guide

All styles live in `internal/http/templates/layout.html` as a single `<style>` block. No external CSS framework — fully custom Vanilla CSS.

### Typography
- **Primary UI Font Stack:** `'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif` (body, headers, navigation, buttons, labels)
- **Monospace Font Stack:** `'JetBrains Mono', 'Fira Code', ui-monospace, 'SF Mono', 'Cascadia Code', monospace` (IPs, ports, domains, badges, terminal, logs, secrets, code)
- **Base size:** 14px, line-height: 1.5
- **Headings:** weight 600; h1 = 1.35em, h2 = 1.1em

### Color System & Design Tokens

| Token | Light (`:root`) | Dark (`html.dark`) | Usage |
|---|---|---|---|
| `--bg` | `#f8fafc` | `#090d16` | App background |
| `--bg-elevated` | `#ffffff` | `#0f172a` | Raised surface |
| `--bg-card` | `#ffffff` | `#1e293b` | Card background |
| `--border` | `#e2e8f0` | `rgba(255, 255, 255, 0.08)` | Subtle borders |
| `--border-hover` | `#cbd5e1` | `rgba(255, 255, 255, 0.18)` | Interactive borders |
| `--text` | `#0f172a` | `#f8fafc` | Primary text |
| `--text-muted` | `#475569` | `#94a3b8` | Muted secondary text |
| `--text-dim` | `#94a3b8` | `#64748b` | Dimmed tertiary text |
| `--accent` | `#4f46e5` | `#6366f1` | Primary brand accent (Indigo) |
| `--accent-hover` | `#4338ca` | `#818cf8` | Hover accent |
| `--green` | `#10b981` | `#34d399` | Success / online / running |
| `--red` | `#ef4444` | `#f87171` | Error / offline / failed |
| `--yellow` | `#f59e0b` | `#fbbf24` | Warning / pending |
| `--orange` | `#f97316` | `#fb923c` | Info / deploying |
| `--blue` | `#3b82f6` | `#60a5fa` | Links / neutral interactive |
| `--shadow-sm` | `0 1px 2px 0 rgba(0, 0, 0, 0.05)` | `0 1px 2px 0 rgba(0, 0, 0, 0.3)` | Subtle card elevation |
| `--shadow-md` | `0 4px 6px -1px rgba(0,0,0,0.07)` | `0 4px 6px -1px rgba(0,0,0,0.4)` | Card hover / dropdown elevation |

### Dark/Light Mode
- Default: light mode (`<html>` without class)
- Dark: `<html class="dark">`
- Toggle button in nav uses `localStorage('dockify-theme')`, defaults to OS preference
- Icons: ☀ (light mode), ☾ (dark mode)

### Component Patterns

**Nav:** `<div class="top-nav">` — `.logo` (bold), `.nav-links` (flex). Sticky top bar with glassmorphism backdrop (`backdrop-filter: blur(12px)`). Includes a responsive mobile drawer navigation for viewports under 680px.

**Cards:** `<div class="card">` — `border: 1px solid var(--border)`, `border-radius: 8px`, `background: var(--bg-card)`, `box-shadow: var(--shadow-sm)`. Includes smooth hover micro-transitions (`transform: translateY(-1px)`).

**Buttons:** `<button class="btn">` or `<a class="btn">`
- `btn-primary` — filled indigo accent, white text
- `btn-ghost` — transparent, muted text with hover transition
- `btn-red` — red text, subtle red border hover
- `btn-danger` — filled red background for destructive actions

**Tables:** `<table>` — `<thead>` (uppercase th, letter-spacing), rounded container wrapping, row hover highlights.

**Badges:** `<span class="badge badge-{status}">` — Status pill badges with optional pulsing indicator dot (`<span class="badge-dot"></span>`) for live online/running/deploying states.

**Logs & Terminal:** Terminal and log container styled with dark glass topbar, macOS-style window controls, and crisp monospace log streaming.

### CSS Conventions
- CSS custom properties for all design tokens (no hardcoded color values)
- Fully responsive across desktop, tablet, and mobile viewports

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
