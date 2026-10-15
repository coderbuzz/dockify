# Dockify Build Spec

## Project Overview

Dockify is a self-hosted Docker app deployment platform — single Go binary, SQLite database, SSH-based worker management, embedded Web UI (Go `html/template` + HTMX), Caddy reverse proxy integration, and Cloudflare DNS automation.

## Tech Stack

| Layer | Choice | Details |
|---|---|---|
| Language | Go 1.26+ | Single binary, cross-compile |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGo, embedded |
| Router | `github.com/go-chi/chi/v5` | Middleware, groups, path params |
| SSH | `golang.org/x/crypto/ssh` | Std SSH client, PTY sessions |
| Web UI | Go `html/template` + HTMX | Embedded via `embed`, no JS framework, no build step |
| CSS | Fully custom (no framework) | CSS variables, dark/light mode, single `<style>` block |
| Terminal | xterm.js + WebSocket | Interactive SSH console in browser |
| Docker | Multi-stage build | Published to `ghcr.io/coderbuzz/dockify` |

## Project Structure

```
cmd/dockify/main.go              # Entry point: config → db → services → router, HTTP server
internal/
  config/config.go               # Environment variable config
  db/db.go + schema.sql          # SQLite (one connection, WAL, foreign_keys=ON), 12 tables; ad-hoc migrations run on every start
  model/model.go                 # Shared view types (AppSummary, ChartPoint)
  ssh/
    interface.go                 # Connector interface + Factory (a new SSH connection per remote action)
    client.go                    # SSH client: exec wrapped in `timeout -k 10 N sh -c`, pipes, stdin file writes, PTY, TOFU host keys, key generation
    mock.go                      # Mock SSH for dev mode (DOCKIFY_DEV_MOCK=true)
  server/
    model.go                     # Server, ServerStats
    repository.go                # servers + server_stats queries
    service.go                   # Server CRUD, worker init and prune, resource refresh, Monitor (every 10s)
    handler.go                   # JSON API handlers + web UI handlers (pages, forms, HTMX cards)
  app/
    model.go                     # App, Deployment, stats types
    repository.go                # apps, deployments, routes, secrets, files and stats queries
    service.go                   # Deploy/undeploy/redeploy/rollback/stop/start, move between workers, Caddy route + DNS per domain
    compose.go                   # Simple-mode compose generation, dockify network injection, service rename
    stats.go                     # Stats collector (containers 10s, Caddy traffic 30s, disk 5 min, prune hourly) + history
    handler.go                   # JSON API + web UI handlers (apps, deployments, secrets, files, logs, stats)
  scheduler/scheduler.go         # Auto-select least-loaded server
  caddy/client.go                # Caddy Admin API client: route CRUD and config save via `docker exec caddy curl` over SSH
  cloudflare/client.go           # Cloudflare DNS API v4 (list, create, upsert records)
  webhook/handler.go             # GitHub + GitLab webhook receiver, HMAC/token validation
  settings/
    settings.go                  # Webhook secret, GitHub Releases update check, systemd-run self-update
    handler.go                   # Settings page, webhook secret get/roll, update check/run
  backup/
    backup.go                    # YAML export/import, AES-GCM encryption with PBKDF2 passphrase
    handler.go                   # Export/import pages and endpoints
  http/
    router.go                    # Chi router, all routes, middleware
    auth.go                      # Session auth, login/logout, bearer token, cross-site request check
    console.go                   # WebSocket upgrade, PTY relay for SSH and container consoles
    stats_ws.go                  # WebSocket real-time server stats (1s interval)
    app_stats_ws.go              # WebSocket real-time app container stats
    templates.go                 # Embedded templates, template funcs, Render
    templates/                   # HTML templates (pages, layout, partials)
      layout.html                # head/nav/scripts blocks, all CSS (single <style> block)
      dashboard.html             # Dashboard with stats cards + server/app summary tables
      servers.html               # Server list page (servers_list.html: table partial)
      servers_add.html           # Add server form
      servers_detail.html        # Server detail: info, resources card, live charts, SSH console, actions
      servers_edit.html          # Edit server form
      servers_resources_card.html# HTMX partial: resource usage with human-readable values
      servers_stats_card.html    # HTMX partial: server stats
      apps.html                  # App list page (apps_list.html: table partial)
      apps_add.html              # Deploy app form, also used for edit (simple/advanced toggle)
      apps_detail.html           # App detail: info, compose, env vars, files, logs, deployments, live charts, console
      apps_stats_card.html       # HTMX partial: app stats
      live_charts.html           # Partial: live chart script (included by layout)
      settings.html              # Settings: webhook secret, export/import links
      about.html                 # About page: version, update check/run, sponsor link
      export.html                # Export page: passphrase generator, download YAML
      import.html                # Import page: file upload, mode selector (merge/replace)
      login.html                 # Login page
      error.html                 # Error page
scripts/
  install.sh                     # One-liner install (3 modes: Docker Compose / Binary / Binary + Caddy)
  setup-worker.sh                # setup-worker.sh '<public key>': authorize the controller key, install Docker, prepare /opt/dockify
  gen-ssh-key.sh                 # Dedicated SSH key for external/agent access to a VM
  update.sh                      # Auto-detect install mode, download & restart latest
  update-latest.sh               # Same, run directly on the host
  release.sh                     # Version bump + tag helper
  smoke-test.sh                  # Build, start, probe /health, /login and the auth redirect
Dockerfile                       # Multi-stage Docker build
docker-compose.yml               # Dockify + Caddy reverse proxy (mode 1)
.air.toml                        # Air live-reload config
.github/workflows/build.yml      # CI: vet, test, smoke test, build binary + Docker, release on v* tag
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
| `DOCKIFY_ADMIN_PASSWORD` | — | Unless loopback | Web UI password. Not set = no authentication, and `serve` exits unless `DOCKIFY_HOST` is loopback or `DOCKIFY_ALLOW_NO_AUTH=true` |
| `DOCKIFY_ALLOW_NO_AUTH` | `false` | No | Allow a passwordless UI on a non-loopback bind |
| `DOCKIFY_API_TOKEN` | — | No | `Authorization: Bearer` token accepted on all protected routes (scripts/agents) |
| `DOCKIFY_DEV_MOCK` | `false` | No | Enable mock SSH client for local development |
| `DOCKIFY_BASE_PATH` | — | No | URL prefix when behind reverse proxy (e.g. `/proxy/9898`) |
| `CLOUDFLARE_API_TOKEN` | — | No | Cloudflare API token for DNS automation (Zone:DNS:Edit scope) |
| `CLOUDFLARE_ZONE_ID` | — | No | Cloudflare zone ID for the domain |

## Database Schema (12 Tables)

Foreign keys are enforced (`foreign_keys=ON`). `ON DELETE CASCADE` exists only on `app_secrets`, `app_files`, the stats tables' `app_id` and `server_stats.server_id`.

### servers

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `name` | TEXT NOT NULL | Display name |
| `host` | TEXT NOT NULL | IP or hostname |
| `port` | INTEGER DEFAULT 22 | SSH port |
| `user` | TEXT DEFAULT 'root' | SSH user |
| `ssh_key` | TEXT NOT NULL | Path to the private key file (`0600`, `$DOCKIFY_SSH_KEY_DIR/<id>.pem`) |
| `status` | TEXT DEFAULT 'pending' | pending, online, offline, error, initializing |
| `cpu_cores` | INTEGER | Total CPU cores |
| `ram_mb` | INTEGER | Total RAM in MB |
| `disk_gb` | INTEGER | Total disk in GB |
| `cpu_usage` | REAL | CPU usage % (0.0 - 100.0) |
| `ram_usage` | REAL | RAM usage % (0.0 - 100.0) |
| `disk_usage` | REAL | Disk usage % (0.0 - 100.0) |
| `resources_updated_at` | DATETIME | Last resource refresh timestamp |
| `registry_host` | TEXT DEFAULT '' | Registry for `docker login` (empty → `ghcr.io`) |
| `registry_user` | TEXT DEFAULT '' | Registry username (empty = no credential) |
| `registry_token` | TEXT DEFAULT '' | Path to registry token file (`0600`, `$DOCKIFY_SSH_KEY_DIR/<id>.registry-token`); never serialized to the API |
| `host_key` | TEXT DEFAULT '' | Pinned SSH host key (base64 wire format), learned on first connect; empty = not pinned yet; never serialized to the API |
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
| `auth_pass` | TEXT DEFAULT '' | HTTP basic auth password (plaintext; bcrypt-hashed when the Caddy route is built) |
| `status` | TEXT DEFAULT 'created' | draft, created, deploying, running, stopped, failed |
| `compose_mode` | TEXT DEFAULT 'advanced' | simple or advanced |
| `memory_limit`, `cpu_limit` | TEXT DEFAULT '' | Simple-mode resource limits |
| `log_max_size`, `log_max_file` | TEXT DEFAULT '' | Simple-mode json-file log rotation |
| `command` | TEXT DEFAULT '' | Simple-mode command override |
| `ports` | TEXT DEFAULT '' | Simple-mode published ports (one per line) |
| `ulimits_nofile` | TEXT DEFAULT '' | Simple-mode `nofile` ulimit |
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
| `is_secret` | INTEGER DEFAULT 0 | UI masks the value |
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

### Stats tables

Written by the background collectors, pruned after 7 days, `created_at` in UTC.

| Table | Columns | Written |
|---|---|---|
| `server_stats` | `server_id` (cascade), `cpu_percent`, `ram_percent`, `disk_percent`, `cpu_cores`, `ram_mb`, `disk_gb` | Monitor, every 10s per server |
| `container_stats` | `app_id` (cascade), `server_id`, `container_name`, `cpu_percent`, `mem_usage_bytes`, `mem_limit_bytes`, `mem_percent`, `net_io_rx_bytes`, `net_io_tx_bytes`, `block_io_read`, `block_io_write` | Stats collector, every 10s per container |
| `route_stats` | `app_id` (cascade), `domain`, `total_requests`, `requests_rps`, `status_2xx` … `status_5xx` | Caddy metrics, every 30s |
| `app_disk_stats` | `app_id` (cascade), `server_id`, `disk_usage_bytes` | `du` of the app folder, every 5 min |

## API Endpoints

### Public (no auth)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check → `ok` |
| GET/POST | `/login` | Login page / form submit |
| POST | `/logout` | Clear session, redirect to login |
| POST | `/api/webhook/github` | GitHub push webhook (HMAC-SHA256), body ≤ 1 MiB |
| POST | `/api/webhook/gitlab` | GitLab push webhook (token), body ≤ 1 MiB |
| GET | `/static/*` | Static files (`web/static/`) |

Protected routes accept the session cookie or `Authorization: Bearer $DOCKIFY_API_TOKEN`. A POST/PATCH/DELETE sent by another site (`Sec-Fetch-Site`, or `Origin` ≠ `Host` for older browsers) gets `403`; bearer-token requests are exempt. WebSocket handshakes must be same-origin. Request bodies under `/api/servers` and `/api/apps` are capped at 1 MiB.

### Protected — Server API (JSON)

| Method | Path | Description |
|---|---|---|
| GET | `/api/servers` | List servers |
| POST | `/api/servers` | Create server `{name, host, port?, user?, ssh_key?, registry_*?}`; without `ssh_key` an ed25519 key pair is generated on the controller (the worker is then not initialized automatically). Response includes `public_key` |
| GET | `/api/servers/:id` | Get server details, including `public_key` (authorized_keys line of the server's key) |
| PATCH | `/api/servers/:id` | Partial update (host, port, user, ssh_key, registry_host, registry_user, registry_token, registry_clear, host_key_clear); a host or port change also clears the pinned host key |
| DELETE | `/api/servers/:id` | Delete server |
| POST | `/api/servers/:id/init` | Initialize worker (Docker + Caddy) |
| POST | `/api/servers/:id/refresh` | Refresh resource metrics |
| POST | `/api/servers/:id/prune` | `docker system prune` on the worker `{all?, volumes?}` + builder prune |
| GET | `/api/servers/:id/console` | WebSocket upgrade → SSH terminal |
| GET | `/api/servers/:id/stats/history` | Stats history (`?range=realtime\|1h\|6h\|24h\|7d`, default `1h`) |
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
| POST | `/servers/:id/prune` | Prune worker (web form) |
| GET | `/servers/:id/resources` | Resource card (HTMX partial) |
| GET | `/servers/:id/stats` | Stats card (HTMX partial) |
| POST | `/servers/:id/refresh` | Refresh resources (web form) |
| POST | `/servers/:id/delete` | Delete server (web form) |
| DELETE | `/servers/:id` | Delete server |

### Protected — App API (JSON)

| Method | Path | Description |
|---|---|---|
| GET | `/api/apps` | List apps |
| POST | `/api/apps` | Create and deploy app |
| GET | `/api/apps/:id` | Get app details |
| PATCH | `/api/apps/:id` | Partial update `{compose?, server_id?, move_files?, purge_old?}`; never touches env vars, files or domains; unknown fields are rejected; redeploys (or moves) a non-draft app |
| DELETE | `/api/apps/:id` | Undeploy and remove app |
| POST | `/api/apps/:id/redeploy` | Redeploy app |
| POST | `/api/apps/:id/rollback` | Rollback to last successful deployment |
| POST | `/api/apps/:id/stop` | Stop app (docker compose stop + remove Caddy route) |
| POST | `/api/apps/:id/start` | Start app (docker compose start + add Caddy route + DNS) |
| GET | `/api/apps/:id/deployments` | List deployment history (last 20) |
| DELETE | `/api/apps/:id/deployments` | Clear deployment history |
| GET | `/api/apps/:id/logs` | Stream logs (`docker compose logs --tail=N`) |
| GET | `/api/apps/:id/secrets` | List secrets |
| POST | `/api/apps/:id/secrets` | Set env var `{key, value, is_secret?}`; `is_secret` omitted keeps the stored flag (new keys: not secret); key must match `[A-Za-z_][A-Za-z0-9_]*` |
| DELETE | `/api/apps/:id/secrets/:key` | Delete secret |
| GET | `/api/apps/:id/files` | List config files |
| POST | `/api/apps/:id/files` | Set config file `{path, content}`; path validated (see Input validation) |
| DELETE | `/api/apps/:id/files/:path` | Delete config file |
| GET | `/api/apps/:id/console` | WebSocket upgrade → shell in the app's first service container |
| GET | `/api/apps/:id/stats` | Latest container stats |
| GET | `/api/apps/:id/stats/history` | Container stats history (`?range=`, default `1h`) |
| GET | `/api/apps/:id/stats/live` | WebSocket real-time container stats |
| GET | `/api/apps/:id/traffic` | Latest Caddy traffic per domain |
| GET | `/api/apps/:id/traffic/history` | Traffic history (`?range=`, default `1h`) |
| GET | `/api/deployments/:id` | Get single deployment detail |

### Protected — App Web UI

| Method | Path | Description |
|---|---|---|
| GET | `/apps` | App list page |
| GET | `/apps/add` | Deploy app form |
| POST | `/apps/add` | Submit deploy app form |
| POST | `/apps/redeploy-all` | Redeploy every running app |
| GET | `/apps/:id` | App detail page |
| POST/DELETE | `/apps/:id/undeploy` | Undeploy app (web form) |
| POST | `/apps/:id/deploy` | Deploy a draft app (web form) |
| POST | `/apps/:id/clear-deployments` | Clear deployment history (web form) |
| POST | `/apps/:id/redeploy` | Redeploy app (web form) |
| POST | `/apps/:id/rollback` | Rollback app (web form) |
| POST | `/apps/:id/stop` | Stop app (web form) |
| POST | `/apps/:id/start` | Start app (web form) |
| GET | `/apps/:id/edit` | Edit app form page |
| POST | `/apps/:id/edit` | Submit edit app form |
| GET | `/apps/:id/stats` | Stats card (HTMX partial) |

### Protected — Settings, Backup, Pages

| Method | Path | Description |
|---|---|---|
| GET | `/` | Dashboard: stats, servers table, apps table |
| GET | `/settings` | Settings page |
| GET | `/about` | About page (version, sponsor link) |
| GET | `/export` | Export page (passphrase generator, download) |
| GET | `/import` | Import page (file upload, merge/replace) |
| GET | `/api/settings/webhook-secret` | Get webhook secret |
| POST | `/api/settings/webhook-secret/roll` | Regenerate webhook secret (it cannot be disabled) |
| GET | `/api/settings/update/check` | Check latest GitHub Release version (`?force=1` skips the 5-minute cache) |
| POST | `/api/settings/update/run` | Trigger self-update via systemd-run |
| POST | `/api/backup/export` | Download YAML export |
| POST | `/api/backup/import` | Upload YAML import (multipart: `file` + `mode`), ≤ 16 MiB |

## Deploy Flow

When an app is deployed (create/redeploy):

1. Parse and validate compose (simple mode: generate compose from image + env vars + volumes)
2. Select target server (manual or auto-scheduler — `server_id=0` = auto-select)
3. Ensure `dockify` Docker network exists on worker
4. SSH → write `docker-compose.yml` to `/opt/dockify/apps/app-{id}/`
5. Write app secrets as `.env` file to `/opt/dockify/apps/app-{id}/.env` (mode `0600`, written on every deploy, empty when the app has no variables, so removed variables disappear). One `KEY="value"` line per variable: the value is trimmed, then `\` → `\\`, `"` → `\"`, newline → `\n`, CR → `\r`, `$` → `$$`. Config files removed in the UI are not deleted from the worker
6. Write config files to `/opt/dockify/apps/app-{id}/{path}`
7. If the server has a registry credential: SSH → `docker login <host> -u <user> --password-stdin` with the token on stdin; on failure record `registry login failed: …` (token redacted) and stop
8. SSH → `docker compose pull`, then `docker compose up -d` (auto-detects `docker-compose` vs `docker compose`)
9. Inject Caddy route via Admin API (`POST /config/apps/http/servers/srv0/routes`)
10. If HTTP basic auth set: include bcrypt `basic_auth` handler in route
11. Persist Caddy's running config to the file named by its `--config` argument (read with `docker inspect caddy`: `/etc/caddy/config.json` on workers set up by Init, `/data/config.json` on older ones) so routes survive a Caddy restart
12. Create Cloudflare DNS A record (if configured, skips duplicates, upserts on IP change)
13. Save compose snapshot in deployment record (for rollback)
14. Record deployment with status, log, commit SHA (if Git-triggered)
15. Update app status → running

Undeploy runs `docker compose down --volumes --rmi all` for the app only; worker-wide pruning happens only through the Prune action.

## Worker Setup

1. Add the server (UI or `POST /api/servers`) without a key: Dockify generates an ed25519 key pair, stores the private key as `<id>.pem` (`0600`) and shows the public key with the setup command on the server page. Pasting an existing private key remains possible (advanced).
2. On the worker, as the user Dockify connects as: `curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/setup-worker.sh | bash -s -- '<public key>'`. The script appends the key to `~/.ssh/authorized_keys` (`600`, no duplicates), installs Docker and the compose plugin, prepares `/opt/dockify` and prints the worker IP and user. Without an argument it prints usage and exits 1.
3. Click **Init Worker**.

### SSH host keys (trust on first use)

Every SSH connection (deploys, monitor, console, live stats) goes through one factory that checks the worker's host key against `servers.host_key` (looked up by host and port). The first connection stores the presented key and logs `pinned host key for host:port (SHA256:…)`; later connections fail with `host key mismatch for host:port` if the key differs. The server page shows the fingerprint (or "not pinned yet"). After reinstalling a worker, tick **Forget pinned host key** on the edit form or send `host_key_clear: true`; changing host or port clears it too.

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
2. If not a push event → return 200 `"ignored"`
3. Validate HMAC-SHA256 (GitHub) or the token (GitLab, constant-time) against the global secret; reject 401 on mismatch or when the secret cannot be read
4. Parse: `ref` → branch, `after` → commit SHA, `clone_url`/`git_http_url` → repo URL
5. Find **all** matching apps by `git_repo` + `git_branch`
6. For each match: trigger `deployWithCommit(app.ID, commitSHA)` — same deploy flow as UI
7. Record each deployment with commit SHA

## Backup & Restore Flow

### Export

1. Query all servers (name, host, port, user, SSH key, registry host/user/token)
2. Query all apps (name, domain, port, compose, git_repo, git_branch, auth_user, auth_pass, compose_mode, server name mapping)
3. Query all app secrets and config files
4. If passphrase provided: encrypt SSH keys, registry tokens, secrets, auth_pass, and file contents with AES-GCM (PBKDF2 key derivation, 600,000 iterations, `enc:` prefix)
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

Resource metrics are refreshed every 10 seconds by the background monitor goroutine.

## Resource Monitoring

There are three monitoring paths:

### Background Server Monitor (every 10s)

`server.Monitor` runs every 10 seconds (and prunes `server_stats` hourly):

- Queries all online/offline servers and refreshes each in its own goroutine (new SSH connection per server per tick)
- SSH exec to collect:
  - **CPU cores:** `nproc`
  - **CPU usage:** two-sample `/proc/stat` — `(delta_total - delta_idle) / delta_total × 100` — 1s sample interval
  - **RAM total:** `awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo`
  - **RAM usage:** `awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} END{printf "%.1f", 100*(t-a)/t}' /proc/meminfo`
  - **Disk total:** `df -BG /` → Size
  - **Disk usage:** `df -BG /` → Use% as-is
- Updates server record with metrics + timestamp and inserts a `server_stats` row
- Human-readable display on resource card (e.g. "3200 MB used, 4800 MB free")

### App Stats Collector

`app.Service.statsLoop()`: container stats every 10s (one `docker ps` + one `docker stats --no-stream` per worker), Caddy traffic from `/metrics` every 30s, app folder disk usage every 5 min, prune of rows older than 7 days hourly.

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
- SSH private keys stored in `DOCKIFY_DATA_DIR/keys/` with `0600` permissions; generated on the controller, only the public key is shown or returned by the API
- SSH host keys pinned on first use (see Worker Setup)
- Files are written to workers by piping the content to `cat` over SSH stdin with shell-quoted paths; no user content is parsed by the shell
- Input validation at the trust boundary (API, web forms, backup import; 400 or a rejected import naming the value): domains are lowercased and trimmed, then must match `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`; config file paths are cleaned and must be relative, without `..`, made of `[A-Za-z0-9._/-]`; env keys must match `^[A-Za-z_][A-Za-z0-9_]*$`. The simple-mode compose is generated with a YAML encoder, not string concatenation
- Registry tokens stored the same way (`<id>.registry-token`, `0600`); DB holds only the path. Sent to `docker login --password-stdin` over SSH stdin (never in the command line), redacted from deploy logs and errors, never returned by the API
- Cloudflare API token requires minimal scope: Zone:DNS:Edit
- App containers on internal `dockify` Docker network only — no public port exposure except via Caddy
- Webhook authentication: HMAC-SHA256 (GitHub), token with constant-time compare (GitLab), auto-generated 64-char hex secret; always on (no disable), fails closed
- Session auth: cookie-based (`dockify_session`, `HttpOnly`, `SameSite=Lax`, `Secure` when the request is TLS or `X-Forwarded-Proto: https`), 24-hour expiry, in-memory sessions
- Cross-site requests: unsafe methods from another site get `403` (Go `http.CrossOriginProtection`), WebSocket handshakes must be same-origin, no CORS headers. A reverse proxy in front of Dockify must forward the original `Host` header (Caddy does by default)
- HTTP server: `ReadHeaderTimeout` 10s, `IdleTimeout` 120s; request bodies capped (API JSON 1 MiB, webhooks 1 MiB, import 16 MiB)
- HTTP Basic Auth for apps enforced at Caddy proxy level with bcrypt
- Backup encryption: AES-GCM with PBKDF2 passphrase (600,000 iterations, 256-bit key)
- Caddy auto-obtains Let's Encrypt certificates for all domains
