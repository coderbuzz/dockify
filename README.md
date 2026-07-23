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
- **SSH console** — interactive terminal via browser (WebSocket + xterm.js)
- **Dashboard** — live resource monitoring and deployment status at a glance
- **Encrypted backups** — export/import config with AES-GCM passphrase encryption
- **Web UI** — manage everything from the browser, dark/light mode

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
| `DOCKIFY_DEV_MOCK` | `false` | No | Enable mock SSH for local development (no real VMs needed) |
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
3. Click **Add Server** → redirects to server detail. Dockify auto-tests the connection and starts initialization in the background.
4. Click **Initialize Worker** (if not already auto-initialized)

**What "Initialize Worker" does automatically:**
1. SSH connect + verify
2. Install Docker via `get.docker.com` (if not present)
3. Create `dockify` Docker network
4. Deploy Caddy container (port 80/443 + Admin API on localhost:2019)
5. Collect CPU, RAM, Disk info
6. Status → **online**. Ready to deploy apps.

> Initialization is idempotent — re-running it on an already-initialized server skips existing components.

### Step 4: Deploy Your First App

1. Go to **Apps** → **Deploy App**
2. Choose **Simple Mode** (just an image name + env vars + volumes) or **Advanced Mode** (full `docker-compose.yml`)
3. Set domain, port, and select a server (or **Auto-select** for least-loaded)
4. Optional: fill **Basic Auth** username/password to protect the app behind HTTP basic auth
5. Click **Deploy App**

**What happens on deploy:**
1. SSH → write compose file to `/opt/dockify/apps/<name>/`
2. SSH → `docker compose up -d`
3. Inject Caddy route via Admin API (domain → container:port)
4. If basic auth is set, Caddy requires username/password before proxying
5. Create Cloudflare DNS A record (if configured, skips duplicates)
6. Record deployment + save compose snapshot for rollback
7. Status → **running**

## Dashboard

The home page (`/`) provides a live overview of your infrastructure:

- **Stats cards** — total servers, online servers, total apps, running apps
- **Server summary** — all servers with live CPU/RAM usage and status badges
- **App summary** — all apps with status badges and domain links
- Empty state call-to-action buttons when no servers or apps exist yet

## Server Management

The **Servers** page lists all worker VMs with connection status and live resource usage. Click any server for its detail page.

### Add, Edit, Delete

Servers can be added via the web form (name, host, port, user, SSH private key) and edited after creation — update host, port, user, or SSH key without deleting and re-adding.

### Resource Monitoring

Dockify collects CPU, RAM, and disk usage from all online servers every 60 seconds in the background. Resource cards update via HTMX partial refresh (no page reload). A manual refresh button is available on each server detail page.

- **CPU** — core count via `nproc`, usage % via `/proc/stat`
- **RAM** — total and usage via `free -m`, with human-readable "used / free" info
- **Disk** — total and usage via `df -BG`

The scheduler uses these metrics to auto-select the least-loaded server (score = CPU% × 0.5 + RAM% × 0.5).

### SSH Console

Each server detail page includes an **interactive SSH terminal** powered by xterm.js and WebSocket. Open a full terminal session directly in the browser — no SSH client needed.

- WebSocket connection: `GET /api/servers/:id/console`
- Full xterm.js with FitAddon for responsive terminal sizing
- Window resize events propagate to the remote PTY
- Raw keystroke passthrough for a native terminal feel

### Worker Init

The **Initialize Worker** action installs Docker, creates the `dockify` network, and deploys Caddy. It is idempotent — safe to run multiple times.

## App Management

The **Apps** page lists all deployed apps. Click any app for its detail page with full management controls.

### Deploy Modes

- **Simple Mode** — provide an image name, port, volumes, environment variables, resource limits, and an optional command override. Dockify auto-generates the docker-compose file.
- **Advanced Mode** — paste a full `docker-compose.yml` with complete control over the stack.

The compose mode (`simple` / `advanced`) is tracked per app and used to render the correct edit form.

### Resource Limits (Simple Mode)

Optionally enforce container resource constraints via dropdown selects:

- **Memory Limit** — `128m`, `256m`, `512m`, `1g`, `2g`, `4g`
- **CPU Limit** — `0.25`, `0.5`, `1.0`, `2.0`, `4.0`
- **Log Max Size** — max size per log file (`10m`, `50m`, `100m`, `500m`, `1g`)
- **Log Max File** — number of log files to keep (`1`, `3`, `5`, `10`)

Defaults to "unset" (no limit applied). In Advanced mode, set resource limits directly in the compose YAML.

### Command (Simple Mode)

Override the container's default command/entrypoint. Useful for passing flags or running a specific script instead of the image's built-in CMD. Leave empty to use the image default.

### Edit App

Re-configure an existing app via the edit page. Dockify pre-fills the form with current settings (parsed from simple fields or the raw compose), saves changes, and automatically redeploys the app.

### Server Auto-Select

When creating or editing an app, set the server field to **Auto-select** (or `server_id=0` via API). Dockify picks the least-loaded online worker VM based on CPU and RAM usage.

### Stop / Start

Pause and resume an app without undeploying or losing configuration. Stopping runs `docker compose stop` and removes the Caddy route. Starting re-creates the route and DNS record.

### Rollback

Roll back to the last successful deployment. Dockify restores the compose snapshot saved at deploy time and redeploys the app.

### Deployment History

Each app keeps a history of deployments with status (running/failed), log output, commit SHA (for Git-triggered deploys), and timestamps. Viewable on the app detail page.

### Logs

Stream container logs via SSH (`docker compose logs`). Lazy-load buttons on the app detail page let you tail the last 50, 200, or 500 lines with HTMX partial updates.

### Auto-Refresh

While an app is deploying or a server is initializing, the detail page auto-refreshes every 2 seconds until the operation completes.

## Environment Variables & Config Files

### Environment Variables

Each app's environment variables are managed through a **unified editor** with per-variable type control:

- **Plain** — value is visible in the UI and editable inline
- **Secret** — value is masked (`••••••`) in the UI and encrypted in exports

**Multi-line support** — values can span multiple lines (useful for private keys, certificates, etc.) using resizable textareas.

On deploy, **all** variables (plain + secret) are written to a `.env` file at `/opt/dockify/apps/<name>/.env` on the worker VM. In Simple mode, the compose file auto-generates `${KEY}` environment variable references, so containers can access all variables without manual compose editing.

**Editing secrets:** On the edit page, secret values are shown as empty (like GitHub Actions). Leave empty to keep the existing value. Type a new value to override.

**Import from .env:** Click the import button to open a modal dialog, paste `KEY=VALUE` lines, and bulk-add variables. Imported variables default to the Secret type.

### Config Files

Each app can have arbitrary config files. Files are written to `/opt/dockify/apps/<name>/<path>` on the worker VM. Upload files directly from the browser (FileReader API) or paste content. Manage them inline on the app detail page.

### HTTP Basic Auth

Set `auth_user` and `auth_pass` on an app to require HTTP basic auth before accessing it. Caddy enforces this at the proxy level with bcrypt — the app itself does not need to handle authentication.

### Unique Service Name (Simple Mode)

In simple mode, Dockify renames the first service in the generated compose to a sanitized version of the app name (replacing `.`, `_`, spaces with `-`). This prevents service name conflicts when multiple apps use the same image.

## Git Webhook CI/CD

Dockify can auto-deploy on every push via GitHub or GitLab webhooks. When an app is created with a `Git Repo URL` and `Branch`, Dockify matches incoming webhooks by repo + branch and triggers a redeploy. A single webhook can trigger redeploys for multiple matching apps.

A global **webhook secret** is auto-generated on first startup. See the [Webhook Security](#webhook-security) section below.

### Setup

1. In your **app repo**, go to **Settings → Webhooks → Add webhook**
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

Dockify verifies incoming webhooks using HMAC-SHA256 (GitHub) or secret token (GitLab). Non-push events are ignored gracefully (returns 200 with `"ignored"`).

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

### How It Works

1. GitHub sends a push event to `POST /api/webhook/github`
2. Dockify parses `ref` → branch, `after` → commit SHA, `clone_url` → repo URL
3. Finds **all** matching apps by `git_repo` + `git_branch`
4. Triggers `deployWithCommit(app.ID, commitSHA)` for each match — same deploy flow as UI
5. Records the deployment with commit SHA in history

## Cloudflare DNS

When `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ZONE_ID` are configured, Dockify automatically manages DNS records on deploy:

- Creates an A record pointing to the worker VM's IP on app deploy (120s TTL)
- Skips creation if a matching record already exists (deduplication)
- Upserts records if the worker IP changes on re-deploy
- DNS records are tracked in the database for cleanup
- **Certificate-safe updates:** When updating an existing A record with a different IP, Dockify temporarily disables Cloudflare's proxy (orange cloud → gray) to allow Caddy to complete HTTP-based certificate validation. Once the certificate is issued, the proxy is re-enabled to its original state.

No DNS automation occurs if the env vars are not set.

## Backup & Restore

Export your configuration (servers + apps + secrets + config files) as YAML and import it into another instance.

### Encrypted Export

Backups can be protected with a passphrase using **AES-GCM encryption** (PBKDF2 key derivation, 600,000 iterations):

- **Secret** environment variables, SSH keys, and auth passwords are encrypted before export
- **Plain** environment variables are exported as readable YAML (they are not sensitive by definition)
- The `is_secret` flag is preserved on import, so plain/secret distinctions survive migration
- Encrypted values use the prefix `enc:` with base64 salt + nonce + ciphertext
- The export page includes a client-side passphrase generator (32-character hex, via `crypto.getRandomValues`)
- Without a passphrase, data is exported without encryption (still useful for offline/internal backups)

When importing on a new instance, all servers and their SSH keys are restored automatically — no need to re-enter keys one by one, as long as the target VMs are reachable or use the same key pair.

### Import

Upload a previously exported YAML file via the Import page (`/import`):

- **merge** (default): skip entries that already exist by name
- **replace**: delete all existing servers and apps before import
- Encrypted values are validated by attempting decryption before the actual import

### Unified Backup Page

Both **Export** and **Import** are accessible from dedicated pages (`/export`, `/import`) and also linked from the Settings page for convenience.

## Settings

The **Settings** page (`/settings`) provides centralized management:

| Feature | Description |
|---|---|
| **Webhook Secret** | View, copy, or roll (regenerate) the global webhook secret |
| **Update Check** | Checks GitHub Releases for a newer version |
| **Run Update** | Triggers a self-update via `systemd-run` — downloads and runs the latest `update.sh` |

The **About** page (`/about`) shows the current version, project description, and a **Sponsor** link. An update progress bar with live polling keeps you informed during self-updates.

## UI Features

Dockify's web UI is built with Go `html/template` + HTMX and fully custom CSS (no framework):

- **Dark/Light mode** — toggle via nav button, persisted in `localStorage` (`dockify-theme`)
- **HTMX partial updates** — resource cards, log viewer, and status badges update without full page reloads
- **Responsive** — adapts to mobile screen sizes (single breakpoint at 600px)
- **Status badges** — color-coded for all server and app states
- **Confirm dialogs** — destructive actions (undeploy, delete, rollback) require confirmation
- **Relative timestamps** — deployment and resource times rendered in browser timezone

## Dev Mode

For local development without real worker VMs, set `DOCKIFY_DEV_MOCK=true`:

- Enables a **mock SSH client** with realistic CPU/RAM/Disk responses
- **Mock SSH console** — simulated interactive terminal that echoes commands
- A yellow **"Dev Mock Mode"** banner appears above the nav bar
- Works with [Air](https://github.com/air-verse/air) for live-reload:

```bash
air   # starts at http://localhost:8080, auto-rebuilds on save
```

Data is stored in `./data/` (SQLite DB, gitignored). See `AGENTS.md` for full development workflow details.

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

## Helper Scripts

### Generate Remote Access SSH Key

`scripts/gen-ssh-key.sh` generates a dedicated SSH key on any VM for external,
general-purpose access — e.g. handing an agentic AI SSH access for remote
debugging, running shell commands, or cleaning up a compromised VM. It is fully
independent of Dockify's automation key, so you can revoke or rotate it without
affecting Dockify.

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/gen-ssh-key.sh | bash
```

The script will:

1. Generate an SSH key pair at `$HOME/.ssh/remote-access` (ed25519, no passphrase)
2. Add the public key to `$HOME/.ssh/authorized_keys`
3. Configure `KexAlgorithms` in `/etc/ssh/sshd_config` (requires `sudo`)
4. Output the **private key content** — copy this into your agentic-AI / SSH client config

The key is generic and separate from Dockify's `/root/.ssh/dockify` automation key.
To revoke access later: `rm ~/.ssh/remote-access*` and remove its line from
`~/.ssh/authorized_keys`.

## Project Structure

```
dockify/
├── cmd/dockify/main.go
├── internal/
│   ├── ssh/           # SSH client, remote exec, worker init, mock client
│   ├── server/        # Server CRUD, resource monitoring
│   ├── app/           # App CRUD, deployment, rollback
│   ├── caddy/         # Caddy Admin API client
│   ├── cloudflare/    # Cloudflare DNS API
│   ├── webhook/       # Git webhook handler (GitHub + GitLab)
│   ├── scheduler/     # Idle server auto-selection
│   ├── settings/      # Global settings, update checker
│   ├── backup/        # Export/import YAML with encrypted secrets
│   ├── db/            # SQLite layer
│   └── http/          # HTTP server, handlers, templates (HTMX + custom CSS)
├── scripts/           # Install, worker setup, update, release scripts
└── Dockerfile         # Multi-stage Docker build
```

## License

MIT © 2025 CoderBuzz
