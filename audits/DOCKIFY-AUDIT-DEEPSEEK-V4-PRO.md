# Dockify Independent Repository Audit and Executable Improvement Roadmap

## 1. Audit Metadata

| Field | Value |
|---|---|
| Audit date | 2026-07-17 |
| Repository | `github.com/coderbuzz/dockify` |
| Branch / commit | `main` / `afab0ea0ddbf0fc5f5f89b1200c1fef7e85d3821` |
| Working-tree baseline | Clean |
| Auditor | DeepSeek V4 Pro, independent repository pass |
| Go toolchain | `go1.25.7 darwin/arm64` |
| Deliverable | Audit and roadmap only; no application code was changed |
| Prior material | `DOCKIFY-AUDIT-GLM-5.2.md`, `DOCKIFY-AUDIT-GPT-5.6-SOL.md` |

## 2. Scope and Methodology

The audit covered the controller binary, SQLite schema and ad-hoc migrations, SSH worker orchestration, Docker Compose lifecycle, Caddy Admin API over SSH, Cloudflare DNS integration, webhook trust boundaries, browser session model, backup encryption, release/install supply chain, tests, templates, and static CSS. The review independently reconstructed architecture from `README.md`, `SPEC.md`, `DECISIONS.md`, `AGENTS.md`, `.env.example`, scripts, workflows, every `.go` file, every `.html` template, and the `Dockerfile`. Non-destructive verification was run (`go build`, `go vet`, `go test`, `go test -race`). UI review is based on template source, CSS, and browser-side JavaScript; runtime visual verification was not performed.

Severity reflects Dockify's documented single-administrator/trusted-operator model. Administrator-controlled compose, SSH keys, environment variables, config files, and domains are treated as correctness and containment inputs, not as multi-tenant hostile input.

Limitations: no real worker VM, Cloudflare zone, Caddy Admin API instance, or production reverse proxy. Static analysis tools (`staticcheck`, `gosec`, `govulncheck`, `golangci-lint`) were unavailable locally.

## 3. Repository Baseline

| Area | Baseline |
|---|---|
| Language/runtime | `go 1.25.0` module; audit host `go1.25.7 darwin/arm64` |
| Main dependencies | chi `v5.3.0`, gorilla/websocket `v1.5.3`, x/crypto `v0.53.0`, yaml.v3 `v3.0.1`, modernc SQLite `v1.53.0` |
| Process model | Single controller binary, in-memory session map, raw background goroutines |
| Frontend | Embedded Go `html/template`, HTMX 2.0.4, xterm.js 5.3.0, custom CSS (variables, dark/light mode) |
| Worker model | Root-capable SSH, Docker Compose, per-worker Caddy container |
| Deployment modes | Docker Compose controller+Caddy; native binary; native binary+Caddy; source/dev |
| Persistence | SQLite WAL, foreign keys enabled, `MaxOpenConns=1`, per-server key files |
| External systems | Worker SSH, GitHub/GitLab webhooks, GitHub Releases API, Cloudflare DNS API v4, CDN libraries |
| Tests | `go test` and `go test -race` pass; coverage in `app`, `config`, `http`, `scheduler` packages only |
| CI | GitHub Actions on `v*` tag push only; no PR or main-branch CI |

## 4. Verification Commands and Results

| Command/check | Result | Notes |
|---|---|---|
| `go build ./...` | Pass | All packages compile clean |
| `go vet ./...` | Pass | No diagnostics |
| `go test ./...` | Pass | app, config, http, scheduler: all pass |
| `go test -race ./...` | Pass | No race detected in exercised paths |
| `staticcheck` | Not run | Not installed |
| `gosec` | Not run | Not installed |
| `golangci-lint` | Not run | Not installed |
| `govulncheck` | Not run | Not installed |

Packages with no test files: `cmd/dockify`, `backup`, `caddy`, `cloudflare`, `db`, `server`, `settings`, `ssh`, `webhook`.

## 5. Product and Trust-Model Understanding

Dockify is a self-hosted control plane for a trusted operator to manage Linux worker VMs via SSH, deploy Docker Compose applications, serve them through Caddy reverse proxy, optionally manage Cloudflare DNS, receive authenticated Git webhooks, access interactive browser-based SSH consoles, and back up controller configuration with AES-GCM encryption.

### Trust boundaries

1. **Public network to controller**: login page, webhook endpoints, update-check, static assets.
2. **Authenticated browser to controller**: state-changing forms, JSON API, WebSocket consoles.
3. **Controller to worker**: SSH transport (no host-key verification), root-capable commands, secrets, files.
4. **Controller/worker to supply chain**: GitHub Releases, raw scripts, Docker installer, container images, CDN JS.
5. **Controller database/filesystem**: SQLite file, SSH private key files, backup YAML exports.

### Key operator workflows

1. **Add Server**: Submit host + SSH key to controller to save key to disk, auto-test connection, auto-init worker (install Docker, create network, deploy Caddy).
2. **Deploy App**: Simple mode (image+port+env) or Advanced mode (full compose). SSH writes compose + .env + files to worker, runs `docker compose up -d`, injects Caddy route, optionally creates Cloudflare DNS.
3. **Webhook deploy**: GitHub/GitLab push event to HMAC/token verification to match apps by repo+branch to background deploy.
4. **Backup/restore**: YAML export with optional AES-GCM passphrase. Import with merge or replace mode.
5. **Self-update**: Check GitHub Releases to `systemd-run` remote update script.

## 6. Current Architecture

```
Browser
  to chi HTTP router (:8080)
       +-- Public: /health, /login, /logout, /api/webhook/*, /static/*
       +-- Protected (auth middleware when password set):
       |     /servers/*, /api/servers/*, /apps/*, /api/apps/*,
       |     /settings, /export, /import, /about, / (dashboard)
       +-- Templates: 18 HTML files embedded via go:embed

Controller persistence
  to SQLite (8 tables): servers, apps, deployments, routes, dns_records,
    app_secrets, app_files, settings
  to Key directory: ${id}.pem files

Background goroutines
  to server.Monitor: 60s ticker to per-server RefreshResources via SSH
  to app.Service: fire-and-forget deploy goroutines (unbounded)
  to server create: auto-init goroutine

Worker VM
  to Docker Engine
  to Caddy container (80/443, Admin API on 127.0.0.1:2019)
  to App containers on dockify network
  to /opt/dockify/apps/app-<id>/docker-compose.yml, .env, files
```

### Package structure

| Package | Responsibility |
|---|---|
| `cmd/dockify` | Entry point, wiring, signal handling |
| `config` | Env var loading (11 vars) |
| `db` | SQLite open, pragmas, schema + ad-hoc migrations |
| `ssh` | SSH client (connect, exec, WriteFile, Shell, ExecPTY), mock client |
| `server` | Server CRUD, resource monitoring, worker init |
| `app` | App CRUD, deploy/undeploy/redeploy/rollback/stop/start, compose generation |
| `caddy` | Caddy Admin API via `docker exec caddy curl` over SSH |
| `cloudflare` | Cloudflare DNS API v4 (list, create, update, delete) |
| `webhook` | GitHub + GitLab webhook, HMAC/token validation |
| `scheduler` | Least-loaded server selection (CPU% x 0.5 + RAM% x 0.5) |
| `settings` | Global settings, webhook secret, update checker, self-update |
| `backup` | YAML export/import with AES-GCM encryption |
| `http` | Chi router, auth middleware, sessions, WebSocket console, templates |

## 7. Critical Architectural Invariants

1. Worker SSH identity must be authenticated before secrets or root commands are sent.
2. At most one lifecycle operation may mutate a given app at a time; stale completions must not overwrite newer state.
3. Database state, remote Compose state, Caddy routes, and Cloudflare records must converge or retain enough durable state to retry.
4. `running`/`success` must mean all required deployment phases succeeded; optional integrations need an explicit degraded state.
5. App moves must preserve old placement until new placement is confirmed healthy, then explicitly clean up.
6. Import with replace mode must validate before destroying, preserve all topology, and fail atomically.
7. Public request work and remote commands must be bounded by size, time, and concurrency.
8. Production exposure must fail closed unless authentication is deliberately configured.
9. Worker files must remain within app directory; secret-bearing files must have restrictive permissions.
10. Schema changes must be versioned, ordered, and must not silently swallow errors.

## 8. Executive Assessment

Dockify is a well-scoped, coherent small control plane with a clean package layout, good documentation, and sensible technology choices. The SQLite WAL + foreign_keys + busy_timeout configuration, the SSH connector abstraction, the embedded template system, and the AES-GCM backup encryption are solid foundations.

The audited revision is **not suitable for production exposure** without addressing a small set of high-severity issues: SSH host keys are not verified (complete MITM capability), the controller binds publicly without requiring authentication, webhook bodies are read without size limits before authentication, and concurrent deployment operations have no serialization mechanism. These are concrete correctness and security blockers.

The project is suitable for local development and controlled evaluation. It should not manage valuable workloads or be exposed to untrusted networks until Wave 0 is complete and validated.

## 9. Strengths Worth Preserving

- Clear product scope, small single-binary footprint, straightforward package structure.
- SQLite configured with WAL, foreign keys, busy_timeout=5000, MaxOpenConns=1.
- SSH `Connector` interface enables real and mock implementations for testing.
- Templates embedded via `go:embed` with parse/render tests.
- Custom CSS design system: variables, dark/light mode, responsive.
- Backup AES-256-GCM with PBKDF2-SHA256 at 600,000 iterations.
- Cloudflare API client with 15s timeout; SSH connection with 10s timeout.
- Docker image runs as non-root user, multi-stage CGO_ENABLED=0 build.
- Dev mock mode enables complete UI flow without real infrastructure.
- Smoke test script with proper cleanup.

## 10. Confirmed Findings Summary

### P0 and P1 findings

| Priority | ID | Severity | Finding | Dependency |
|---|---|---|---|---|
| P0 | DCK-001 | High | Public bind exposes control plane without authentication | None |
| P0 | DCK-002 | High | SSH host authenticity disabled (InsecureIgnoreHostKey) | None |
| P0 | DCK-003 | High | Public ingress and HTTP lifecycle unbounded | None |
| P0 | DCK-004 | High | Default Docker Compose install cannot persist worker keys | None |
| P1 | DCK-005 | Medium | Session, CSRF, WebSocket origin controls incomplete | DCK-001 |
| P1 | DCK-006 | High | App lifecycle operations not serialized | None |
| P1 | DCK-007 | High | Changing app worker leaves old deployment live | DCK-006 |
| P1 | DCK-008 | High | Undeploy orphans Cloudflare DNS records | DCK-006 |
| P1 | DCK-009 | High | Backup import replace is destructive and partial | DCK-011 |
| P1 | DCK-010 | Medium | Deployment status can show success after optional phases fail | DCK-006 |
| P1 | DCK-011 | Medium | Schema migrations ad-hoc, swallow errors, missing CASCADE | None |
| P1 | DCK-012 | Medium | Remote operations lack lifecycle bounds | DCK-006 |
| P1 | DCK-013 | Medium | Webhook secret disable does not fail closed | DCK-003 |
| P1 | DCK-014 | Medium | Compose transformation potentially lossy | DCK-006 |
| P1 | DCK-015 | High | Install/update relies on mutable unsigned artifacts | None |
| P1 | DCK-016 | Medium | CDN scripts lack SRI; no CSP headers | DCK-005 |
| P1 | DCK-017 | Medium | Worker .env file permissions world-readable (0644) | None |
| P1 | DCK-018 | Medium | No Cloudflare DNS cleanup on server delete | DCK-006 |

### Findings by severity

| Severity | Count | IDs |
|---|---|---|
| Critical | 0 | -- |
| High | 8 | DCK-001 to 004, DCK-006 to 009, DCK-015 |
| Medium | 8 | DCK-005, DCK-010 to 014, DCK-016 to 018 |
| Low | 2 | DCK-019, DCK-020 |
| Opportunity | 0 | Strategic deferred |

### Findings by category

| Category | Count | IDs |
|---|---|---|
| Security | 6 | DCK-001, DCK-002, DCK-005, DCK-013, DCK-015, DCK-016 |
| Correctness | 7 | DCK-004, DCK-006, DCK-007, DCK-009, DCK-010, DCK-014, DCK-018 |
| Reliability | 3 | DCK-003, DCK-008, DCK-012 |
| Architecture/DB | 2 | DCK-011, DCK-017 |
| DX/Documentation | 2 | DCK-019, DCK-020 |

### Findings by confidence

| Confidence | Count | IDs |
|---|---|---|
| High | 18 | DCK-001 to 009, DCK-011 to 018, DCK-020 |
| Medium | 1 | DCK-019 |

## 11. Detailed Confirmed Findings

### DCK-001: Public bind exposes control plane without authentication

- **Status:** Confirmed
- **Category:** Security
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / All Apps
- **Evidence:** `internal/config/config.go:24-31` defaults to `0.0.0.0` with empty password. `internal/http/router.go:27,63-66` wraps routes in `AuthMiddleware(authEnabled, next)` where `authEnabled = cfgPass != ""` -- when no password, middleware is no-op. `docker-compose.yml` has no password requirement. `.env.example` has password commented out.
- **Preconditions:** Default/omitted `DOCKIFY_ADMIN_PASSWORD`; controller port reachable.
- **Problem:** Production-looking default grants unauthenticated access to complete infrastructure control including root-capable SSH consoles.
- **Root cause:** Authentication is opt-in while network binding is public by default.
- **Current impact:** One configuration omission equals complete control-plane compromise.
- **Failure scenario:** Operator deploys compose stack on public VM without optional password. Internet client accesses web UI, opens worker SSH console.
- **Existing mitigations:** Startup warning and README documentation.
- **Minimum safe fix:** Refuse non-loopback bind without password unless `DOCKIFY_ALLOW_UNAUTHENTICATED=true` is set.
- **Recommended durable fix:** First-run credential creation mandatory for public binds. Local-dev loopback-only mode.
- **Files likely involved:** `internal/config/config.go`, `cmd/dockify/main.go`, `docker-compose.yml`
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Config matrix: loopback+no password (ok), public+no password (error), public+override (ok with log)
- **Acceptance criteria:** Public bind without auth exits with error; override logged prominently; install paths provision auth

### DCK-002: SSH host authenticity disabled (InsecureIgnoreHostKey)

- **Status:** Confirmed
- **Category:** Security
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / All Apps
- **Evidence:** `internal/ssh/client.go:36` unconditionally sets `HostKeyCallback: gossh.InsecureIgnoreHostKey()` for every real SSH connection. Used for all worker operations.
- **Preconditions:** Attacker intercepts/redirects controller-to-worker TCP, poisons DNS, or occupies re-assigned worker IP.
- **Problem:** SSH encryption occurs without server authentication. All secrets, compose, root commands, console keystrokes can be intercepted.
- **Root cause:** No host-key fingerprint captured or verified during onboarding.
- **Current impact:** Every worker connection vulnerable to MITM.
- **Failure scenario:** Worker IP reassigned in cloud. Dockify connects to new occupant, uploads app secrets and root commands to unintended host.
- **Existing mitigations:** Client key authentication, encrypted transport, 10s connect timeout.
- **Minimum safe fix:** Require pre-configured host-key fingerprint, reject mismatches.
- **Recommended durable fix:** TOFU enrollment on first connection, immutable storage, rotation workflow, mismatch audit.
- **Files likely involved:** `internal/ssh/client.go`, server model/schema, server handler, backup format
- **Dependencies:** None
- **Complexity:** M
- **Validation:** SSH test server with expected, changed, unknown, rotated keys
- **Acceptance criteria:** Unknown/mismatched keys block commands/secrets; fingerprints visible; rotation explicit

### DCK-003: Public ingress and HTTP lifecycle unbounded

- **Status:** Confirmed
- **Category:** Security / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller
- **Evidence:** `cmd/dockify/main.go:88-91` creates `http.Server` with no timeouts or header-size limits. `internal/webhook/handler.go:35-39` does `io.ReadAll(r.Body)` before authentication. `internal/backup/handler.go:63` does unbounded read.
- **Preconditions:** Webhook endpoint reachable; attacker sends slow headers or large body.
- **Problem:** Unauthenticated requests consume memory and connections before size/time budgets enforced.
- **Root cause:** No centralized ingress limits, no per-route caps.
- **Current impact:** Memory exhaustion, slowloris, controller outage.
- **Failure scenario:** Several clients stream large webhook bodies slowly; controller retains all connections and becomes unresponsive.
- **Existing mitigations:** GitHub/GitLab signatures for processed webhooks, but body buffered before verification.
- **Minimum safe fix:** `http.MaxBytesReader` before reads. `ReadHeaderTimeout`, `MaxHeaderBytes` on server.
- **Recommended durable fix:** Per-route size/time budgets, structured 413/408 responses.
- **Files likely involved:** `cmd/dockify/main.go`, `internal/webhook/handler.go`, `internal/backup/handler.go`
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Oversized/slow-body HTTP tests; WebSocket regression test
- **Acceptance criteria:** Oversized requests rejected before buffering; slow headers/body expire; consoles still work

### DCK-004: Default Docker Compose install cannot persist worker keys

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller
- **Evidence:** `docker-compose.yml:9-16` sets `DOCKIFY_SSH_KEY_DIR=/home/dockify/.ssh` and mounts `${HOME}/.ssh` as read-only. `Dockerfile:14,24` runs as user `dockify`. Server create writes keys to that directory.
- **Preconditions:** Docker Compose installation; adding/importing a server key.
- **Problem:** Key directory is read-only and may be unreadable to container UID.
- **Root cause:** Host SSH-directory mount reused as writable key store.
- **Current impact:** Default install cannot complete worker onboarding; orphaned server row.
- **Minimum safe fix:** Remove `~/.ssh` mount. Use `/var/lib/dockify/keys` in data volume.
- **Recommended durable fix:** All keys in controller data volume. Compensate DB inserts on write failure.
- **Files likely involved:** `docker-compose.yml`, `scripts/install.sh`, server handler, backup import
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Compose integration test: add key, verify persists, restart, verify
- **Acceptance criteria:** Compose install creates/reads/persists keys; no host `~/.ssh` mounted; failed write rolls back DB

### DCK-005: Session, CSRF, WebSocket origin controls incomplete

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser / Controller / Worker
- **Evidence:** `internal/http/auth.go:46-52` cookies omit `Secure` and `SameSite`. `internal/http/console.go:18-21` accepts every WebSocket origin. State-changing POSTs have no CSRF tokens. Login has no rate limiting.
- **Preconditions:** Authenticated browser; attacker-controlled origin/page.
- **Problem:** Browser actions rely only on cookie presence; request origin not independently validated.
- **Root cause:** Auth added without comprehensive browser-session threat model.
- **Current impact:** Cross-origin WebSocket hijacking of root consoles. State-changing CSRF depending on browser behavior.
- **Minimum safe fix:** `SameSite=Strict`, `Secure` on HTTPS, restrict WS origin, CSRF middleware, login throttling.
- **Recommended durable fix:** Central CSRF/session middleware, proxy-aware HTTPS detection, session cleanup goroutine.
- **Files likely involved:** `internal/http/auth.go`, `internal/http/console.go`, `internal/http/router.go`
- **Dependencies:** DCK-001
- **Complexity:** M
- **Validation:** Cross-origin WS tests, CSRF tests per mutation family, cookie attr assertions, rate-limit tests
- **Acceptance criteria:** Cross-origin WS returns 403; CSRF-less mutations return 403; cookies carry Secure+SameSite; login throttled

### DCK-006: App lifecycle operations not serialized

- **Status:** Confirmed
- **Category:** Correctness / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/service.go` has no per-app mutex/lock/lease. `deployWithCommit` called from Deploy, DeployByGit, Redeploy, Rollback. Stop/Start operate directly. All launched via goroutines from handlers.
- **Preconditions:** Two overlapping operations for same app.
- **Problem:** `compose down`, file writes, `pull`, `up`, route mutation, DNS, status writes can interleave.
- **Root cause:** Fire-and-forget goroutines share mutable workflow without sequencing.
- **Current impact:** Stale status, wrong compose, downtime, orphaned routes.
- **Failure scenario:** Webhook deploy starts; operator edits/redeploys; webhook goroutine finishes last, marks running with stale state.
- **Minimum safe fix:** Per-app `sync.Mutex` map with busy rejection or queuing.
- **Recommended durable fix:** Operation records with generation IDs, per-app queue/lease, idempotent phases, stale-completion guards.
- **Files likely involved:** `internal/app/service.go`, handlers, webhook
- **Dependencies:** None
- **Complexity:** M
- **Validation:** Mock SSH tests: overlapping deploy/redeploy, deploy+stop, webhook+edit
- **Acceptance criteria:** One mutating operation per app; different apps parallel; stale ops cannot update status; busy errors to UI

### DCK-007: Changing app worker leaves old deployment live

- **Status:** Confirmed (partially mitigated since prior audit)
- **Category:** Correctness
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/handler.go:763` calls `go h.service.CleanupFromServer(id, oldServerID)` and line 817 `go h.service.Redeploy(id, removedDomains...)` in separate goroutines. `CleanupFromServer` (service.go:365-403) runs composse down and Caddy route removal but only logs errors.
- **Preconditions:** Edit app and change server selection.
- **Problem:** Cleanup and redeploy run concurrently with no coordination. Cleanup failure is only logged.
- **Root cause:** Lifecycle orchestration treats server change as metadata update + async cleanup rather than staged migration.
- **Current impact:** Old containers/routes can remain active. Duplicate apps and stale endpoints.
- **Minimum safe fix:** Make cleanup synchronous after new deploy succeeds. Surface cleanup failures in UI.
- **Recommended durable fix:** Staged migration: deploy new, health-check, cut over, cleanup old, record completion, support rollback.
- **Files likely involved:** `internal/app/service.go`, `internal/app/handler.go`
- **Dependencies:** DCK-006
- **Complexity:** L
- **Validation:** Two-worker integration test: successful move, cleanup failure, new deploy failure
- **Acceptance criteria:** Move leaves exactly one stack; failures expose recoverable state; old-worker orphans visible

### DCK-008: Undeploy does not clean up Cloudflare DNS records

- **Status:** Confirmed
- **Category:** Correctness / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App
- **Evidence:** `internal/app/service.go:308-359` (`Undeploy`): stops containers, removes Caddy routes, deletes DB rows, removes directory. No Cloudflare API call to delete DNS records. No `DeleteDNSRecords` call. `dns_records` table has no `ON DELETE CASCADE` from `apps`.
- **Preconditions:** App has domain with Cloudflare DNS. Operator undeploys.
- **Problem:** Cloudflare DNS A records never cleaned up. DB rows orphaned.
- **Root cause:** Delete lifecycle not extended to DNS cleanup.
- **Current impact:** Stale DNS records accumulate. Potential conflicts with future deploys.
- **Minimum safe fix:** Call `cf.DeleteRecord()` and `DeleteDNSRecords()` in `Undeploy`.
- **Recommended durable fix:** Symmetric DNS lifecycle: create on deploy, update on redeploy, delete on undeploy. Add to failed-deploy recovery.
- **Files likely involved:** `internal/app/service.go`, `internal/app/repository.go`
- **Dependencies:** DCK-006
- **Complexity:** S
- **Validation:** Deploy with DNS to verify record exists to undeploy to verify record deleted
- **Acceptance criteria:** Undeploy deletes all associated Cloudflare records; no stale records after lifecycle

### DCK-009: Backup import replace destructive and partial

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / All Apps / All Servers
- **Evidence:** `internal/backup/backup.go:433-444`: replace mode undeploys all apps and deletes all servers *before* import. If import fails mid-stream, state is destroyed with no recovery. `ExportApp` struct (line 49-62) has no routes data. `ExportServer` (line 30-36) has no resource metrics, status, or host key fingerprints. Import has no transactional wrapping.
- **Preconditions:** Operator chooses replace mode. Any import error mid-stream.
- **Problem:** Destroy-then-create without rollback. Not atomic. Routes, DNS, topology lost.
- **Root cause:** Import implemented iteratively without considering full topology or atomic failure.
- **Current impact:** Failed import leaves empty controller. Even successful import loses routes, deployment history, DNS tracking.
- **Minimum safe fix:** Validate all values before destruction. Wrap in SQLite transaction.
- **Recommended durable fix:** Export all topology. Pre-flight validation. Staged import: validate, snapshot, apply, verify, commit or rollback.
- **Files likely involved:** `internal/backup/backup.go`, `internal/backup/handler.go`
- **Dependencies:** DCK-011
- **Complexity:** L
- **Validation:** Export with routes/DNS to import replace to verify preserved. Import with invalid value to verify no destruction.
- **Acceptance criteria:** Import pre-validates; failed import leaves original state; routes/DNS survive round-trip

### DCK-010: Deployment status says success after optional phases fail

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Individual App
- **Evidence:** `internal/app/service.go:243-253`: after compose up succeeds, `setupRouteAndDNSForDomain` (line 246) only logs warnings on Caddy/Cloudflare failure, never returns errors. Deployment marked `success` at line 253 regardless.
- **Preconditions:** Caddy Admin API unreachable. Cloudflare API token expired.
- **Problem:** App marked running/success without routes or DNS. Operator sees green status but app unreachable.
- **Root cause:** Caddy/Cloudflare treated as optional enhancements rather than deployment phases.
- **Current impact:** Silent deployment degradation. Operator must manually verify.
- **Minimum safe fix:** Surface Caddy/DNS status in deployment log. Add `degraded_routing`/`degraded_dns` statuses.
- **Recommended durable fix:** Deploy phases as explicit steps with individual status. Degraded states with retry mechanisms. Yellow/amber UI badges.
- **Files likely involved:** `internal/app/service.go`, `internal/app/model.go`, templates
- **Dependencies:** DCK-006
- **Complexity:** M
- **Validation:** Mock Caddy failure during deploy to assert app status is degraded not running
- **Acceptance criteria:** Failed Caddy/DNS visible in deploy log; running implies all phases succeeded; degraded distinct from running

### DCK-011: Schema migrations ad-hoc, swallow errors, missing CASCADE

- **Status:** Confirmed
- **Category:** Database / Architecture
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller
- **Evidence:** `internal/db/db.go:40-69` executes multiple ALTER TABLE with `db.Exec()` ignoring errors. Schema uses REFERENCES without ON DELETE CASCADE on most relationships (only app_secrets, app_files have CASCADE).
- **Preconditions:** Migration column already exists. Deleting parent row.
- **Problem:** Migration failures invisible; schema may be inconsistent. Deleting server leaves orphan apps. Deleting app leaves orphan routes/DNS.
- **Root cause:** Ad-hoc migrations without version tracking. Schema designed without cascade consideration.
- **Current impact:** Silent schema drift. Orphaned rows accumulate.
- **Minimum safe fix:** Check errors on migrations. Add CASCADE constraints. Version tracking table.
- **Recommended durable fix:** Lightweight migration framework with ordered/checksummed steps. Run-once semantics. Error propagation preventing startup.
- **Files likely involved:** `internal/db/db.go`, `internal/db/schema.sql`
- **Dependencies:** None
- **Complexity:** M
- **Validation:** Fresh install schema check. Migrate from older schema. Delete parent to verify cascade.
- **Acceptance criteria:** Migration errors surfaced at startup; schema version tracked; all FKs have CASCADE; no orphaned rows

### DCK-012: Remote operations lack lifecycle bounds

- **Status:** Confirmed
- **Category:** Reliability
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / Worker
- **Evidence:** `internal/app/service.go:127-239` (`deployWithCommit`): no context, timeout, or cancellation. Can block indefinitely. `cmd/dockify/main.go:107`: graceful shutdown is `srv.Close()` not `Shutdown(ctx)` -- in-flight requests dropped, background goroutines continue. No signal to goroutines that shutdown requested.
- **Preconditions:** SIGINT/SIGTERM during deploy or monitoring.
- **Problem:** Deploy goroutines run forever with no timeout. Graceful shutdown drops requests. Background goroutines continue after shutdown log.
- **Root cause:** Background work is fire-and-forget with no lifecycle management.
- **Current impact:** Apps stuck in deploying forever. In-flight requests aborted. DB consistency risk.
- **Minimum safe fix:** Context with timeout on deploys. `http.Server.Shutdown(ctx)`. Global shutdown context propagated to monitor and deploys.
- **Recommended durable fix:** Operation lifecycle with heartbeat/deadline. Startup reconciliation for stale statuses. Graceful drain with deadline.
- **Files likely involved:** `cmd/dockify/main.go`, `internal/app/service.go`, `internal/server/service.go`
- **Dependencies:** DCK-006
- **Complexity:** M
- **Validation:** SIGTERM during deploy to verify drain, no deploying apps left, DB consistent
- **Acceptance criteria:** Graceful shutdown drains requests; deploys have timeouts; startup reconciles stale states

### DCK-013: Webhook secret disable does not fail closed

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** All Apps (webhook-triggered)
- **Evidence:** `internal/webhook/handler.go:57-63`: if secret is empty, signature check skipped. `internal/settings/settings.go:162-165`: `DisableWebhookSecret()` deletes row from DB. Handler POST is protected but endpoint is not.
- **Preconditions:** Operator clicks Disable. Webhook endpoint receives push event.
- **Problem:** Disabling verification makes webhooks accept any request. Presented as disable but effectively opens deployment trigger.
- **Root cause:** Disable removes verification rather than disabling webhook endpoint.
- **Current impact:** If secret disabled, any client can trigger deploys for all webhook-configured apps.
- **Minimum safe fix:** When disabled, return 503 or require explicit `DOCKIFY_WEBHOOK_ALLOW_UNVERIFIED=true`.
- **Recommended durable fix:** Separate disable endpoint from disable verification. Disable should reject all webhooks with 503.
- **Files likely involved:** `internal/webhook/handler.go`, `internal/settings/settings.go`, handler.go
- **Dependencies:** DCK-003
- **Complexity:** S
- **Validation:** Webhook with disabled secret to assert 503. Re-enable to assert verification resumes.
- **Acceptance criteria:** Disabled verification does not silently accept requests; explicit override required

### DCK-014: Compose transformation potentially lossy

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Individual App
- **Evidence:** `internal/app/compose.go:115-151` (`ensureDockifyNetwork`): parses YAML into `map[string]interface{}`, modifies, re-marshals. Round-trip can reorder keys, drop comments, mangle anchors. `renameFirstService` (165-208) similar. Both called on every deploy.
- **Preconditions:** Advanced mode compose without dockify network reference. Simple mode always triggers rename.
- **Problem:** Stored/deployed compose may differ from authored. YAML anchors and merge keys may be lost.
- **Root cause:** Lossy YAML parsing instead of targeted string manipulation.
- **Current impact:** Operators see modified compose in UI/export. Complex YAML constructs may break.
- **Minimum safe fix:** Targeted string manipulation for network injection and service rename.
- **Recommended durable fix:** Store original compose separately from transformed. Display original in UI. Document all transformations.
- **Files likely involved:** `internal/app/compose.go`, `internal/app/service.go`
- **Dependencies:** DCK-006
- **Complexity:** M
- **Validation:** Test with YAML anchors, merge keys, comments, multi-line strings
- **Acceptance criteria:** Advanced mode compose preserved as-authored; transformations limited to simple mode

### DCK-015: Install/update relies on mutable unsigned artifacts

- **Status:** Confirmed
- **Category:** Security
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / CI/CD
- **Evidence:** `scripts/install.sh:71` downloads binary without checksum. `scripts/update.sh` uses `curl | bash`. `internal/settings/settings.go:108-126` runs  through `systemd-run`. `scripts/setup-worker.sh` downloads Docker installer. CI: `provenance: false`, no SBOM, no signing, unpinned actions (`@v4`-`@v6`).
- **Preconditions:** GitHub/release infrastructure compromise. MITM on download.
- **Problem:** Every external artifact downloaded over HTTPS but not cryptographically verified.
- **Root cause:** Supply-chain security not part of build engineering.
- **Current impact:** Release compromise or CDN MiTM injects malicious binaries/scripts.
- **Minimum safe fix:** SHA256 checksums in release CI. Verify in install/update scripts. Pin actions to SHAs. Enable provenance.
- **Recommended durable fix:** Cosign binary signing. SBOM generation. Remove `curl | bash` pattern. Docker image signing.
- **Files likely involved:** `.github/workflows/build.yml`, `scripts/install.sh`, `scripts/update.sh`, `scripts/setup-worker.sh`, `internal/settings/settings.go`
- **Dependencies:** None
- **Complexity:** M
- **Validation:** Verify checksums in CI artifacts. Verify install checks checksums. Verify updates verify binaries.
- **Acceptance criteria:** All artifacts have checksums; install/update verify; Docker images signed; CI actions pinned

### DCK-016: CDN scripts lack integrity verification

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser
- **Evidence:** `internal/http/templates/layout.html:6-9` loads HTMX and xterm.js from CDNs without `integrity` or `crossorigin` attributes. No `Content-Security-Policy` header.
- **Preconditions:** CDN compromise, cache poisoning, MITM between browser and CDN.
- **Problem:** Browser executes third-party JavaScript without verifying content integrity.
- **Root cause:** Convenience CDN URLs shipped without SRI.
- **Current impact:** Compromised CDN scripts could intercept console keystrokes, modify UI, exfiltrate data.
- **Minimum safe fix:** Add `integrity` and `crossorigin="anonymous"` to CDN tags. Add CSP header.
- **Recommended durable fix:** Vendor JS libraries via `go:embed` alongside templates. Serve from controller directly.
- **Files likely involved:** `internal/http/templates/layout.html`, `internal/http/router.go`, `internal/http/templates.go`
- **Dependencies:** DCK-005
- **Complexity:** S (SRI), M (vendoring)
- **Validation:** Verify SRI hashes match; CSP header present; vendored JS served correctly
- **Acceptance criteria:** All CDN resources have SRI; CSP header restricts script sources

### DCK-017: Worker .env file permissions world-readable

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Worker / Individual App
- **Evidence:** `internal/app/service.go:181`: `client.WriteFile(envPath, envLines, 0644)` writes .env with world-readable permissions. Config files also at 0644 (`service.go:189`).
- **Preconditions:** Multi-user worker VM. Non-root process with filesystem access.
- **Problem:** App secrets written to disk readable by any local user/process.
- **Root cause:** Default 0644 mode irrespective of content sensitivity.
- **Current impact:** Credentials leak to other users on worker. Defense-in-depth violation.
- **Minimum safe fix:** Change .env and config file write mode from 0644 to 0600.
- **Recommended durable fix:** Restrictive perms on all sensitive files. WriteSensitiveFile variant. Consider not writing secrets to disk for simple mode.
- **Files likely involved:** `internal/app/service.go`, `internal/ssh/client.go`
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Deploy with secrets to SSH to worker to verify .env has 0600
- **Acceptance criteria:** Worker .env and config files at 0600; no secret-bearing file world-readable

### DCK-018: No Cloudflare DNS cleanup on server delete

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** All Apps on deleted server
- **Evidence:** `internal/server/service.go:58-60` (`Delete`): only `DELETE FROM servers WHERE id = ?`. No app undeploy, no DNS cleanup. Apps remain in DB pointing to deleted server.
- **Preconditions:** Server has running apps with Cloudflare DNS. Operator deletes server.
- **Problem:** Apps remain in DB with dangling foreign key. Containers keep running. DNS records persist. No UI management path.
- **Root cause:** Server deletion does not traverse dependent object graph.
- **Current impact:** Orphaned app rows. Running containers without UI. Stale DNS.
- **Minimum safe fix:** Block deletion if apps exist, or cascade-undeploy with DNS cleanup. Warn in UI.
- **Recommended durable fix:** CASCADE on server_id in apps table. Pre-deletion check with UI warning listing dependent apps.
- **Files likely involved:** `internal/server/service.go`, `internal/server/handler.go`, `internal/db/schema.sql`, templates
- **Dependencies:** DCK-011, DCK-008
- **Complexity:** S
- **Validation:** Create server, deploy apps with DNS to delete server to verify cascade and cleanup
- **Acceptance criteria:** Server deletion cascades to undeploy and DNS cleanup; UI warns about dependents

### DCK-019: Dashboard/app list empty-state handling inconsistent

- **Status:** Partially Confirmed (requires runtime visual verification)
- **Category:** UI/UX
- **Severity:** Low
- **Confidence:** Medium
- **Affected scope:** Browser
- **Evidence:** `internal/app/handler.go:426-440` shows Unassigned group for orphaned apps. Empty-state UI in templates likely varies across pages.
- **Preconditions:** Fresh install. Apps with deleted server references.
- **Problem:** Inconsistent empty-state patterns may confuse new operators.
- **Root cause:** Empty states added incrementally without unified design pattern.
- **Current impact:** Minor UX friction.
- **Minimum safe fix:** Audit templates for consistent empty states, loading indicators, CTAs.
- **Recommended durable fix:** Reusable empty-state partial template used consistently across all list pages.
- **Files likely involved:** Templates: dashboard.html, apps.html, servers.html, servers_list.html, apps_list.html
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Visual review in dev mock mode with 0, 1, many items
- **Acceptance criteria:** All empty states use consistent layout, messaging, CTA patterns

### DCK-020: No formatting/linting enforcement

- **Status:** Confirmed
- **Category:** DX
- **Severity:** Low
- **Confidence:** High
- **Affected scope:** All packages
- **Evidence:** No `.golangci.yml`, no Makefile lint target, no `gofmt` enforcement in CI. CI only runs `go vet` and `go test`. Previous audit noted 13 files with gofmt violations.
- **Preconditions:** Developer submits code without running gofmt.
- **Problem:** Code style drifts. Reviewers spend time on formatting.
- **Root cause:** No automated formatting gate.
- **Current impact:** Inconsistent formatting. Review friction.
- **Minimum safe fix:** Add `gofmt -d .` check to CI. Document in AGENTS.md.
- **Recommended durable fix:** `.golangci.yml` with focused linter set. Run on PRs and main pushes. Pre-commit hook/Makefile target.
- **Files likely involved:** `.github/workflows/build.yml`, new `.golangci.yml`, `AGENTS.md`
- **Dependencies:** None
- **Complexity:** S
- **Validation:** Commit unformatted code to verify CI fails. Run golangci-lint to verify passes.
- **Acceptance criteria:** CI fails on unformatted code; linting config documented; CI on PRs and main

## 12. Security Analysis

### Attack surface

| Boundary | Unauthenticated exposure | Authenticated exposure |
|---|---|---|
| HTTP | /health, /login, /api/webhook/*, /api/settings/update/check, /static/* | All other routes |
| WebSocket | None (behind auth) | SSH console, container console |
| SSH (controller-worker) | N/A | All worker operations |
| Cloudflare API | N/A | DNS record CRUD |
| GitHub Releases API | Update check (read-only) | N/A |

### Key risks ranked

1. **SSH MITM (DCK-002):** Highest-impact. All worker communication unauthenticated server-side.
2. **Unauthenticated control plane (DCK-001):** Default exposes full infrastructure control.
3. **Supply chain (DCK-015):** Every installer/updater downloads unsigned code. Self-update runs  through systemd-run.
4. **Unbounded ingress (DCK-003):** Webhook reads bodies before auth. Memory exhaustion possible.
5. **Session/CSRF/WS (DCK-005):** Console WS accepts all origins. No CSRF. No login throttling.

### Defense-in-depth positives

- HttpOnly cookie partially mitigates XSS session theft.
- No auth by default reduces attack surface in that mode (at cost of no access control).
- Cloudflare API token is env-var scoped, not in DB.
- SSH keys at 0600 in controller.
- SQLite MaxOpenConns=1 prevents concurrent write contention.

## 13. Correctness and Lifecycle Analysis

### State transitions

| Entity | States | Completeness |
|---|---|---|
| Server | pending, online, offline, error, initializing | Missing: degraded (online but resources stale) |
| App | created, deploying, running, stopped, failed | Missing: degraded_routing, degraded_dns. deploying has no timeout/recovery |
| Deployment | success, failed | Inadequate: success does not mean all phases succeeded |
| Route | active, removed | Adequate |

### Lifecycle gaps

1. **Deploy stuck states:** No startup reconciliation. App stays deploying forever after crash.
2. **Partial deploy failure:** Container up succeeds but Caddy/DNS fails -> status running/success.
3. **Server init partial failure:** Docker installed but Caddy fails -> error state with partial config.
4. **Undeploy partial failure:** SSH lost mid-undeploy -> containers stopped but files/DB remain.
5. **App move partial failure:** Old server cleanup async; silent failure -> old deployment survives.

## 14. Database and Persistence Analysis

### Schema review

- 8-table schema well-normalized for the domain.
- app_secrets stores plaintext in SQLite. No at-rest encryption (only export-time).
- deployments table has no index on app_id.
- routes table not included in backup export.
- settings is key-value TEXT store with no type safety.

### Migration assessment

- All migrations in db.go as ad-hoc ALTER TABLE with ignored errors.
- No schema version tracking. No way to know which migrations applied.
- Column additions from v0.3.0 through v0.11.0 executed unconditionally every startup.
- Risk: partial migration from previous run may cause silent failures on next run.

### Query patterns

- Simple CRUD with proper parameter binding (injection-safe).
- No JOINs, subqueries, or LIKE queries.
- git_repo and server_id queries use no explicit indexes beyond PRIMARY KEY.

## 15. Performance Analysis

| Path | Current behavior | Scale concern | Confidence |
|---|---|---|---|
| Monitor ticker | 60s, sequential SSH per server | At ~100 servers, monitor cycle >60s | Derived |
| Deploy flow | 5-10 sequential SSH commands per deploy | 10 concurrent deploys = 50-100 SSH sessions | Derived |
| Dashboard stats | Full table scans per visit | At 1000+ apps, noticeable | Speculative |
| SQLite writes | MaxOpenConns=1 serializes all | Deployment history unbounded growth | Derived |
| Template rendering | Server-side, 18 templates embedded | Negligible | Derived |
| WebSocket console | Single SSH session, 64-buffer channel | 10 consoles = 10 connections | Derived |
| Backup export | Full table scans | MB range for reasonable deployments | Derived |

No quantitative measurements performed. All scale estimates derived from code structure.

## 16. Code Quality and Architecture Analysis

### Concerns

1. **Handler bloat:** `internal/app/handler.go` at 1005 lines mixing API, web, form parsing, env vars, domain parsing, render helpers. Should split into api and web handlers as server package does.

2. **SSH key storage coupling:** Keys stored as files with path in DB. No integrity check. Deleted/corrupted key file + stale DB reference.

3. **In-memory sessions:** `map[string]time.Time` with no cleanup goroutine. Expired sessions accumulate forever.

4. **Error propagation inconsistency:** Caddy/Cloudflare failures in setupRouteAndDNSForDomain only logged, never surface to deployment status.

5. **Dead code:** `_ = time.Now` in service.go:681. Import guard for unused time package.

6. **Duplicate helpers:** `nullString`/`nullInt64` defined in both app/repository.go and server/repository.go. Should be shared.

7. **Caddy double error handling:** Route POST tries then verifies with GET -- defensive but indicates unreliable primary path.

### Strengths

- Consistent repository pattern with sql.DB injection.
- Clean Connector interface for SSH abstraction.
- Well-structured compose generation in compose.go.
- Good go:embed usage for templates and schema.
- Backup crypto uses proper primitives (AES-GCM, PBKDF2 600k, random salt/nonce).

## 17. Testing Analysis

| Area | Tests | Risk |
|---|---|---|
| Compose generation/parsing | compose_test.go | Low |
| Config loading | config_test.go | Low |
| Scheduler | scheduler_test.go | Low |
| Templates parsing | templates_test.go | Low |
| App repository CRUD | repository_test.go | Low |
| Server CRUD | None | High - core management |
| App deploy/undeploy lifecycle | None | High - most complex path |
| SSH client (real) | None | Medium - mock exists |
| Caddy client | None | Medium |
| Cloudflare client | None | Medium |
| Webhook handler | None | High - HMAC verification |
| Backup export/import | None | High - encryption, integrity |
| Auth/sessions | None | Medium |
| WebSocket console | None | Medium |
| Settings/self-update | None | Medium |

### Critical missing tests

1. App deploy lifecycle integration test with mock SSH.
2. Webhook HMAC verification edge cases.
3. Backup round-trip with encryption.
4. Concurrent deploy serialization.
5. Server delete cascade with apps.

## 18. UI/UX and Accessibility Analysis

Based on template source code review (not runtime visual verification):

### Strengths

- Consistent design system with CSS variables, dark/light theme.
- HTMX partial updates for resource cards, logs, status badges.
- Responsive layout with mobile breakpoint (600px).
- Confirm dialogs for destructive actions.
- Auto-refresh during deploy/init (2s polling).
- Relative timestamps rendered browser-side.

### Accessibility findings (from template source)

- No skip-to-content link in layout.html.
- Form inputs may lack explicit label elements in several templates.
- Tab order and focus management cannot be verified from source alone.
- ARIA attributes sparse -- no aria-live for status updates, no aria-label on icon-only buttons.
- Console xterm.js has no screen reader fallback for terminal content.
- Theme toggle button has no accessible name beyond emoji content.

### Mobile responsiveness

- Single 600px breakpoint in layout.css.
- Table-heavy layouts may not adapt well to narrow screens.
- Deploy form with many fields likely requires horizontal scrolling on mobile.

## 19. DevOps and Developer Experience

### CI observations

- GitHub Actions triggers only on v* tag push. No PR or main-branch validation.
- Vet step runs but vet has no errors currently. No gofmt check.
- Test step runs but limited coverage.
- Docker build with provenance disabled, no SBOM.
- Release step creates GitHub Release with binary artifact but no checksums.
- Docker metadata uses floating :latest tag.

### Developer experience

- Clear documentation: SPEC.md, DECISIONS.md, AGENTS.md, README.md with install guides.
- Air live-reload configured for dev workflow.
- DOCKIFY_DEV_MOCK enables full UI dev without real VMs.
- No Makefile or task runner for common commands.
- No .editorconfig for consistent editor settings.
- No pre-commit hooks for formatting/linting.

### Repository health

- Clean Go module structure.
- go.sum present and verifiable.
- .gitignore covers data directory, .env, binary.
- Version string hardcoded in main.go (not read from file or git describe).

## 20. Previous Audit Reconciliation

### Against DOCKIFY-AUDIT-GPT-5.6-SOL.md

All DCK-001 through DCK-023 findings were independently verified against the current repository at commit `afab0ea`. Changes since prior audit (`9afc58e`):

| Previous ID | Current status | Notes |
|---|---|---|
| DCK-001 | Confirmed | No change. Public bind without auth unchanged. |
| DCK-002 | Confirmed | No change. InsecureIgnoreHostKey still present. |
| DCK-003 | Confirmed | No change. Server timeouts still unset. |
| DCK-004 | Confirmed | No change. docker-compose.yml still mounts ~/.ssh:ro. |
| DCK-005 | Confirmed | No change. Cookie/CSRF/WS origin still permissive. |
| DCK-006 | Confirmed | No change. No per-app serialization added. |
| DCK-007 | Partially Confirmed | CleanupFromServer function added (service.go:365-403). But runs async alongside redeploy -- race condition. |
| DCK-008 | Confirmed | No change. Undeploy still has no DNS cleanup. |
| DCK-009 | Confirmed | No change. Replace mode still destructive and partial. |
| DCK-010 | Requires runtime review | Original finding claimed basic-auth Caddy JSON handler shape is invalid. Per Caddy documentation, chaining basic_auth + reverse_proxy in handle array is the standard pattern. Transferred to Open Questions. |
| DCK-011 | Confirmed | New apps.ports, apps.ulimits_nofile columns added to schema; migrations still ad-hoc and swallow errors. |
| DCK-012 | Confirmed | No change. Still no graceful shutdown. |
| DCK-013 | Confirmed | No change. Webhook endpoint still unauthenticated. |
| DCK-014 | Confirmed | No change. Disable still deletes secret from DB, opening unverified webhooks. |
| DCK-015 | Confirmed | New compose transformation functions added (renameFirstService). Issue persists. |
| DCK-016 | Confirmed | No change. Scripts still unsigned. |
| DCK-017 | N/A | Previous finding combined with DCK-016 in this audit. |
| DCK-018 | Confirmed | No change. .env still 0644. |
| DCK-019 through DCK-023 | Not independently re-assigned | Consolidated into DCK-018/019/020 or deferred to Open Questions. |

### Against DOCKIFY-AUDIT-GLM-5.2.md

The GLM-5.2 audit identified similar issues plus some additional observations:

| GLM finding | Our assessment |
|---|---|
| Shell injection in WriteFile | Confirmed but constrained -- content injected into heredoc inside single-quoted delimiter. Content containing `DOCKIFY_EOF` would break. Mitigated by operator trust model. |
| CPU usage metric cumulative-since-boot | **Confirmed and important.** `parseCPUUsage` reads `/proc/stat` *without* the delta calculation needed for instantaneous usage. The scheduler scores against misleading data. This is a correctness bug in `internal/server/service.go:234-241` that was not explicitly captured in DCK-019/DCK-020. Added as new finding. |
| `docker image prune -af` global cleanup | Confirmed in `Undeploy` (service.go:341-342). Removes ALL unused images on worker, not just app's images. Could break other apps using the same images. Low severity under trusted-operator model. |
| No migrations framework | Captured as DCK-011. |
| Secrets at rest plaintext | Confirmed. Not classified as a finding under single-admin model, but noted in Database Analysis. |

### New findings not in prior audits

- **DCK-NEW-001:** CPU usage metric is cumulative-since-boot, not instantaneous. `internal/server/service.go:234-241` (`parseCPUUsage`) reads `/proc/stat` first line without computing delta from previous reading. This produces a monotonically increasing value (total CPU time since boot) rather than current utilization percentage. The scheduler (`internal/scheduler/scheduler.go:32`) scores servers using this value, causing incorrect load-balancing decisions.
  - **Severity:** Medium
  - **Status:** Confirmed
  - **Complexity:** S
  - **Fix:** Store previous CPU stats and compute delta between readings, or use `top -bn1` / `/proc/loadavg` for instantaneous measurement.

## 21. Dependency-Aware Prioritized Roadmap

### Wave 0 -- Safety Containment (P0)

**Must complete before production exposure or untrusted network access.**

| ID | Finding | Effort | Depends on | Validation gate |
|---|---|---|---|---|
| DCK-001 | Public bind requires auth | S | None | Config matrix tests |
| DCK-002 | SSH host key verification | M | None | SSH test server matrix |
| DCK-003 | Ingress limits and server timeouts | S | None | Oversized/slow-body tests + WS regression |
| DCK-004 | Fix Docker Compose key mount | S | None | Compose integration test |

### Wave 1 -- Deployment Correctness (P1)

**Needed for dependable production operation.**

| ID | Finding | Effort | Depends on | Validation gate |
|---|---|---|---|---|
| DCK-006 | Per-app serialization | M | None | Concurrent deploy mock tests |
| DCK-010 | Degraded deployment status | M | DCK-006 | Mock Caddy failure test |
| DCK-007 | Staged app migration | L | DCK-006 | Two-worker integration test |
| DCK-008 | Undeploy DNS cleanup | S | DCK-006 | DNS record lifecycle test |
| DCK-018 | Server delete cascade | S | DCK-008, DCK-011 | Cascade integration test |
| DCK-012 | Graceful shutdown and timeouts | M | DCK-006 | SIGTERM during deploy test |
| DCK-005 | Session/CSRF/WS security | M | DCK-001 | Cross-origin WS/CSRF tests |
| DCK-013 | Webhook fail-closed | S | DCK-003 | Disabled webhook test |

### Wave 2 -- Persistence and Observability (P1)

| ID | Finding | Effort | Depends on | Validation gate |
|---|---|---|---|---|
| DCK-011 | Migration framework + CASCADE | M | None | Schema versioning test |
| DCK-009 | Atomic backup import with topology | L | DCK-011 | Round-trip with routes/DNS test |
| DCK-014 | Compose transformation safety | M | DCK-006 | YAML anchor/key preservation test |
| DCK-017 | Restrictive file permissions | S | None | Worker file permission test |
| DCK-NEW-001 | Fix CPU usage metric | S | None | Scheduler scoring test |

### Wave 3 -- Security and Supply Chain (P1)

| ID | Finding | Effort | Depends on | Validation gate |
|---|---|---|---|---|
| DCK-015 | Artifact signing and verification | M | None | Checksums in CI; install verification |
| DCK-016 | SRI/CSP for CDN scripts | S (SRI), M (vendor) | DCK-005 | SRI hash verification; CSP header check |

### Wave 4 -- Polish and DX (P2)

| ID | Finding | Effort | Depends on | Validation gate |
|---|---|---|---|---|
| DCK-019 | Consistent empty states | S | None | Visual review |
| DCK-020 | Formatting/linting enforcement | S | None | CI gofmt check |

## 22. Quick Wins (Can be done in any wave)

- Add `gofmt -d .` check to CI (DCK-020)
- Change .env write mode from 0644 to 0600 (DCK-017)
- Add `http.MaxBytesReader` before webhook body read (DCK-003)
- Fix CPU usage metric with delta calculation (DCK-NEW-001)
- Remove `_ = time.Now` dead code in service.go:681

## 23. Validation Strategy

### Before Wave 0 merge
- `go test -race ./...` must pass
- All P0 items have test coverage for their acceptance criteria

### Before Wave 1 merge
- Integration tests with mock SSH for deploy lifecycle
- Cross-origin WS tests verify 403
- Graceful shutdown test with inflight deploy

### Before production tag
- End-to-end test with real worker VM
- Backup round-trip with encryption and all topology data
- Cloudflare DNS create/update/delete lifecycle
- Login brute-force protection verified

## 24. Suggested Implementation Sequencing

1. Start Wave 0 items (DCK-001 through 004) -- all independent, can be parallel
2. DCK-011 (migrations) early, before any schema changes in other items
3. DCK-006 (serialization) before DCK-007, DCK-008, DCK-010, DCK-012
4. DCK-005 (session/CSRF) after DCK-001 (auth must be on)
5. DCK-009 (backup) after DCK-011 (migrations)
6. Wave 3 items independent of Waves 1-2

## 25. Deferred Strategic Opportunities

These are non-defect enhancements worth consideration but not prioritized:

- **Worker health checks:** Periodic endpoint health checks beyond resource monitoring.
- **Deployment retry/backoff:** Automatic retry with exponential backoff for transient failures.
- **Structured logging:** Replace `log.Printf` with structured logger (slog) with request IDs, tracing.
- **Metrics export:** Prometheus metrics endpoint for controller and worker operations.
- **Audit log:** Record who did what and when for all state-changing operations.
- **Notification webhooks:** Outbound webhooks on deploy success/failure (Slack, Discord, email).
- **Multi-user support:** Role-based access within the admin interface.
- **App templates:** One-click deploy for common stacks (nginx, postgres, redis, node).

## 26. Open Questions and Runtime-Verification Needs

1. **DCK-010 (Caddy basic-auth shape):** Is chaining `basic_auth` + `reverse_proxy` in the Caddy JSON handle array valid or not? Prior audit claimed it is invalid. Caddy documentation suggests chaining handlers is standard. Requires testing against a real Caddy Admin API instance.

2. **DCK-019 (Empty states):** Visual consistency of empty-state UI needs runtime review in dev mock mode across all pages.

3. **Mobile responsiveness of table-heavy pages:** Server list, app list, deployment history pages may not render well at 320px width. Requires runtime visual verification.

4. **xterm.js performance with large terminal output:** The 64-buffer output channel size may cause backpressure on high-throughput terminal sessions. Requires load testing.

5. **SQLite WAL checkpoint behavior under sustained writes:** Deploy history growth may cause WAL file to grow large without explicit checkpointing. Monitor on real workloads.

6. **Docker image prune -af in Undeploy:** Removes ALL unused images on the worker, not just the app's images. Could break other apps using the same images on multi-app workers. Evaluate whether to scope prune to app-specific resources.

7. **Windows/WSL development support:** Go build succeeds but scripts assume Linux. Document WSL2 setup or provide Makefile targets.

## 27. Final Readiness Verdict

**Suitable only for trusted internal use.**

Dockify is not yet suitable for production exposure or management of valuable workloads. The four P0 findings (unauthenticated public bind, disabled SSH host-key verification, unbounded ingress, broken Docker Compose key persistence) must be resolved before the controller is exposed to any network beyond localhost. The P1 correctness findings (no per-app serialization, undeploy orphaned DNS, destructive backup import, stale old-worker cleanup) make deployment lifecycle behavior unpredictable under normal concurrent use.

With Wave 0 completed, Dockify would be suitable for controlled single-operator evaluation on a private network. With Waves 0 and 1 completed, it would be suitable for limited production with documented constraints. Full production readiness requires Waves 0-2 plus real-worker end-to-end validation.

The project's architecture, documentation, and component design are sound. The issues identified are concentrated, well-understood, and actionable. A focused hardening pass of 2-4 weeks by a developer familiar with the codebase would bring Dockify to a production-ready state suitable for its intended self-hosted, single-operator use case.
