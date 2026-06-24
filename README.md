# Dockify

Self-hosted Docker app deployment platform. Deploy Docker Compose stacks to your own VMs with auto HTTPS, Cloudflare DNS, and Git-based CI/CD — all from a single binary.

**Inspired by Coolify. Built for simplicity.**

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)

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

### Step 1: Install Dockify on Controller VM

```bash
# Option A: Docker Compose (recommended)
git clone https://github.com/coderbuzz/dockify.git && cd dockify
cp .env.example .env   # edit DOMAIN + Cloudflare keys
docker compose up -d    # auto HTTPS via Caddy

# Option B: One-liner install script
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash

# Option C: Build from source
go build -o dockify ./cmd/dockify && ./dockify serve
```

Open `https://<your-domain>` or `http://<controller-ip>:8080`.

### Step 2: Prepare a Worker VM

Fresh Ubuntu/Debian VM. Zero dependencies needed — Dockify installs everything.

**Generate SSH key on the controller:**

```bash
ssh-keygen -t ed25519 -f ~/.ssh/dockify -N ""
```

**Copy the public key to the worker VM:**

```bash
ssh-copy-id -i ~/.ssh/dockify.pub root@<worker-ip>
```

> This appends the key to `/root/.ssh/authorized_keys` on the worker. From this point, the controller can SSH into the worker as root without a password.

### Step 3: Register + Initialize in Web UI

1. Go to **Servers** → **Add Server**
2. Fill in:
   - **Name:** `worker-01`
   - **Host:** `<worker-ip>`
   - **User:** `root`
   - **SSH Private Key Path:** `/home/user/.ssh/dockify` (path on controller)
3. Click **Add Server** → redirects to server detail
4. Click **Initialize Worker**

**What "Initialize Worker" does automatically:**
1. SSH connect + verify
2. Install Docker via `get.docker.com` (if not present)
3. Create `dockify` Docker network
4. Deploy Caddy container (port 80/443 + Admin API on localhost:2019)
5. Collect CPU, RAM, Disk info
6. Status → **online**. Ready to deploy apps.

### Step 4: Deploy Your First App

1. Go to **Apps** → **Deploy App**
2. Paste a `docker-compose.yml`, set domain + port
3. Select server (or **Auto-select** for least-loaded)
4. Click **Deploy App**

**What happens on deploy:**
1. SSH → write compose file to `/opt/dockify/apps/<name>/`
2. SSH → `docker compose up -d`
3. Inject Caddy route via Admin API (domain → container:port)
4. Create Cloudflare DNS A record (if configured)
5. Record deployment + save compose snapshot for rollback
6. Status → **running**

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

## License

MIT © 2025 CoderBuzz
