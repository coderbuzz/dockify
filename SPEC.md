# Dockify Build Spec

## Tech Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.23+ | Single binary, fast, great stdlib |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGo, embedded |
| Web UI | Go html/template + HTMX + Pico CSS | No JS framework, no build step |
| SSH | `golang.org/x/crypto/ssh` | Std SSH client |
| HTTP | `net/http` + `gorilla/mux` or `chi` | Standard router |
| Static embedding | `embed` (Go 1.16+) | Embed web UI into binary |

## Database Schema (SQLite)

```sql
-- VMs registered in the pool
CREATE TABLE servers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    host        TEXT NOT NULL,           -- IP or hostname
    port        INTEGER DEFAULT 22,
    user        TEXT DEFAULT 'root',
    ssh_key     TEXT NOT NULL,           -- path to SSH private key
    status      TEXT DEFAULT 'pending',  -- pending, online, offline
    cpu_cores   INTEGER,
    ram_mb      INTEGER,
    disk_gb     INTEGER,
    cpu_usage   REAL,                    -- 0.0 - 100.0
    ram_usage   REAL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Apps (Docker Compose stacks)
CREATE TABLE apps (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    server_id   INTEGER REFERENCES servers(id),
    domain      TEXT NOT NULL,
    port        INTEGER NOT NULL,        -- internal container port
    compose     TEXT NOT NULL,           -- docker-compose.yml content
    git_repo    TEXT,                    -- optional: git repo URL
    git_branch  TEXT DEFAULT 'main',
    status      TEXT DEFAULT 'created',  -- created, deploying, running, stopped, failed
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Deployment history
CREATE TABLE deployments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id      INTEGER REFERENCES apps(id),
    server_id   INTEGER REFERENCES servers(id),
    status      TEXT,                    -- success, failed
    log         TEXT,
    commit_sha  TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Caddy routes (synced state)
CREATE TABLE routes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id      INTEGER REFERENCES apps(id),
    server_id   INTEGER REFERENCES servers(id),
    domain      TEXT NOT NULL,
    target      TEXT NOT NULL,           -- container_name:port
    status      TEXT DEFAULT 'active',   -- active, removed
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Cloudflare DNS records
CREATE TABLE dns_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id      INTEGER REFERENCES apps(id),
    server_id   INTEGER REFERENCES servers(id),
    zone_id     TEXT NOT NULL,
    record_id   TEXT NOT NULL,
    name        TEXT NOT NULL,           -- subdomain
    type        TEXT DEFAULT 'A',
    content     TEXT NOT NULL,           -- IP address
    proxied     INTEGER DEFAULT 0,       -- CF proxy (orange cloud)
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Global settings
CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## API Design

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
POST   /api/apps                 Create and deploy app
GET    /api/apps/:id             Get app details + status
DELETE /api/apps/:id             Undeploy and remove app
POST   /api/apps/:id/redeploy    Redeploy app
GET    /api/apps/:id/logs        Stream app logs
```

### Deployments

```
GET    /api/apps/:id/deployments     Deployment history for app
GET    /api/deployments/:id          Deployment detail + log
```

### Webhooks

```
POST   /api/webhook/github      GitHub webhook receiver
POST   /api/webhook/gitlab      GitLab webhook receiver
```

### Settings

```
GET    /api/settings            Get all settings
PUT    /api/settings/:key       Update setting
```

## Worker Init Flow

When a new VM is registered, `POST /api/servers/:id/init` triggers:

```
1. SSH connect → verify auth
2. Check OS (Ubuntu/Debian)
3. Install Docker:
   curl -fsSL https://get.docker.com | sh
4. Create Docker network 'dockify':
   docker network create dockify
5. Deploy Caddy container:
   docker run -d --name caddy --network dockify \
     -p 80:80 -p 443:443 -p 127.0.0.1:2019:2019 \
     -v caddy_data:/data caddy:latest
6. Report worker status back to controller
```

## Deploy Flow

When an app is deployed (`POST /api/apps`):

```
1. Validate docker-compose.yml
2. Select server (manual or auto-scheduler)
3. SSH to server → write docker-compose.yml to /opt/dockify/apps/:name/
4. SSH exec: docker compose -f /opt/dockify/apps/:name/docker-compose.yml up -d
5. Inject Caddy route via Admin API:
   POST http://localhost:2019/config/apps/http/servers/srv0/routes
   {
     "match": [{"host": ["app.example.com"]}],
     "handle": [{
       "handler": "reverse_proxy",
       "upstreams": [{"dial": "container_name:3000"}]
     }]
   }
6. Create Cloudflare DNS A record pointing to server IP
7. Record deployment in history
8. Update app status
```

## Git Webhook Flow

```
1. Receive webhook POST (GitHub/GitLab)
2. Parse payload → extract repo URL, branch, commit SHA
3. Find matching app by git_repo + git_branch
4. If found → trigger redeploy
5. Record deployment with commit SHA
```

## Auto-Scheduler Logic

When no server is specified, pick the best VM:

```
1. Query all online servers
2. Calculate load score for each:
   score = (cpu_usage * 0.5) + (ram_usage * 0.5)
3. Pick server with lowest score
4. Return server ID
```

CPU/RAM usage is refreshed every 60 seconds via SSH remote exec.

## Web UI Routes

```
GET  /                         Dashboard (VM + app status)
GET  /servers                  Server list
GET  /servers/add              Add server form
GET  /servers/:id              Server detail
GET  /apps                     App list
GET  /apps/add                 New app form
GET  /apps/:id                 App detail + logs + deployments
GET  /settings                 Settings page
```

UI uses HTMX for partial page updates and Pico CSS for styling. All templates are embedded in the Go binary.

## Configuration

Dockify is configured via environment variables or a `.env` file:

```env
# Required
DOCKIFY_HOST=0.0.0.0
DOCKIFY_PORT=8080
DOCKIFY_DATA_DIR=/var/lib/dockify     # SQLite + app data

# Cloudflare (optional, for DNS automation)
CLOUDFLARE_API_TOKEN=xxx
CLOUDFLARE_ZONE_ID=xxx

# Auth
DOCKIFY_ADMIN_USER=admin
DOCKIFY_ADMIN_PASSWORD=xxx

# SSH
DOCKIFY_SSH_KEY_DIR=/var/lib/dockify/keys
```

## Build Phases

### Phase 1: Core — VM Management + SSH (3-5 days)
- [ ] Project scaffold + SQLite schema
- [ ] SSH client: connect, exec, transfer files
- [ ] VM CRUD API: add/list/get/delete
- [ ] Worker init: install Docker + Caddy
- [ ] VM resource monitoring (CPU/RAM via SSH)
- [ ] Basic Web UI: server list + add form

### Phase 2: App Deployment (3-4 days)
- [ ] App CRUD API: create/deploy/list/get/delete
- [ ] Docker Compose deploy: write file → compose up
- [ ] Caddy Admin API: inject/remove routes
- [ ] App status tracking + health check
- [ ] Web UI: app list + deploy form

### Phase 3: DNS + HTTPS (1-2 days)
- [ ] Cloudflare API client: DNS records CRUD
- [ ] Auto-create A record on deploy
- [ ] Caddy auto Let's Encrypt (zero config)
- [ ] DNS record cleanup on app delete

### Phase 4: CI/CD + Polish (2-3 days)
- [ ] Git webhook handler (GitHub + GitLab)
- [ ] Auto-deploy on push
- [ ] Deployment history
- [ ] Log viewer (SSH docker compose logs)
- [ ] CLI tool (`dockify` commands)

### Phase 5: Advanced (2-3 days)
- [ ] Auto-scheduler (least-loaded VM)
- [ ] Rollback support
- [ ] Health check + auto-recover
- [ ] Dashboard metrics
- [ ] Install script + docs

**Total: ~11-17 days for complete MVP.**

## Worker Caddy Configuration

Caddy runs as a Docker container with Admin API accessible via localhost:

```dockerfile
# Caddy on worker VM
docker run -d \
  --name caddy \
  --network dockify \
  -p 80:80 \
  -p 443:443 \
  -p 127.0.0.1:2019:2019 \
  -v caddy_data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  caddy:latest
```

Route injection via Admin API does NOT require Caddy restart. Caddy auto-obtains Let's Encrypt certificates for new domains.

## Security Considerations

- Worker Caddy Admin API bound to `127.0.0.1:2019` only (no external access)
- Controller ↔ Worker communication via SSH only (encrypted)
- Private keys stored in `DOCKIFY_DATA_DIR/keys/` with `0600` permissions
- Cloudflare API token has minimal scope (Zone:DNS:Edit)
- App containers on internal Docker network only (no port exposure unless via Caddy)
- Database engines (MongoDB, PostgreSQL) deploy without public port mapping
- Admin UI protected by basic auth (future: OIDC/OAuth)
