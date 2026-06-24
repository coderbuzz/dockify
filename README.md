# Dockify

Self-hosted Docker app deployment platform. Deploy Docker Compose stacks to your own VMs with auto HTTPS, Cloudflare DNS, and Git-based CI/CD — all from a single binary.

**Inspired by Coolify. Built for simplicity.**

## Why Dockify

- **Single binary** — Go + SQLite, no external dependencies
- **Caddy reverse proxy** — auto Let's Encrypt, no config needed
- **Cloudflare DNS** — auto-create DNS records on deploy
- **Git webhook** — push to main, auto deploy
- **VM pool** — add VMs, deploy to least-loaded VM
- **Web UI** — manage everything from the browser

## Architecture

```
Controller VM (Dockify binary)
  ├── Web UI + REST API (:8080)
  ├── SQLite (state)
  ├── SSH manager → workers
  ├── Caddy Admin API → inject routes
  ├── Cloudflare API → DNS
  └── Git webhook → auto deploy

Worker VM
  ├── Caddy (Docker, :80/:443 + Admin API :2019)
  ├── App containers (Docker Compose)
  └── Caddy routes to containers via Docker network
```

## Quick Start

### Option 1: Docker Compose (recommended)

```bash
git clone https://github.com/coderbuzz/dockify.git
cd dockify
cp .env.example .env
# Edit .env → set DOMAIN=your-domain.com
# Uncomment CLOUDFLARE_API_TOKEN + CLOUDFLARE_ZONE_ID for DNS automation
docker compose up -d
# Open https://your-domain.com (auto HTTPS via Caddy)
```

### Option 2: Docker

```bash
docker run -d \
  --name dockify \
  -p 8080:8080 \
  -v dockify_data:/var/lib/dockify \
  -v ~/.ssh:/home/dockify/.ssh:ro \
  -e CLOUDFLARE_API_TOKEN=xxx \
  -e CLOUDFLARE_ZONE_ID=xxx \
  ghcr.io/coderbuzz/dockify:latest
```

### Option 3: Install script

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash
```

### Option 4: Build from source

```bash
git clone https://github.com/coderbuzz/dockify.git
cd dockify
go build -o dockify ./cmd/dockify
./dockify serve
```

## Project Structure

```
dockify/
├── cmd/dockify/main.go
├── internal/
│   ├── ssh/           # SSH client, remote exec, worker init
│   ├── vm/            # VM CRUD, resource monitoring
│   ├── app/           # App CRUD, deployment
│   ├── caddy/         # Caddy Admin API client
│   ├── cloudflare/    # Cloudflare DNS API
│   ├── webhook/       # Git webhook handler
│   ├── scheduler/     # Idle VM selection
│   ├── db/            # SQLite layer
│   └── http/          # HTTP server, handlers, templates
├── web/static/        # CSS, JS (embedded)
├── scripts/           # Worker init scripts
└── docs/              # Documentation
```
