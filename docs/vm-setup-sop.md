# VM setup and security SOP (controller and workers)

Standard operating procedure for bringing up a new Dockify **controller** VM or
**worker** VM, written so that an AI operator (or a human) can execute it step by
step. Every step has the exact command, the expected result and how to undo it.
It reflects what runs in production since Dockify 0.7.0 (2026-10-15); items that
are recommended but not yet applied on the current production hosts are marked
**[recommended]**.

Conventions for the operator:

- Ask the owner for secrets (admin password, API token, Cloudflare token); never
  print secret values back, never commit them. Mask values when inspecting `.env`
  (`awk -F= '{print $1"=<set>"}'`).
- Never change a production host without the owner's explicit go for that change.
  Read-only inspection is fine.
- Any firewall or sshd change made over SSH is done with the lockout watchdog in
  §1.4 and verified with a **fresh** SSH connection before the old one is closed.
- Record what was done (date, host, command) in the audit log or PR body.

Placeholders: `<controller-domain>` (e.g. `dockify.example.com`),
`<controller-ip>`, `<worker-ip>`, `<ssh-user>` (`dockify` on new workers).

---

## 1. Controller VM

Production layout: Debian 12, Dockify binary under systemd as root, native Caddy
as the HTTPS front, SQLite in `/var/lib/dockify`, SSH keys in
`/var/lib/dockify/keys`. The controller only talks **outbound** to workers (SSH),
GitHub (releases, webhooks come inbound via Caddy) and Cloudflare (DNS API).

### 1.1 Base OS and SSH access

1. Debian 12 minimal, up to date: `sudo apt-get update && sudo apt-get -y upgrade`.
2. Admin user with passwordless sudo (production: `indra`), key-only login.
   Put the operator's public key in `~/.ssh/authorized_keys` (700/600).
3. sshd: key-only, no passwords. Write `/etc/ssh/sshd_config.d/dockify.conf`:

   ```
   PasswordAuthentication no
   KbdInteractiveAuthentication no
   PermitRootLogin prohibit-password
   ```

   then `sudo sshd -t && sudo systemctl reload ssh`. Verify with a **new** SSH
   connection before closing the current one. Undo: delete the file and reload.
4. Confirm the host has what the scripts need: `curl`, `python3` (used for
   read-only DB checks), `sha256sum`, `systemd-run`.

### 1.2 Install Dockify (binary + native Caddy, install mode 3)

1. Create DNS `A <controller-domain> -> <controller-ip>` (Cloudflare).
2. Run the installer and answer the prompts (mode `3`, admin user, **password is
   required**, Cloudflare token and zone, base path empty, the domain):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/install.sh | bash
   ```

   It installs `/usr/local/bin/dockify` and `/usr/local/bin/caddy`, writes
   `/opt/dockify/.env`, `/opt/dockify/Caddyfile`
   (`<controller-domain> { reverse_proxy 127.0.0.1:8080 }`) and the units
   `dockify.service` (root, `EnvironmentFile=/opt/dockify/.env`, `Restart=on-failure`)
   and `dockify-caddy.service`.
3. Add the two settings the installer does not write yet, then restart:

   ```bash
   sudo tee -a /opt/dockify/.env > /dev/null <<'EOF'
   DOCKIFY_HOST=127.0.0.1
   DOCKIFY_TRUST_PROXY=true
   DOCKIFY_API_TOKEN=<openssl rand -hex 32>
   EOF
   sudo chmod 600 /opt/dockify/.env
   sudo systemctl restart dockify
   ```

   - `DOCKIFY_HOST=127.0.0.1`: only Caddy reaches Dockify; port 8080 is never public.
   - `DOCKIFY_TRUST_PROXY=true`: client IPs come from Caddy's `X-Forwarded-For`
     (needed for the login throttle; never set it when Dockify is reached directly).
   - `DOCKIFY_API_TOKEN`: bearer token for scripts and agents.
   - Dockify refuses to start without `DOCKIFY_ADMIN_PASSWORD` on a non-loopback
     bind unless `DOCKIFY_ALLOW_NO_AUTH=true` (dev only).
4. Expected state:

   ```bash
   systemctl is-active dockify dockify-caddy          # active active
   sudo ss -ltn | grep -E ':8080|:443|:80 '            # 127.0.0.1:8080, *:80, *:443
   curl -s https://<controller-domain>/health          # ok
   sudo journalctl -u dockify -n 30 --no-pager         # "database at version N", "starting on 127.0.0.1:8080"
   ```

   From another machine: `curl -s --max-time 5 http://<controller-ip>:8080/health`
   must **fail** to connect.
5. Data layout and permissions (created by Dockify): `/var/lib/dockify` 0700 root,
   `dockify.db` plus `-wal`/`-shm`, `keys/<id>.pem` and `keys/<id>.registry-token`
   0600. Nothing else writes there.

### 1.3 Caddy front (what the application relies on)

- Caddy passes the original `Host` and sets `X-Forwarded-For`/`X-Forwarded-Proto`
  by default; Dockify's cross-site checks (`Origin`/`Sec-Fetch-Site` vs `Host`),
  the WebSocket origin check and the `Secure` session cookie depend on that. Do not
  rewrite `Host` in the Caddyfile.
- HTTP/3 (UDP 443) is on by default; the firewall allows it.
- Caddy's own admin endpoint stays on loopback (`127.0.0.1:2019`).

### 1.4 Host firewall (ufw) — applied on production 2026-10-15

Enable over SSH only with a watchdog that disables the firewall automatically if
the new rules lock you out:

```bash
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y ufw
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp  comment 'ssh'
sudo ufw allow 80/tcp  comment 'caddy http'
sudo ufw allow 443/tcp comment 'caddy https'
sudo ufw allow 443/udp comment 'caddy http3'
sudo ufw logging low
sudo systemd-run --on-active=240 --unit=ufw-watchdog /usr/sbin/ufw disable   # safety net, 4 min
sudo ufw --force enable
```

Then, from your machine, open a **new** SSH session; if it works:

```bash
sudo systemctl stop ufw-watchdog.timer; sudo systemctl reset-failed ufw-watchdog.service
sudo ufw status verbose          # active, enabled on boot, the four rules (v4 and v6)
```

If the new session fails, wait: the watchdog disables ufw after 4 minutes.
Verify from outside: `nc -z -w4 <controller-ip> 22` ok, 80/443 ok, 8080/2019 blocked;
`https://<controller-domain>/health` 200. Undo: `sudo ufw disable`.

### 1.5 Backups and rollback (before every update)

1. Keep a rollback folder: `/opt/dockify/rollback` (0700 root) with the current
   binary (`dockify-<version>`), a copy of `.env` (`env`) and a database snapshot
   taken **with Dockify stopped**:

   ```bash
   sudo systemctl stop dockify
   sudo cp /var/lib/dockify/dockify.db /opt/dockify/rollback/dockify.db     # plus -wal/-shm if present
   sudo systemctl start dockify
   ```

   (A consistent snapshot while running: `sudo python3 -c "import sqlite3; s=sqlite3.connect('file:/var/lib/dockify/dockify.db?mode=ro',uri=True); d=sqlite3.connect('/opt/dockify/rollback/dockify.db'); s.backup(d)"`.)
2. `rollback.sh` in that folder stops Dockify, restores the binary, the snapshot
   and `.env`, keeps the newer DB aside, and starts Dockify again. One command:
   `sudo /opt/dockify/rollback/rollback.sh`.
3. Delete the folder once the new version has run for a few days.

### 1.6 Updating the controller (release SOP)

1. Code reaches `main` only through reviewed PRs; CI runs `go vet`, `gofmt -l`,
   `go test -race`, `govulncheck` and the smoke test on every PR and push.
2. Release from `main`: `./scripts/release.sh minor|patch` (tags `vX.Y.Z`); CI
   publishes `dockify-linux-amd64` and `dockify-linux-amd64.sha256`.
3. On the controller, after §1.5: `curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash`
   (or the Settings page, which runs the same script pinned to the release tag via
   `systemd-run`). The script verifies the checksum, keeps the previous binary as
   `dockify.bak` and rolls it back if the service does not come up.
4. Verify: `journalctl -u dockify` shows the migration lines and
   `database at version N`; `/health` 200 through the domain; `/api/apps` lists all
   apps; redeploy one staging app and compare the container env before/after
   (`docker exec <c> env | sort | sha256sum` on the worker).

### 1.7 Dockify settings that are part of the standard

- Webhook secret is always enforced (HMAC-SHA256 for GitHub, token for GitLab);
  roll it from Settings and update CI secrets together. Webhooks go to
  `https://<controller-domain>/api/webhook/github`, never to an IP.
- Login: constant-time compare, 1 s delay per failure, 429 after 10 failures per
  client per 15 minutes (needs `DOCKIFY_TRUST_PROXY=true` behind Caddy).
- Sessions: `HttpOnly`, `SameSite=Lax`, `Secure` behind HTTPS, 24 h; browser assets
  are served from the binary with a CSP; no CORS.
- Backups: Settings → Export with a passphrase (AES-GCM, PBKDF2 600k); the export
  carries servers (SSH keys and registry tokens encrypted), apps, env vars, files and
  all domains. Store it off the controller.

---

## 2. Worker VM

Production layout: Debian 12 (or Photon OS), Docker Engine with the compose plugin,
a `caddy` container managed by Dockify (ports 80/443, admin on loopback), apps under
`/opt/dockify/apps/app-<id>` on the `dockify` Docker network. The worker never
initiates connections to the controller.

### 2.1 Base OS and SSH user

1. Debian 12 minimal, updated.
2. Dedicated SSH user for Dockify, non-root (production `prod-heavy-1` pattern):

   ```bash
   sudo adduser --disabled-password --gecos '' dockify
   ```

   Passwordless sudo for that user is **optional**: Dockify only uses it (`sudo -n`)
   to preserve file ownership when an app is moved between workers; without it the
   move still works, ownership is not preserved. Root as the SSH user (the older
   `staging` pattern) works but is not the standard for new VMs.
3. sshd hardening as in §1.1 (`PasswordAuthentication no`, `PermitRootLogin
   prohibit-password`), verified with a fresh connection.
4. GNU coreutils `timeout` must exist (`timeout --version`); Dockify wraps every
   remote command in it. Debian and Photon have it; Alpine/busybox is untested.

### 2.2 Register the server in Dockify (keys are generated on the controller)

1. Dockify UI → Servers → Add Server: name, `<worker-ip>`, port 22, user
   `dockify`, leave the SSH key empty. Dockify generates an ed25519 key pair, stores
   the private half as `/var/lib/dockify/keys/<id>.pem` (0600) and shows the
   **public key** and the exact setup command on the server page. The private key
   never leaves the controller. (API: `POST /api/servers {"name","host","user"}`
   returns `public_key`.)
2. Registry credential (optional, for private images such as `ghcr.io`): fill
   registry host, user and token on the same form; the token is stored as a 0600
   file on the controller, sent to `docker login --password-stdin` over SSH on each
   deploy and never shown by the API.

### 2.3 Prepare the worker with the public key

On the worker, as the SSH user, with the public key from the server page:

```bash
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/setup-worker.sh | bash -s -- 'ssh-ed25519 AAAA... dockify'
```

The script adds the key to `~/.ssh/authorized_keys` (700/600, no duplicates),
installs Docker and the compose plugin if missing, adds the user to the `docker`
group, and prepares `/opt/dockify/{apps,caddy}` owned by that user. It never
generates or prints a private key; without the argument it prints usage and exits.
Log out and in again so the `docker` group applies.

### 2.4 Init Worker and host-key pinning

1. Dockify UI → the server page → **Init Worker**. Dockify connects, pins the
   worker's SSH host key on this first connection (trust on first use), installs
   or verifies Docker, creates the `dockify` network and starts Caddy:
   image `caddy:2`, label `dockify.managed=true`, `--restart unless-stopped`,
   `-p 80:80 -p 443:443 -p 127.0.0.1:2019:2019`, volumes `caddy_data:/data` and
   `/opt/dockify/caddy:/etc/caddy`, config `/etc/caddy/config.json` with metrics
   enabled. A `caddy` container that Dockify did not create makes Init stop with an
   error instead of replacing it.
2. **Verify the pinned host key** (the only moment trust is decided): on the
   worker `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub` must print the same
   `SHA256:…` fingerprint as the server page ("Host Key" row) and the controller
   log line `pinned host key for <worker-ip>:22 (SHA256:…)`. A mismatch means the
   first connection was intercepted: stop and investigate.
3. If the worker is ever reinstalled, tick **Forget pinned host key** on the edit
   form (or `PATCH /api/servers/<id> {"host_key_clear":true}`); changing host or
   port also clears it. Dockify refuses to connect on a mismatch otherwise.
4. Expected state on the worker:

   ```bash
   docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep caddy      # caddy caddy:2 Up
   docker exec caddy curl -s localhost:2019/config/ | head -c 200             # JSON config
   ls -la /opt/dockify/caddy/config.json                                      # persisted config
   ```

### 2.5 Host firewall on the worker — [recommended], not yet applied on production workers

Same procedure and watchdog as §1.4: `ufw default deny incoming`, allow `22/tcp`,
`80/tcp`, `443/tcp`, `443/udp`. Two Docker caveats: Docker publishes ports with its
own iptables rules that bypass ufw, so anything a compose file publishes with
`ports:` is reachable regardless; and the Caddy admin port is already bound to
loopback. Prefer `expose` (simple mode does this) over `ports` for apps.

### 2.6 Conventions enforced per app (by Dockify, nothing to configure)

- Domains, env keys and config-file paths are validated at the API, form and import
  boundary; `.env` is written over stdin as `KEY="value"` with mode 0600 and
  rewritten on every deploy; config files are 0644.
- Apps live only on the internal `dockify` network and are exposed through Caddy;
  HTTP basic auth is enforced by Caddy with bcrypt.
- One lifecycle operation per app at a time (`409 operation in progress`).
- A deploy whose Caddy route fails is marked **failed**, not success.
- Undeploy removes only the app's own containers, volumes and images; worker-wide
  pruning is the explicit **Prune** button. Cloudflare records are deleted on undeploy
  only when they point at a Dockify server.
- Caddy's running config is persisted to the file it was started with after every
  route change, so routes survive a Caddy restart or a reboot.

### 2.7 Migrating a worker set up with the old script

Workers prepared before 0.7.0 hold a private key in `~/.ssh/dockify` that was
pasted into Dockify. They keep working. Optional cleanup: delete `~/.ssh/dockify`
and `~/.ssh/dockify.pub` on the worker, keep their line in `authorized_keys`, or
re-add the server with a controller-generated key and run §2.3 again.

---

## 3. Checklist for a new VM (copy into the PR or audit log)

Controller: [ ] OS updated, key-only sshd verified with a fresh login · [ ] installed
in mode 3 with a password · [ ] `.env` has `DOCKIFY_HOST=127.0.0.1`,
`DOCKIFY_TRUST_PROXY=true`, `DOCKIFY_API_TOKEN` · [ ] `/health` 200 via domain,
`<ip>:8080` refused · [ ] ufw active with the four rules, watchdog cancelled ·
[ ] rollback folder with binary, `.env`, DB snapshot, `rollback.sh` · [ ] export
backup stored off-host.

Worker: [ ] OS updated, `dockify` user, key-only sshd · [ ] server added without a
key, public key installed with `setup-worker.sh` · [ ] Init Worker done, `caddy:2`
running with the Dockify label · [ ] host-key fingerprint compared with
`ssh-keygen -lf` on the worker · [ ] (recommended) ufw with 22/80/443 · [ ] first
app deployed, `.env` 0600, route persisted in `/opt/dockify/caddy/config.json`.
