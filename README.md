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

Recommended OS: **Debian 12** (minimal, no GUI). All specs are **total VM size** (OS + Dockify).

| Mode | Min vCPU | Min RAM | Min Disk | RAM used | HTTPS | Docker? |
|---|---|---|---|---|---|---|
| **1: Docker Compose** | 1 | 1 GB | 10 GB | ~100 MB | Auto (Caddy container) | ✅ |
| **2: Binary only** | 1 | 512 MB | 10 GB | ~30 MB | Manual | ❌ |
| **3: Binary + Caddy** | 1 | 512 MB | 10 GB | ~40 MB | Auto (native Caddy) | ❌ |

**Option A: Binary + native Caddy (lightweight + auto HTTPS) — recommended**

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash
# Select mode 3 (Binary + Caddy)
sudo systemctl start dockify-caddy
```

Dockify + Caddy run as native binaries (no Docker). Caddy auto-obtains Let's Encrypt certificates and proxies to Dockify on `127.0.0.1:8080`.

**Option B: Binary only (lightest)**

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash
# Select mode 2 (Binary only)
sudo systemctl start dockify
```

No Caddy, no Docker. Access at `http://<ip>:8080`. Add a reverse proxy manually if needed.

**Option C: Docker Compose (bundled Caddy)**

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash
# Select mode 1 (Docker Compose) - default
cd /opt/dockify && docker compose up -d
```

**Option D: Build from source (development)**

```bash
git clone https://github.com/coderbuzz/dockify.git
cd dockify
go build -o dockify ./cmd/dockify
./dockify serve
```

**All installation methods share the same architecture for worker VMs:**
- Dockify connects to workers via SSH
- Installs Docker + creates the `dockify` Docker network on each worker
- Deploys Caddy on each worker (port 80/443 + Admin API on localhost:2019)
- Apps communicate with Caddy through the `dockify` network

The choice between binary and Docker only affects how you host the Dockify controller itself. Worker VMs are identical either way.

Open `https://<your-domain>` or `http://<controller-ip>:8080`.

### Environment Variables

Create a `.env` file in the project root or set these environment variables:

```env
# Domain for Caddy reverse proxy (auto HTTPS). Only needed for Mode 1 (Docker Compose).
DOMAIN=dockify.example.com

# Admin credentials (required for web UI login).
# If DOCKIFY_ADMIN_PASSWORD is not set, the web UI has no authentication.
DOCKIFY_ADMIN_USER=admin
DOCKIFY_ADMIN_PASSWORD=your-secure-password

# Cloudflare API credentials (optional, enables automatic DNS A record creation on deploy)
CLOUDFLARE_API_TOKEN=
CLOUDFLARE_ZONE_ID=

# Optional: base path when behind a reverse proxy (e.g., code-server: /proxy/9898)
DOCKIFY_BASE_PATH=
```

### Authentication

The web UI is protected by a login page. To enable authentication, set `DOCKIFY_ADMIN_PASSWORD` in your `.env` or environment:

```bash
# In .env
DOCKIFY_ADMIN_USER=admin
DOCKIFY_ADMIN_PASSWORD=secret123

# Or as env vars
DOCKIFY_ADMIN_PASSWORD=secret123 docker compose up -d
```

- **Default username:** `admin` (configurable via `DOCKIFY_ADMIN_USER`)
- **No password set:** Web UI has **no authentication** (open to anyone)
- **Password set:** All routes (except `/health` and `/api/webhook/*`) require login
- **Session:** Cookie-based, expires after 24 hours
- **Logout:** Visit `/logout`

The login page is at `/login`. After successful login, you are redirected to the dashboard. Webhook endpoints (`/api/webhook/github`, `/api/webhook/gitlab`) and health check (`/health`) do not require authentication.

Dockify runs with sensible defaults. Only `DOMAIN` is required for Option A. `CLOUDFLARE_*` is optional and only needed if you want automated DNS records. `DOCKIFY_BASE_PATH` is only needed when accessing Dockify through a URL prefix (e.g., code-server proxy).

### Step 2: Prepare a Worker VM

Fresh Ubuntu/Debian VM. Zero dependencies needed — Dockify installs everything.

**Option A: One-liner script (run on the worker VM)**

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/setup-worker.sh | bash
```

The script will:
1. Generate an SSH key pair at `/root/.ssh/dockify`
2. Add the public key to `/root/.ssh/authorized_keys`
3. Output the **private key content** — copy this

Then paste the private key into the Dockify Add Server form. No `ssh-copy-id` needed.

**Option B: Manual (generate elsewhere)**

Generate an SSH key pair anywhere (laptop, controller VM, etc.):

```bash
ssh-keygen -t ed25519 -f ~/.ssh/dockify -N ""
```

Copy the public key to the worker VM:

```bash
ssh-copy-id -i ~/.ssh/dockify.pub root@<worker-ip>
```

> The private key content (`cat ~/.ssh/dockify`) is then pasted into the Dockify web UI form — not a file path, but the key material itself.

### Step 3: Register + Initialize in Web UI

1. Go to **Servers** → **Add Server**
2. Fill in:
   - **Name:** `worker-01`
   - **Host:** `<worker-ip>`
   - **User:** `root`
   - **SSH Private Key:** Paste the content of `~/.ssh/dockify` (the private key file, `cat ~/.ssh/dockify`)
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
2. Choose **Simple Mode** (just an image name) or **Advanced Mode** (full `docker-compose.yml`)
3. Set domain, port, and select a server (or **Auto-select** for least-loaded)
4. Optional: fill **Basic Auth** username/password to protect the app behind HTTP basic auth
5. Click **Deploy App**

**What happens on deploy:**
1. SSH → write compose file to `/opt/dockify/apps/<name>/`
2. SSH → `docker compose up -d`
3. Inject Caddy route via Admin API (domain → container:port)
4. If basic auth is set, Caddy requires username/password before proxying
5. Create Cloudflare DNS A record (if configured)
6. Record deployment + save compose snapshot for rollback
7. Status → **running**

## Git Webhook CI/CD

Dockify can auto-deploy on every push via GitHub or GitLab webhooks. When an app is created with a `Git Repo URL` and `Branch`, Dockify matches incoming webhooks by repo + branch and triggers a redeploy.

### Setup

1. In your **app repo** (the one you want to auto-deploy), go to **Settings → Webhooks**
2. Add a webhook pointing to your Dockify instance:

```
Payload URL: https://dockify.example.com/api/webhook/github
Content type: application/json
Events: Just the push event
```

For GitLab, use `/api/webhook/gitlab` instead.

3. In the Dockify UI, when creating the app, fill in:
   - **Git Repo URL:** `https://github.com/user/repo.git`
   - **Branch:** `main`

Dockify ignores non-push events gracefully (returns 200 with `"ignored"`).

### Sample GitHub Actions Workflow

You can also trigger deploy from GitHub Actions directly, without setting up a webhook. This gives you more control (run tests first, then deploy):

```yaml
name: Deploy via Dockify

on:
  push:
    branches: [main]

jobs:
  test-and-deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Run tests
        run: |
          echo "Running tests..."
          # npm test, go test, etc.

      - name: Trigger Dockify deploy
        run: |
          curl -s -X POST https://dockify.example.com/api/webhook/github \
            -H "Content-Type: application/json" \
            -d '{
              "ref": "refs/heads/main",
              "after": "${{ github.sha }}",
              "repository": {
                "clone_url": "https://github.com/${{ github.repository }}.git"
              }
            }'
```

Dockify matches the `clone_url` and `ref` against registered apps, then redeploys the matching app with the commit SHA recorded in deployment history.

### How It Works

1. GitHub sends a push event to `POST /api/webhook/github`
2. Dockify parses `ref` → branch (`refs/heads/main` → `main`), `after` → commit SHA, `clone_url` → repo URL
3. Finds the matching app by `git_repo` + `git_branch`
4. Triggers `deployWithCommit(app.ID, commitSHA)` — same deploy flow as UI
5. Records the deployment with commit SHA in history

## Updating Dockify

A universal update script detects your install mode and updates accordingly:

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash
```

Or manually per mode:

**Mode 1 (Docker Compose):**
```bash
cd /opt/dockify && docker compose pull && docker compose up -d
```

**Mode 2 & 3 (Binary):**
```bash
sudo systemctl stop dockify
sudo curl -fsSL -o /usr/local/bin/dockify \
  "https://github.com/coderbuzz/dockify/releases/latest/download/dockify-linux-amd64"
sudo chmod +x /usr/local/bin/dockify
sudo systemctl start dockify
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

## License

MIT © 2025 CoderBuzz
