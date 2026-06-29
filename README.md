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

| Variable | Default | Required | Description |
|---|---|---|---|
| `DOCKIFY_HOST` | `0.0.0.0` | No | Network interface to bind |
| `DOCKIFY_PORT` | `8080` | No | HTTP port |
| `DOCKIFY_DATA_DIR` | `/var/lib/dockify` | No | SQLite DB + data storage directory |
| `DOCKIFY_SSH_KEY_DIR` | `/var/lib/dockify/keys` | No | Per-server SSH private key storage |
| `DOMAIN` | — | Mode 1 | Domain for Caddy reverse proxy (auto HTTPS) |
| `DOCKIFY_ADMIN_USER` | `admin` | No | Web UI login username |
| `DOCKIFY_ADMIN_PASSWORD` | — | No | Web UI password. If not set, the web UI has **no authentication** |
| `CLOUDFLARE_API_TOKEN` | — | No | Cloudflare API token for automatic DNS A records |
| `CLOUDFLARE_ZONE_ID` | — | No | Cloudflare zone ID |
| `DOCKIFY_BASE_PATH` | — | No | URL prefix when behind a reverse proxy (e.g. `/proxy/9898`) |

### Authentication

To enable web UI authentication, set `DOCKIFY_ADMIN_PASSWORD`:

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

The login page is at `/login`. Webhook endpoints (`/api/webhook/github`, `/api/webhook/gitlab`) and health check (`/health`) do not require authentication.

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

A global **webhook secret** is auto-generated on first startup. See the [Webhook Security](#webhook-security) section below for details on how it works and how to configure it.

### Setup

1. In your **app repo** (the one you want to auto-deploy), go to **Settings → Webhooks → Add webhook**
2. Fill in:
   ```
   Payload URL: https://dockify.example.com/api/webhook/github
   Content type: application/json
   Secret: <copy from Dockify Settings page — Webhook Secret field>
   Events: Just the push event
   ```
3. In the Dockify UI, when creating the app, fill in:
   - **Git Repo URL:** `https://github.com/user/repo.git`
   - **Branch:** `main`

Dockify verifies incoming webhooks using HMAC-SHA256 (GitHub) or secret token (GitLab). If the secret doesn't match, the webhook is rejected with 401. Non-push events are ignored gracefully (returns 200 with `"ignored"`).

### Webhook Security

Dockify auto-generates a global **webhook secret** (64-character hex) on first startup, stored in the database. You can view, copy, regenerate (roll), or disable it on the **Settings** page (`/settings`). When disabled, webhooks are accepted without signature verification.

When the webhook secret is set, Dockify validates incoming webhooks:

- **GitHub** — HMAC-SHA256 via `X-Hub-Signature-256` header
- **GitLab** — plain token comparison via `X-Gitlab-Token` header

If the signature or token does not match, the webhook is rejected with `401 Unauthorized`.

**To configure:**

1. Go to **Settings** in the Dockify web UI
2. Click **Show** → **Copy** the webhook secret
3. Paste it as the **Secret** in your GitHub/GitLab webhook settings
4. For GitHub Actions, pass the secret via `${{ secrets.DOCKIFY_WEBHOOK_SECRET }}` (see examples below)

> **Note:** When you **Roll** the secret, the old one stops working immediately. Update all CI secrets to match. Use **Disable** to turn off secret verification entirely.

### Trigger Deploy via GitHub Actions

You can trigger deploys from GitHub Actions with or without webhook secret verification.

Add repository secrets (**Settings → Secrets and variables → Actions**):

- `DOCKIFY_URL` — your Dockify URL (e.g. `https://dockify.example.com`)
- `DOCKIFY_WEBHOOK_SECRET` — the webhook secret from Dockify Settings (for Option B)

**Option A: Without webhook secret** (simple; disable the webhook secret on the Settings page first, or use when no secret is configured)

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
        env:
          DOCKIFY_URL: ${{ secrets.DOCKIFY_URL }}
        run: |
          curl -s -X POST $DOCKIFY_URL/api/webhook/github \
            -H "Content-Type: application/json" \
            -d '{
              "ref": "refs/heads/main",
              "after": "${{ github.sha }}",
              "repository": {
                "clone_url": "https://github.com/${{ github.repository }}.git"
              }
            }'
```

**Option B: With webhook secret** (HMAC-SHA256, recommended for public-facing deployments)

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
        env:
          DOCKIFY_URL: ${{ secrets.DOCKIFY_URL }}
          DOCKIFY_WEBHOOK_SECRET: ${{ secrets.DOCKIFY_WEBHOOK_SECRET }}
        run: |
          REPO="https://github.com/${{ github.repository }}.git"
          BRANCH="${GITHUB_REF#refs/heads/}"
          COMMIT="${{ github.sha }}"

          PAYLOAD=$(cat <<EOF
          {"ref":"refs/heads/${BRANCH}","after":"${COMMIT}","repository":{"clone_url":"${REPO}"}}
          EOF
          )

          SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$DOCKIFY_WEBHOOK_SECRET" | sed 's/^.* //')

          curl -s -o /dev/null -w "%{http_code}" \
            -X POST $DOCKIFY_URL/api/webhook/github \
            -H "Content-Type: application/json" \
            -H "X-GitHub-Event: push" \
            -H "X-Hub-Signature-256: sha256=${SIGNATURE}" \
            -d "$PAYLOAD"
```

Dockify matches the `clone_url` and `ref` against registered apps, then redeploys the matching app with the commit SHA recorded in deployment history.

### How It Works

1. GitHub sends a push event to `POST /api/webhook/github`
2. Dockify parses `ref` → branch (`refs/heads/main` → `main`), `after` → commit SHA, `clone_url` → repo URL
3. Finds the matching app by `git_repo` + `git_branch`
4. Triggers `deployWithCommit(app.ID, commitSHA)` — same deploy flow as UI
5. Records the deployment with commit SHA in history

## App Configuration

### Environment Variables (Secrets)

Each app can have key-value secrets stored in Dockify. On deploy, they are written as a `.env` file to `/opt/dockify/apps/<name>/.env` on the worker VM.

Manage via the app detail page in the Web UI, or the API:

```
GET    /api/apps/:id/secrets        List secrets
POST   /api/apps/:id/secrets        Set a secret (body: `{"key":"...","value":"..."}`)
DELETE /api/apps/:id/secrets/:key   Delete a secret
```

### Config Files

Each app can have arbitrary config files. On deploy, they are written to `/opt/dockify/apps/<name>/<path>` on the worker VM.

```
GET    /api/apps/:id/files          List files
POST   /api/apps/:id/files          Set a file (body: `{"path":"...","content":"..."}`)
DELETE /api/apps/:id/files/:path    Delete a file
```

### Stop / Start

Pause and resume an app without undeploying or losing configuration:

```
POST   /api/apps/:id/stop           Runs `docker compose stop`
POST   /api/apps/:id/start          Runs `docker compose start`
```

### Unique Service Name

When enabled, Dockify renames the first service in the docker-compose file to a sanitized version of the app name (replacing `.`, `_`, spaces with `-`). This prevents service name conflicts when multiple apps use the same compose template.

### HTTP Basic Auth

Set `auth_user` and `auth_pass` on an app to require HTTP basic auth before accessing it. Caddy enforces this at the proxy level — the app itself does not need to handle authentication.

## Server Features

### Resource Monitoring

Dockify collects CPU, RAM, and disk usage from all online servers every 60 seconds. Resource data is visible on server detail pages and used by the scheduler to pick the least-loaded server for new deployments.

```
POST   /api/servers/:id/refresh     Manually refresh a server's resource metrics
```

### Server Edit

Update a server's host, port, user, or SSH key after creation:

```
PATCH  /api/servers/:id             Partial update of server fields
```

## Backup & Restore

Export your configuration (servers + apps) as YAML and import it into another instance.

### Export

```
GET    /api/backup/export           Download YAML config file
```

The export includes all servers (name, host, port, user) and apps (name, domain, port, compose, git repo, auth settings). **SSH keys are not exported** — you must re-enter them after import.

### Import

Upload a previously exported YAML file via the Import page (`/import`) or API:

```
POST   /api/backup/import           Upload YAML file (multipart: `file` + `mode`)
```

- **merge** (default): skip entries that already exist by name
- **replace**: delete all existing servers and apps before import

## Settings & Updates

The **Settings** page (`/settings`) provides:

| Feature | Description |
|---|---|
| **Webhook Secret** | View, copy, or roll (regenerate) the global webhook secret |
| **Update Check** | Checks GitHub Releases for a newer version |
| **Run Update** | Triggers a self-update via `systemd-run` — downloads and runs the latest `update.sh` |

Manual update via command line:

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
│   ├── server/        # Server CRUD, resource monitoring
│   ├── app/           # App CRUD, deployment
│   ├── caddy/         # Caddy Admin API client
│   ├── cloudflare/    # Cloudflare DNS API
│   ├── webhook/       # Git webhook handler
│   ├── scheduler/     # Idle server selection
│   ├── settings/      # Global settings, update checker
│   ├── backup/        # Export/import YAML config
│   ├── db/            # SQLite layer
│   └── http/          # HTTP server, handlers, templates
├── web/static/        # CSS, JS (embedded)
├── scripts/           # Worker init scripts
└── docs/              # Documentation
```

## License

MIT © 2025 CoderBuzz
