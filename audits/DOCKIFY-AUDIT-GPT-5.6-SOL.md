# Dockify Independent Repository Audit and Executable Improvement Roadmap

## 1. Audit metadata

| Field | Value |
|---|---|
| Audit date | 2026-07-13 (Asia/Jakarta) |
| Repository | `github.com/coderbuzz/dockify` |
| Branch / commit | `main` / `9afc58e0ea38755eaeecbb1a550fcea4e113c4a0` (`v0.3.63-4-g9afc58e`) |
| Working-tree baseline | Clean |
| Auditor | GPT-5.6, independent repository pass |
| Deliverable | Audit and roadmap only; no application code was changed |
| Prior material | `AUDIT.md`, read only after the independent pass |

## 2. Scope and methodology

The audit covered the controller, SQLite schema and migrations, SSH worker orchestration, Docker Compose lifecycle, Caddy and Cloudflare integrations, webhook and browser trust boundaries, backup/restore, release/install paths, tests, documentation, and the rendered UI. The review reconstructed product behavior from `README.md`, `SPEC.md`, `DECISIONS.md`, scripts, workflows, templates, and implementation code; traced destructive and asynchronous paths; ran non-destructive verification; and exercised the primary UI flow in `DOCKIFY_DEV_MOCK=true` against temporary data outside the repository.

Severity reflects the documented single-admin/trusted-operator model. Administrator-controlled compose, domains, and config files are therefore treated as correctness and containment inputs, not automatically as hostile multi-tenant input. Findings are included only where the full evidence schema can be supported.

Limitations: no real worker VM, Cloudflare zone, production reverse proxy, Caddy Admin API instance, or release installation was mutated. Runtime-only behavior is called out explicitly. Dependency vulnerability scanners were unavailable locally.

## 3. Repository baseline

| Area | Baseline |
|---|---|
| Language/runtime | `go 1.25.0`; audit host `go1.26.0 darwin/arm64`; CGO enabled locally |
| Main dependencies | chi `v5.3.0`, gorilla/websocket `v1.5.3`, x/crypto `v0.53.0`, yaml.v3 `v3.0.1`, modernc SQLite `v1.53.0` |
| Process model | One controller binary, in-memory sessions, SQLite, raw background goroutines |
| Frontend | Embedded Go templates, HTMX, xterm.js, one custom CSS system |
| Worker model | Root-capable SSH, Docker Compose, per-worker Caddy container |
| Deployment modes | Docker Compose controller+Caddy; native binary; native binary+Caddy; source/dev |
| Persistence | SQLite WAL, foreign keys, one open DB connection, controller key files, worker app directories |
| External systems | Worker SSH, GitHub/GitLab webhooks, GitHub Releases, Cloudflare API, public CDNs |
| Tests present | app compose/repository, config, scheduler, template parse/render |

The repository is compact and understandable. Most risk concentrates in a few orchestration methods (`app.Service.deployWithCommit`, `Undeploy`, backup import, SSH console) and in production defaults.

## 4. Verification commands and results

| Command/check | Result | Notes |
|---|---|---|
| `go test ./...` | Pass | app/config/http/scheduler passed; other packages have no test files |
| `go test -race ./...` | Pass | No race reported in the exercised tests; high-risk paths are mostly untested |
| `go vet ./...` | Pass | No diagnostics |
| `go build ./...` | Pass | Initial default module-cache stat write warned due audit sandbox permissions; isolated caches succeeded elsewhere |
| `go test ./internal/http/... -run TestTemplates` | Pass | Templates parse/render |
| `go mod verify` | Pass | `all modules verified` |
| `git diff --check` | Pass | No whitespace errors in tracked changes |
| `gofmt -l $(rg --files -g '*.go')` | Fail | 13 Go files listed, including controller, SSH, app, Caddy, webhook, auth, and tests |
| `govulncheck`, `staticcheck`, `gosec`, `golangci-lint` | Not run | Executables not installed; no repository configuration provides them |
| UI flow, desktop and 390 px mobile | Completed | Login, dashboard, add/list server, deploy app, app detail; no console error observed on app detail |

The tests prove that current covered behavior builds and runs, not that deployment/recovery paths are safe. No real SSH/Caddy/Cloudflare end-to-end test was performed.

## 5. Product and trust-model understanding

Dockify is a self-hosted control plane for a trusted operator to add Linux workers, deploy Docker Compose applications, expose domains through Caddy, optionally manage Cloudflare DNS, receive Git webhooks, use interactive consoles, and back up controller configuration.

Primary trust boundaries:

- Public network to controller: login, webhook, update-check, headers, and request bodies.
- Authenticated browser to controller: state-changing forms/APIs and WebSocket consoles.
- Controller to worker: SSH server identity, root-capable commands, secrets, and files.
- Controller/worker to supply chain: GitHub releases, raw scripts, Docker installer, container images, CDNs, and Git repositories.
- Controller database/filesystem: app secrets, basic-auth passwords, SSH key paths/files, backup material.

The single administrator is trusted and already receives root-capable server and container consoles. Compose, route, file, and domain inputs are nevertheless expected to remain structurally valid and contained so mistakes cannot silently corrupt unrelated worker state.

## 6. Current architecture

```text
Browser
  -> chi HTTP router / templates / JSON / WebSocket
       -> server.Service -> SSH -> worker Docker + Caddy
       -> app.Service -> SQLite + scheduler + Caddy/Cloudflare
       -> webhook.Handler -> background app deploys
       -> backup.Service -> YAML + optional field encryption
       -> settings.Service -> GitHub Releases + self-update

Controller persistence
  -> SQLite: servers, apps, deployments, routes, dns_records,
             app_secrets, app_files, settings
  -> key directory: per-server private keys

Background activity
  -> 60-second server monitor
  -> unbounded deploy and initialization goroutines
```

The narrow package layout and SSH connector abstraction are useful. The dominant architectural weakness is that multi-step remote and database operations have no operation coordinator or durable reconciliation state.

## 7. Critical architectural invariants

1. A worker's SSH identity must be authenticated before secrets or root commands are sent.
2. At most one lifecycle operation may mutate a given app at a time, and stale completions must not overwrite newer state.
3. Database state, remote Compose state, Caddy routes, and Cloudflare records must converge or retain enough durable state to retry safely.
4. `running`/`success` must mean required deployment phases succeeded; optional integrations need an explicit degraded state.
5. App moves must preserve the old placement until the new placement is healthy, then clean up the old placement.
6. Restore/replace must be validated and staged before destructive mutation and must preserve all routable topology.
7. Public request work and remote commands must be bounded by size, time, and concurrency.
8. Production exposure must fail closed unless authentication is deliberately configured or explicitly overridden.
9. Worker files must remain within the app directory and secret material must have least-readable permissions.
10. Schema changes must be versioned, ordered, transactional where possible, and fail visibly.

## 8. Executive assessment

Dockify is a coherent and promising small control plane, but the audited revision is not suitable for production exposure. Four immediate blockers dominate: it binds publicly without requiring authentication, does not authenticate SSH host keys, accepts unbounded public webhook bodies with an untimed HTTP server, and its documented Docker Compose installation mounts the configured SSH-key directory read-only so worker onboarding cannot persist keys.

Production correctness is also not yet dependable. Concurrent deploys can interleave, moving an app leaves the old worker running, undeploy can claim success while leaving DNS or remote resources, backup replace is partial and lossy, and basic-auth routes are emitted using a Caddy JSON handler shape that the standard Caddy API does not support. These are concrete lifecycle failures, not polish gaps.

The project is suitable for local development and controlled evaluation after operators understand the constraints. It should not manage valuable workloads or be exposed to untrusted networks until Wave 0 and Wave 1 of the roadmap are complete and validated against real workers.

## 9. Strengths worth preserving

- Clear product scope, small binary, straightforward package structure, and good operator documentation.
- SQLite is configured with WAL, foreign keys, busy timeout, and a single connection, a sensible baseline for a small controller.
- Templates are embedded and have parse/render tests; the custom design system is consistent and avoids framework sprawl.
- SSH and scheduler dependencies are abstracted enough to support deterministic tests.
- Backup sensitive fields use AES-GCM with random salt/nonce and a deliberately expensive PBKDF2 derivation.
- Cloudflare client requests have an HTTP timeout; SSH connection establishment has a timeout.
- The Docker image runs the controller as a non-root user.
- Dev mock mode enables a complete UI flow without real infrastructure.
- Release update scripts contain binary rollback behavior even though artifact provenance needs hardening.

## 10. Confirmed findings summary

### P0 and P1 findings

| Priority | ID | Severity | Finding | Primary dependency |
|---|---|---:|---|---|
| P0 | DCK-001 | High | Public bind can expose the entire control plane without authentication | None |
| P0 | DCK-002 | High | SSH host authenticity is disabled | None |
| P0 | DCK-003 | High | Public ingress and HTTP connection lifetime are unbounded | None |
| P0 | DCK-004 | High | Default Docker Compose install cannot persist worker keys | None |
| P1 | DCK-005 | Medium | Browser session, CSRF, and WebSocket origin controls are incomplete | DCK-001 |
| P1 | DCK-006 | High | App lifecycle operations are not serialized | None |
| P1 | DCK-007 | High | Changing an app's worker leaves the old deployment live | DCK-006 |
| P1 | DCK-008 | High | Undeploy can report success while resources remain orphaned | DCK-006, DCK-012 |
| P1 | DCK-009 | High | Backup replace is destructive, partial, and route-lossy | DCK-012 |
| P1 | DCK-010 | High | Basic-auth routes use an invalid Caddy JSON handler shape | DCK-011 |
| P1 | DCK-011 | Medium | Deployment status can say success after required phases fail | DCK-006 |
| P1 | DCK-012 | Medium | Schema migration and integrity controls are incomplete | None |
| P1 | DCK-013 | Medium | Remote operations and background workers lack lifecycle bounds | DCK-006 |
| P1 | DCK-014 | Medium | Webhook secret disable and database-error behavior do not fail closed | DCK-003 |
| P1 | DCK-015 | Medium | Compose and remote-file transformation is non-deterministic and weakly contained | DCK-006 |
| P1 | DCK-016 | High | Install/update/release trust relies on mutable, unsigned artifacts | None |
| P1 | DCK-017 | Medium | Browser code is trusted from CDNs without integrity or response policy | DCK-005 |
| P1 | DCK-018 | Medium | Secret storage and file permissions exceed required exposure | DCK-004, DCK-012 |

### Findings by severity

| Severity | Count | IDs |
|---|---:|---|
| Critical | 0 | — |
| High | 10 | DCK-001–004, DCK-006–010, DCK-016 |
| Medium | 12 | DCK-005, DCK-011–015, DCK-017–022 |
| Low | 1 | DCK-023 |
| Opportunity | 0 | Strategic opportunities are deferred in §26 |

### Findings by primary category

| Category | Count | IDs |
|---|---:|---|
| Security | 7 | DCK-001, DCK-002, DCK-005, DCK-014, DCK-016–018 |
| Correctness | 8 | DCK-004, DCK-006, DCK-007, DCK-010, DCK-011, DCK-015, DCK-019, DCK-020 |
| Reliability | 3 | DCK-003, DCK-008, DCK-013 |
| Database / Backup | 2 | DCK-009, DCK-012 |
| UI/UX / Accessibility | 1 | DCK-021 |
| Testing / DevOps | 1 | DCK-022 |
| DX / Documentation | 1 | DCK-023 |

The table assigns one primary category per finding; detailed schemas record cross-category effects.

### Findings by confidence

| Confidence | Count | IDs |
|---|---:|---|
| High | 22 | DCK-001–009, DCK-011–023 |
| Medium | 1 | DCK-010 |
| Low | 0 | — |

## 11. Detailed confirmed findings

### `DCK-001` Default public bind can expose the entire control plane without authentication

- **Status:** Confirmed
- **Category:** Security
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / All Apps
- **Evidence:** `internal/config/config.go:24-33` defaults to `0.0.0.0` and an empty password; `internal/http/router.go:27,62-66` disables auth when the password is empty; `cmd/dockify/main.go:82-85` only logs a warning; `docker-compose.yml` does not require an admin password.
- **Preconditions:** Default or omitted `DOCKIFY_ADMIN_PASSWORD`; controller port reachable directly or through a proxy.
- **Problem:** A production-looking default exposes server keys, deploy controls, destructive actions, and root-capable consoles without authentication.
- **Root cause:** Authentication is opt-in while network binding is public by default.
- **Current impact:** One configuration omission grants complete control-plane access.
- **Failure or abuse scenario:** An operator launches the documented Compose stack on a public VM without setting the optional password; an Internet client opens a worker console and controls all workloads.
- **Existing mitigations:** Startup warning and README documentation; reverse proxies may add external auth.
- **Minimum safe fix:** Refuse to bind non-loopback without a password unless an explicit `DOCKIFY_ALLOW_UNAUTHENTICATED=true` override is set.
- **Recommended durable fix:** Make first-run credential creation mandatory for public binds and expose a clear trusted-local development mode.
- **Alternatives and trade-offs:** Loopback-only default preserves passwordless local use; external-auth mode needs an explicit trusted-proxy contract.
- **Files likely involved:** `internal/config/config.go`, `cmd/dockify/main.go`, `docker-compose.yml`, `scripts/install.sh`, docs.
- **Dependencies:** None.
- **Estimated implementation complexity:** S
- **Validation plan:** Config matrix tests for loopback/public bind, password present/absent, and explicit override.
- **Acceptance criteria:** Public bind without authentication exits with an actionable error; intentional override is logged prominently; documented install paths provision authentication.

### `DCK-002` SSH host authenticity is disabled

- **Status:** Confirmed
- **Category:** Security
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / All Apps
- **Evidence:** `internal/ssh/client.go:31-38` sets `HostKeyCallback: gossh.InsecureIgnoreHostKey()` for every real connection.
- **Preconditions:** Attacker can intercept or redirect controller-to-worker traffic, poison routing/DNS, or impersonate a reprovisioned worker address.
- **Problem:** Encryption occurs without authenticating the SSH server.
- **Root cause:** No host fingerprint is captured or verified during onboarding.
- **Current impact:** Secrets, compose content, private operations, and root commands can be sent to an impersonated host.
- **Failure or abuse scenario:** A worker IP is reassigned or traffic is intercepted; Dockify connects to the replacement and uploads app secrets because every host key is accepted.
- **Existing mitigations:** Public-key client authentication, encrypted SSH transport, and a 10-second connect timeout.
- **Minimum safe fix:** Require a configured fingerprint and reject mismatches.
- **Recommended durable fix:** Implement explicit fingerprint entry or TOFU enrollment with a reviewable fingerprint, immutable storage, rotation workflow, and mismatch audit event.
- **Alternatives and trade-offs:** System `known_hosts` integration is simpler but harder to manage inside the container; TOFU protects subsequent connections but not an already-intercepted first contact.
- **Files likely involved:** `internal/ssh/client.go`, server model/schema/forms, backup format, tests.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** SSH test server with expected, changed, unknown, and rotated keys.
- **Acceptance criteria:** Unknown/mismatched keys cannot receive commands or secrets; fingerprints are visible and rotation is deliberate.

### `DCK-003` Public ingress and HTTP connection lifetime are unbounded

- **Status:** Confirmed
- **Category:** Security / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller
- **Evidence:** `internal/webhook/handler.go:27-66,70-109` uses unbounded `io.ReadAll` before authentication; `cmd/dockify/main.go:88-91` configures no read-header, read, idle, or header-size limits. Backup upload also uses an unbounded read in `internal/backup/handler.go` after authentication.
- **Preconditions:** Webhook endpoint is reachable; attacker can hold connections or send a large body with the expected event header.
- **Problem:** An unauthenticated request can consume controller memory and connection resources before its signature is verified.
- **Root cause:** No centralized ingress limits and no per-route body cap.
- **Current impact:** Memory exhaustion, slowloris exposure, and controller outage are possible.
- **Failure or abuse scenario:** Several clients stream very large webhook bodies slowly; the process retains connections and buffers bodies until it is killed.
- **Existing mitigations:** GitHub/GitLab signatures for normal configured operation; update HTTP client has a timeout.
- **Minimum safe fix:** Apply `http.MaxBytesReader` before reads and set conservative server timeouts/header limits.
- **Recommended durable fix:** Define per-route size/time budgets, bounded webhook dispatch, structured 413/408 responses, and reverse-proxy limits aligned with application limits.
- **Alternatives and trade-offs:** A global read timeout must not break WebSockets; use compatible server settings plus handler-specific deadlines.
- **Files likely involved:** `cmd/dockify/main.go`, `internal/http/router.go`, `internal/webhook/handler.go`, `internal/backup/handler.go`.
- **Dependencies:** None.
- **Estimated implementation complexity:** S
- **Validation plan:** Oversized and slow-body HTTP tests plus a WebSocket regression test.
- **Acceptance criteria:** Oversized webhook/import requests are rejected before full buffering; slow headers/bodies expire; consoles still work.

### `DCK-004` Default Docker Compose install cannot persist worker keys

- **Status:** Confirmed
- **Category:** Correctness / DevOps
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller
- **Evidence:** `docker-compose.yml:9-16` sets `DOCKIFY_SSH_KEY_DIR=/home/dockify/.ssh` and mounts `${HOME}/.ssh` read-only; `Dockerfile:14-24` runs as user `dockify`; server create/import writes `<id>.pem` into that directory in `internal/server/handler.go` and `internal/backup/backup.go:341-349`.
- **Preconditions:** Documented Docker Compose installation; adding/importing a server key.
- **Problem:** The configured key directory is read-only and host private keys may also be unreadable to the container UID.
- **Root cause:** A host SSH-directory mount is reused as Dockify's writable key store.
- **Current impact:** The default install cannot complete worker onboarding and leaves a `pending` database row when the subsequent file write fails.
- **Failure or abuse scenario:** The operator installs with mode 1, submits a worker key, and gets a save failure while the server row remains orphaned.
- **Existing mitigations:** The named data volume already contains a writable `/var/lib/dockify/keys` directory; native installs use it by default.
- **Minimum safe fix:** Remove the host `~/.ssh` mount and use `/var/lib/dockify/keys` in the named data volume.
- **Recommended durable fix:** Keep per-server keys solely in the controller data volume, compensate database inserts on write failure, and document backup/restore ownership.
- **Alternatives and trade-offs:** A dedicated read-write key volume works too; importing an operator's entire host SSH directory is unnecessary and broadens exposure.
- **Files likely involved:** `docker-compose.yml`, `scripts/install.sh`, server create handlers, backup import, docs.
- **Dependencies:** None.
- **Estimated implementation complexity:** S
- **Validation plan:** Compose integration test adding a server key and checking ownership/mode across restart.
- **Acceptance criteria:** Documented Compose install can create/read key files; no host `~/.ssh` is mounted; failed persistence leaves no orphan row.

### `DCK-005` Browser session, CSRF, and WebSocket origin controls are incomplete

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser / Controller / Worker
- **Evidence:** `internal/http/console.go:18-21` accepts every origin; `internal/http/auth.go:41-68` omits explicit `Secure` and `SameSite`; state-changing routes have no CSRF token/origin middleware; login has no throttling.
- **Preconditions:** Authenticated browser plus a same-site attacker origin, browser/client behavior that sends the cookie, compromised sibling subdomain, or user interaction with a forged request.
- **Problem:** Powerful browser actions rely mainly on an implicit cookie default and do not independently validate request origin.
- **Root cause:** Authentication was added without a complete browser-session threat model.
- **Current impact:** Cross-site/same-site request and WebSocket defenses are inconsistent around root-capable actions.
- **Failure or abuse scenario:** A compromised sibling origin opens a console WebSocket or submits a destructive same-site request using the victim's session.
- **Existing mitigations:** `HttpOnly`; modern browsers generally default unspecified SameSite to Lax, which reduces ordinary cross-site form/subresource attacks; routes are authenticated when a password exists.
- **Minimum safe fix:** Strict same-origin WebSocket checks, explicit `SameSite=Lax/Strict`, `Secure` when HTTPS, and origin/CSRF checks for mutations.
- **Recommended durable fix:** Central session/CSRF middleware, login throttling, trusted-proxy-aware HTTPS detection, session expiry cleanup, and security-focused HTTP tests.
- **Alternatives and trade-offs:** Strict cookies can complicate unusual proxy/embedding setups; support them through explicit origin configuration, not permissive defaults.
- **Files likely involved:** `internal/http/auth.go`, `internal/http/console.go`, `internal/http/router.go`, templates.
- **Dependencies:** DCK-001.
- **Estimated implementation complexity:** M
- **Validation plan:** Same-origin/cross-origin WS tests, CSRF tests for every mutation family, cookie attribute assertions, rate-limit tests.
- **Acceptance criteria:** Cross-origin console upgrade and tokenless invalid-origin mutations return 403; valid proxied same-origin flows pass.

### `DCK-006` App lifecycle operations are not serialized

- **Status:** Confirmed
- **Category:** Correctness / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/service.go:115-127,127-239,439-505` has no per-app lock/lease; webhook, redeploy, rollback, edit, start/stop, and delete paths launch or execute overlapping operations from `internal/app/handler.go` and `internal/webhook/handler.go`.
- **Preconditions:** Two manual/webhook/scheduled operations overlap for one app.
- **Problem:** Remote `down`, file writes, `pull`, `up`, route mutation, and status writes can interleave.
- **Root cause:** Fire-and-forget goroutines call the same mutable workflow without operation identity or exclusion.
- **Current impact:** Stale status, wrong compose version, downtime, and orphaned routes are possible under normal webhook retries or operator actions.
- **Failure or abuse scenario:** A webhook deploy starts, an operator edits/redeploys, then the older run finishes last and marks the app running with stale routing/state.
- **Existing mitigations:** SQLite serializes individual DB statements; each SSH command is sequential inside one run.
- **Minimum safe fix:** Per-app single-flight lock that rejects or queues conflicting operations.
- **Recommended durable fix:** Operation records with generation IDs, per-app queue/lease, idempotent phases, cancellation, and stale-completion guards.
- **Alternatives and trade-offs:** In-memory locks are quick but do not survive restarts; DB leases add recovery complexity but give durable ownership.
- **Files likely involved:** `internal/app/service.go`, handlers, repository/schema, webhook.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** Blocking fake SSH tests that overlap deploy/redeploy/stop/delete and assert command order and final generation.
- **Acceptance criteria:** One app has at most one mutating operation; different apps can run in parallel; stale runs cannot update current status.

### `DCK-007` Changing an app's worker leaves the old deployment live

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/handler.go:588-705` writes the new `ServerID` and redeploys; `internal/app/service.go:127-239` loads only the current server. No path snapshots or cleans the previous worker's compose stack/Caddy route.
- **Preconditions:** Edit an existing app and select a different server.
- **Problem:** Placement change is treated as an ordinary redeploy rather than a migration.
- **Root cause:** The old server identity is discarded before lifecycle orchestration.
- **Current impact:** Old containers and routes remain active; duplicate application instances and stale public endpoints persist.
- **Failure or abuse scenario:** An operator moves a stateful app; DNS points to the new worker while the old worker continues processing jobs or serving traffic through its route.
- **Existing mitigations:** Updated routes store the new server ID; Cloudflare update may move the A record.
- **Minimum safe fix:** Detect a server change, retain old placement, and explicitly clean it after a successful new deployment.
- **Recommended durable fix:** A staged migration operation: deploy/health-check new, cut over DNS, remove old route/stack, record completion, and support rollback.
- **Alternatives and trade-offs:** Cleanup-first is simpler but creates downtime and weak rollback; deploy-first may briefly run two copies and needs stateful-app warnings.
- **Files likely involved:** app handler/service/repository, schema, Caddy/Cloudflare clients.
- **Dependencies:** DCK-006.
- **Estimated implementation complexity:** L
- **Validation plan:** Two-worker integration test covering success, cutover failure, old-worker cleanup failure, and rollback.
- **Acceptance criteria:** A completed move leaves exactly one intended stack/route; failures expose a recoverable migration state and never silently forget old placement.

### `DCK-008` Undeploy can report success while resources remain orphaned

- **Status:** Confirmed
- **Category:** Correctness / Reliability
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Individual App / Worker / Backup
- **Evidence:** `internal/app/service.go:292-343` ignores remote cleanup and repository errors, never removes Cloudflare records, never calls `DeleteDNSRecords`, and returns `nil`; `internal/db/schema.sql:58-69` gives DNS rows non-cascading FKs. When SSH/server lookup fails, local state is deleted without a durable remote-cleanup record.
- **Preconditions:** App has a saved DNS record, a remote cleanup command fails, or worker/server is unavailable.
- **Problem:** Destructive cleanup is best-effort but presented as completed.
- **Root cause:** No cleanup state machine; errors are discarded and local state is treated as disposable source data.
- **Current impact:** App deletion can fail at the DB FK, preserve remote DNS, orphan remote containers, or erase the only pointer needed for later cleanup.
- **Failure or abuse scenario:** A worker is offline during undeploy; Dockify deletes controller rows and reports success while the stack returns when the worker reconnects.
- **Existing mitigations:** Caddy routes and compose stack are attempted before local deletion when SSH works.
- **Minimum safe fix:** Propagate errors, explicitly delete stored DNS records, and retain a `cleanup_pending` tombstone when remote cleanup cannot run.
- **Recommended durable fix:** Idempotent cleanup phases with retries, per-resource outcomes, durable tombstones, operator force-delete, and reconciliation.
- **Alternatives and trade-offs:** Force-delete remains useful but must be explicit and preserve an audit record of skipped cleanup.
- **Files likely involved:** `internal/app/service.go`, repository/schema, Cloudflare client, UI.
- **Dependencies:** DCK-006, DCK-012.
- **Estimated implementation complexity:** L
- **Validation plan:** Failure-injection tests for SSH, Compose, Caddy, Cloudflare, and DB errors; retry/idempotency tests.
- **Acceptance criteria:** Success means all required resources are removed; partial cleanup is visible/retryable; DNS rows and remote records are handled explicitly.

### `DCK-009` Backup replace is destructive, partial, and route-lossy

- **Status:** Confirmed
- **Category:** Database / Backup
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Backup / All Apps / Worker
- **Evidence:** `internal/backup/backup.go:47-60,187-242` exports only the primary domain; import `292-423` ignores replace cleanup errors, mutates incrementally without a transaction/staging area, and creates no route rows. Extra domains/DNS topology are absent.
- **Preconditions:** Import, especially `mode=replace`; any mid-import failure or app with routes.
- **Problem:** A recovery feature can destroy the working configuration and return a partially restored controller that cannot recreate routing.
- **Root cause:** Backup schema models app rows, not the full deployed topology, and replace is delete-then-create.
- **Current impact:** Additional domains are lost, even primary domains lack route rows, and failures leave mixed old/new or incomplete state.
- **Failure or abuse scenario:** Replace removes apps, then a key write fails; remaining apps are not imported and already-removed routes cannot be recovered from the file.
- **Existing mitigations:** YAML and version are parsed first; encrypted values are validated before deletion; sensitive fields use authenticated encryption.
- **Minimum safe fix:** Add route data, validate all references/keys before mutation, surface a dry-run, and stop ignoring cleanup errors.
- **Recommended durable fix:** Versioned v2 export, staged import into a temporary DB/transactional repository layer, atomic controller swap where feasible, explicit post-restore deploy plan, and rollback backup.
- **Alternatives and trade-offs:** SQLite transactions cannot roll back remote undeploys; stage controller state first and separate remote reconciliation from atomic data replacement.
- **Files likely involved:** `internal/backup`, app/server repositories, schema, templates.
- **Dependencies:** DCK-012.
- **Estimated implementation complexity:** L
- **Validation plan:** Golden v1/v2 fixtures; lossless multi-route round trip; injected failure at every phase; replace rollback and dry-run tests.
- **Acceptance criteria:** Restore preserves all routes/settings needed for redeploy; preflight catches failures; partial remote work is reconciled; no silent skips.

### `DCK-010` Basic-auth routes use an invalid Caddy JSON handler shape

- **Status:** Requires Runtime Verification
- **Category:** Correctness
- **Severity:** High
- **Confidence:** Medium
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/caddy/client.go:24-29,55-76` emits `{"handler":"basic_auth","username":...,"hash":...}`. Standard Caddy JSON documents [`authentication` as the HTTP handler](https://caddyserver.com/docs/json/apps/http/servers/routes/handle/) with an [`http_basic` provider](https://caddyserver.com/docs/json/apps/http/servers/routes/handle/authentication/providers/); `basic_auth` is a Caddyfile directive, not the standard JSON handler name. No Caddy client tests exist.
- **Preconditions:** App route has Dockify basic authentication enabled and uses a standard Caddy image.
- **Problem:** Caddy is expected to reject the route JSON, so the advertised basic-auth deployment path cannot create its route.
- **Root cause:** Caddyfile directive syntax was translated directly into JSON without using Caddy's module schema or validating it.
- **Current impact:** Protected apps may be unreachable while Dockify continues to record deployment success under DCK-011.
- **Failure or abuse scenario:** Operator enables basic auth; containers start, Caddy returns a configuration error, UI shows running, and no protected route exists.
- **Existing mitigations:** Caddy HTTP status/body is parsed and logged; bcrypt hashing is used.
- **Minimum safe fix:** Emit the documented `authentication`/`http_basic` JSON structure and make route failure fail/degrade deployment.
- **Recommended durable fix:** Build typed Caddy module structures and validate representative routes against the exact supported Caddy image in CI.
- **Alternatives and trade-offs:** Generate a Caddyfile and adapt it through Caddy's config adapter; simpler authoring, but still requires versioned integration tests.
- **Files likely involved:** `internal/caddy/client.go`, app service, worker image/version, tests.
- **Dependencies:** DCK-011.
- **Estimated implementation complexity:** M
- **Validation plan:** Start the pinned Caddy image, POST plain/auth routes, inspect `/id/...`, and verify 401/challenge then successful credentials.
- **Acceptance criteria:** Caddy accepts persisted config; unauthenticated request receives 401; configured credentials proxy successfully; deployment reflects failures.

### `DCK-011` Deployment status can say success after required phases fail

- **Status:** Confirmed
- **Category:** Correctness
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/service.go:158-175` ignores secret/file repository errors and treats write failures as warnings; `227-238,346-436` records `running`/`success` after Caddy/DNS failures. Stop/start `448-505` changes container state without reconciling routes despite documented behavior.
- **Preconditions:** Secret/config write, Caddy, DNS, route persistence, or status update fails.
- **Problem:** One terminal status collapses container start, routing, DNS, and persistence into an optimistic success.
- **Root cause:** Deployment phases are not classified as required/optional and error propagation is inconsistent.
- **Current impact:** Operators cannot trust the dashboard; apps can run without secrets or remain unreachable while marked healthy.
- **Failure or abuse scenario:** `.env` write fails, Compose starts with old environment, Caddy update fails, and a success deployment is recorded.
- **Existing mitigations:** Compose pull/up failures mark failed; warning details are appended to deployment logs in some paths.
- **Minimum safe fix:** Fail on required file/compose/route errors and introduce a visible degraded result for optional configured integrations.
- **Recommended durable fix:** Phase model with operation ID, phase outcomes, health verification, reconciliation, and explicit stopped-route policy.
- **Alternatives and trade-offs:** DNS can remain optional when not configured; once configured for a route, failure should be degraded rather than silently successful.
- **Files likely involved:** app service/repository/model/templates, Caddy/Cloudflare clients.
- **Dependencies:** DCK-006.
- **Estimated implementation complexity:** M
- **Validation plan:** Phase failure matrix and UI tests for failed/degraded/running/stopped.
- **Acceptance criteria:** Status accurately distinguishes container, route, and DNS outcomes; required failures never produce success.

### `DCK-012` Schema migration and integrity controls are incomplete

- **Status:** Confirmed
- **Category:** Database / Architecture
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / Backup
- **Evidence:** `internal/db/db.go:39-51` runs unordered ad-hoc ALTER/UPDATE statements on every boot and ignores every error; `internal/db/schema.sql` omits cascades/indexes/uniqueness for core topology; duplicate names conflict with backup merge-by-name.
- **Preconditions:** Upgrade from an older/partially migrated DB, unexpected ALTER failure, duplicate names, or growing history.
- **Problem:** The controller cannot distinguish an expected duplicate-column error from a failed migration and cannot prove its schema version.
- **Root cause:** Boot-time schema patching replaced a migration ledger and domain constraints.
- **Current impact:** Mixed schemas can boot and fail later; ambiguous names undermine restore; cleanup relies on fragile manual ordering.
- **Failure or abuse scenario:** An ALTER fails for a real reason, the error is ignored, and a later deploy fails against the missing column after startup reported success.
- **Existing mitigations:** Base schema uses `IF NOT EXISTS`; foreign keys are enabled; app secrets/files have cascades and unique keys.
- **Minimum safe fix:** Add a schema version and check each known migration result, ignoring only explicitly understood idempotent cases.
- **Recommended durable fix:** Ordered transactional migrations with backups, compatibility checks, required indexes, and uniqueness/foreign-key policies aligned with lifecycle workflows.
- **Alternatives and trade-offs:** `PRAGMA user_version` is sufficient for this project; a third-party framework is optional.
- **Files likely involved:** `internal/db`, repositories, backup merge logic, tests.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** Upgrade fixtures for every released schema, interrupted migration test, duplicate-name behavior, FK cleanup tests.
- **Acceptance criteria:** Startup either reaches a known schema atomically or fails clearly; migrations run once and are covered by upgrade tests.

### `DCK-013` Remote operations and background workers lack lifecycle bounds

- **Status:** Confirmed
- **Category:** Reliability
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / Worker
- **Evidence:** `internal/ssh/client.go:49-65` has no command context/deadline; PTY/Shell goroutines wait/send without complete channel shutdown; `internal/server/service.go:199-202,272-294` has uncancelled overlapping monitor goroutines; `cmd/dockify/main.go:93-108` calls `Close` rather than graceful `Shutdown` and never stops/waits for background work.
- **Preconditions:** Hung remote command, slow/unreachable worker, repeated consoles, monitor cycle overlap, or SIGTERM during work.
- **Problem:** Work can outlive its request/process intent and accumulate without caps.
- **Root cause:** Root context, time budgets, wait groups, and bounded concurrency are absent.
- **Current impact:** Leaked goroutines/connections, stuck statuses, abrupt requests, and uncertain shutdown behavior.
- **Failure or abuse scenario:** `docker pull` hangs; each monitor/deploy generation adds blocked work, then SIGTERM drops requests while remote mutation continues or remains half-finished.
- **Existing mitigations:** SSH connect timeout; Cloudflare/update HTTP clients have timeouts; process exit eventually clears local goroutines.
- **Minimum safe fix:** Context-aware SSH exec, command deadlines, single-flight monitor per server, `Server.Shutdown`, monitor cancel, and bounded waits.
- **Recommended durable fix:** Root lifecycle manager with operation registry, time budgets by command class, cancellation, wait groups, and stuck-operation recovery on boot.
- **Alternatives and trade-offs:** Hard timeouts must account for large image pulls; make them configurable with safe maxima and progress visibility.
- **Files likely involved:** `cmd/dockify/main.go`, `internal/ssh`, server/app services, console.
- **Dependencies:** DCK-006.
- **Estimated implementation complexity:** L
- **Validation plan:** Fake hung SSH, clean PTY close, repeated monitor ticks, goroutine-count checks, SIGTERM integration test.
- **Acceptance criteria:** No operation is unbounded; shutdown drains or records incomplete work; monitor cannot overlap per server.

### `DCK-014` Webhook secret disable and database-error behavior do not fail closed

- **Status:** Confirmed
- **Category:** Security / Correctness
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / All Apps
- **Evidence:** `internal/settings/settings.go:129-165` implements disable by deleting the row, but `GetWebhookSecret` recreates a missing row; `cmd/dockify/main.go:79` discards DB errors and returns empty; `internal/webhook/handler.go:57-63,100-106` skips verification when empty.
- **Preconditions:** Operator disables the secret or settings DB read fails.
- **Problem:** "Disabled" immediately regenerates an unknown secret, while an actual settings error converts verification into unsigned acceptance.
- **Root cause:** Absence ambiguously means disabled, not initialized, and read failure; the callback cannot return an error/enabled state.
- **Current impact:** Disable does not behave as the UI describes; a DB fault can fail authentication open.
- **Failure or abuse scenario:** Settings query fails during a webhook request; empty secret causes an unsigned push payload to be accepted for matching repositories.
- **Existing mitigations:** Startup normally creates a strong 32-byte secret; GitHub uses `hmac.Equal`; deploy matching limits effects to registered repo/branch values.
- **Minimum safe fix:** Treat lookup error/empty as 503 and persist an explicit enabled state.
- **Recommended durable fix:** API returning `{secret, enabled, error}`, transactional secret rotation, constant-time GitLab comparison, tag/ref validation, and tests.
- **Alternatives and trade-offs:** Disabling verification entirely is unsafe; a clearer "disable webhook endpoints" state is preferable.
- **Files likely involved:** settings service/handler, main wiring, webhook handler, schema/templates.
- **Dependencies:** DCK-003.
- **Estimated implementation complexity:** S
- **Validation plan:** Missing, disabled, enabled, DB-error, rotate, GitHub/GitLab signature, and tag-ref tests.
- **Acceptance criteria:** Errors and disabled state never accept unsigned payloads; UI and endpoint behavior agree.

### `DCK-015` Compose and remote-file transformation is non-deterministic and weakly contained

- **Status:** Confirmed
- **Category:** Correctness / Architecture
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Individual App / Worker
- **Evidence:** `internal/app/compose.go:15-36` chooses the first service from Go map iteration; `77-113` treats any substring `dockify` as proof of network configuration; `internal/app/service.go:170-175` concatenates stored paths; `internal/ssh/client.go:67-73` uses a fixed heredoc delimiter and shell-quoted path without robust escaping.
- **Preconditions:** Multi-service compose, incidental `dockify` text, config path with traversal/control characters, or content containing the delimiter.
- **Problem:** Route targets and transformed files depend on nondeterministic/lexical behavior rather than parsed intent and containment.
- **Root cause:** Compose is partly parsed and partly handled as raw text/shell script.
- **Current impact:** Wrong service routing, missing network attachment, broken writes, or accidental writes outside the app directory by the trusted operator.
- **Failure or abuse scenario:** A two-service compose routes to a different service after restart; or a config content line `DOCKIFY_EOF` truncates the file and changes shell execution.
- **Existing mitigations:** YAML parsing exists; simple mode renames the first YAML node deterministically; the administrator is trusted and already has root console access.
- **Minimum safe fix:** Require/derive an explicit route service, structurally ensure network membership, validate relative paths, and randomize/avoid heredocs.
- **Recommended durable fix:** Typed compose normalization plus SFTP/streamed file writes rooted under a fixed remote directory.
- **Alternatives and trade-offs:** A full worker agent is unnecessary now; SFTP and strict validation remove most fragility at lower cost.
- **Files likely involved:** app compose/service/handlers, SSH client, templates/tests.
- **Dependencies:** DCK-006.
- **Estimated implementation complexity:** M
- **Validation plan:** Deterministic multi-service tests, YAML network fixtures, traversal/delimiter/binary file tests.
- **Acceptance criteria:** Route service is explicit/deterministic; every app service joins the required network; files cannot escape the app root and round-trip exactly.

### `DCK-016` Install, update, and release trust relies on mutable, unsigned artifacts

- **Status:** Confirmed
- **Category:** Security / DevOps
- **Severity:** High
- **Confidence:** High
- **Affected scope:** Controller / Worker / CI/CD
- **Evidence:** `internal/settings/settings.go:107-126` executes a mutable raw `main` update script via `systemd-run`; `scripts/install.sh`, worker initialization, and README use `curl | sh`; update/install download binaries without checksum/signature verification; `.github/workflows/build.yml` disables provenance and uses mutable action tags/images.
- **Preconditions:** Upstream repository/account/CDN/artifact or dependency delivery is compromised; operator invokes install/update/init.
- **Problem:** Privileged code and binaries are trusted by transport/location rather than immutable identity and verified digest/signature.
- **Root cause:** Release automation publishes artifacts but no integrity manifest/provenance contract consumed by installers.
- **Current impact:** A supply-chain compromise becomes controller or worker code execution with high privileges.
- **Failure or abuse scenario:** A mutable raw script or release binary is replaced; the settings update path downloads and executes it through systemd.
- **Existing mitigations:** HTTPS, GitHub access controls, update script rollback, non-root controller container.
- **Minimum safe fix:** Pin versioned URLs/commits and verify published SHA-256 checksums before execution/replacement.
- **Recommended durable fix:** Reproducible release workflow with checksums, SBOM, signed provenance/artifacts, SHA-pinned actions, digest-pinned images, and verification in every installer/update path.
- **Alternatives and trade-offs:** Checksums hosted beside artifacts protect transport mistakes but not a compromised release account; signatures/provenance add stronger separation at more operational cost.
- **Files likely involved:** settings update, scripts, Docker/Compose, GitHub workflows, docs.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** Tampered artifact/script tests, release dry-run, signature/checksum verification, rollback test.
- **Acceptance criteria:** No privileged path pipes mutable network content to a shell; every installed binary has verified identity; CI dependencies are immutable.

### `DCK-017` Browser code is trusted from CDNs without integrity or response policy

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser / Controller / Worker
- **Evidence:** `internal/http/templates/layout.html:6-9` loads HTMX/xterm resources from unpkg/jsDelivr without SRI; router sets no CSP, frame, referrer, or content-type security headers.
- **Preconditions:** CDN/package delivery compromise, malicious upstream publish, or ability to frame/inject through another browser channel.
- **Problem:** Third-party script execution has the same authority as Dockify's authenticated control-plane UI.
- **Root cause:** Frontend dependencies are runtime network dependencies rather than embedded single-binary assets.
- **Current impact:** Compromised JavaScript can read page data and invoke authenticated APIs/consoles.
- **Failure or abuse scenario:** A CDN serves modified xterm/HTMX code that opens a worker console under the operator's session.
- **Existing mitigations:** HTTPS CDNs, Go template autoescaping, `HttpOnly` cookie.
- **Minimum safe fix:** Add SRI/crossorigin and baseline security headers compatible with current UI.
- **Recommended durable fix:** Embed pinned frontend assets in the binary and enforce a nonce/hash-based CSP, `frame-ancestors`, `nosniff`, and referrer policy.
- **Alternatives and trade-offs:** SRI complicates upgrades but is small; self-hosting best matches the single-binary/offline product model.
- **Files likely involved:** templates, embed/static routing, HTTP middleware, dependency update docs.
- **Dependencies:** DCK-005.
- **Estimated implementation complexity:** M
- **Validation plan:** Header tests, offline UI load, CSP report/test, asset hash update process.
- **Acceptance criteria:** Core UI loads without unverified runtime scripts; restrictive policy blocks framing and unauthorized script sources.

### `DCK-018` Secret storage and file permissions exceed required exposure

- **Status:** Confirmed
- **Category:** Security
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / Worker / Backup
- **Evidence:** `internal/app/service.go:158-167` writes worker `.env` as `0644`; schema stores `app_secrets.value` and `apps.auth_pass` plaintext; backup can emit sensitive values plaintext when called without a passphrase; installer-created `.env` permissions are not explicitly restricted; server deletion does not remove key files.
- **Preconditions:** Another local worker/controller user, filesystem/backup access, stale key after server removal, or unencrypted programmatic export.
- **Problem:** Secrets remain readable longer and by more principals than required.
- **Root cause:** Configuration data and secret data share generic persistence/write paths without a lifecycle policy.
- **Current impact:** Local compromise or backup mishandling exposes app credentials and SSH material; deleted servers leave credential residue.
- **Failure or abuse scenario:** A non-root worker account reads `/opt/dockify/apps/app-N/.env`, or a removed server's key remains reusable from the controller disk.
- **Existing mitigations:** Controller data/key directories are created `0700`; saved key files are `0600`; UI export generates/requires a passphrase.
- **Minimum safe fix:** Write `.env`/installer env `0600`, remove keys on confirmed server deletion, and reject/clearly gate plaintext secret exports.
- **Recommended durable fix:** Secret inventory/lifecycle, envelope encryption with a separately managed instance key, rotation support, redacted UI/logging, and encrypted versioned backups.
- **Alternatives and trade-offs:** At-rest application encryption does not help if the controller process is compromised and introduces key recovery obligations; permissions/lifecycle are the immediate high-value fix.
- **Files likely involved:** app service, server delete, backup, schema/repositories, install scripts.
- **Dependencies:** DCK-004, DCK-012.
- **Estimated implementation complexity:** M (permissions/lifecycle), L (encryption)
- **Validation plan:** Permission assertions, deletion cleanup, plaintext-export test, encryption migration/restore tests if adopted.
- **Acceptance criteria:** Worker secret files are owner-only; removed credentials are deleted or intentionally retained with warning; backups cannot silently expose secrets.

### `DCK-019` Resource monitoring can publish invalid data and mis-schedule apps

- **Status:** Confirmed
- **Category:** Correctness / Reliability
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Controller / Worker
- **Evidence:** `internal/server/service.go:173-196` ignores all metric errors and marks the server online; `234-241` computes CPU from a single cumulative `/proc/stat` sample; scheduler uses CPU/RAM without freshness or disk capacity; dev mock substring-map iteration produced inconsistent RAM values during UI exercise.
- **Preconditions:** Metric command/parse failure, long-lived worker, stale measurements, equal scores, or constrained disk.
- **Problem:** "Online" and load scores can be based on zero, stale, or semantically wrong measurements.
- **Root cause:** Reachability, metric validity, freshness, and scheduling eligibility are conflated.
- **Current impact:** Dashboard metrics mislead and auto-select can choose the wrong or disk-full worker.
- **Failure or abuse scenario:** Commands fail but status becomes online with 0% load; scheduler sends a new app to that server.
- **Existing mitigations:** SSH connection failure marks offline; resource timestamps are stored; server list order is deterministic by created time for scheduler.
- **Minimum safe fix:** Propagate parse errors, keep last-good metrics with freshness, and exclude invalid/stale samples.
- **Recommended durable fix:** One versioned remote metrics probe, delta CPU sampling, explicit health/eligibility model, capacity-aware scoring, and deterministic mock responses.
- **Alternatives and trade-offs:** More sophisticated weights should follow measured needs; correctness/freshness comes before tuning.
- **Files likely involved:** server service/repository/model, scheduler, SSH mock, UI/tests.
- **Dependencies:** DCK-013.
- **Estimated implementation complexity:** M
- **Validation plan:** Parser fixtures, failing command matrix, stale timestamp tests, scheduling invariants, deterministic mock test.
- **Acceptance criteria:** Invalid samples never overwrite last-good data or qualify a worker; CPU reflects an interval; scheduling is deterministic and capacity-safe.

### `DCK-020` The documented base-path deployment mode is incomplete

- **Status:** Confirmed
- **Category:** Correctness / UI/UX
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser / Controller
- **Evidence:** `SPEC.md`/`README.md` document `DOCKIFY_BASE_PATH`; `internal/http/router.go:227-238` only injects a template `<base>` and trusts client `X-Forwarded-Prefix`; many handlers/templates use absolute `/login`, `/apps/...`, `/export`, `/import`, and `/api/backup/...` URLs.
- **Preconditions:** Serve Dockify below a reverse-proxy prefix.
- **Problem:** Absolute actions/redirects escape the prefix, while the header does not mount/strip router paths.
- **Root cause:** Base path is presentation context, not a first-class router/URL-generation setting.
- **Current impact:** Login, backup, edit, and redirects break behind the documented subpath; untrusted headers can alter page base URLs.
- **Failure or abuse scenario:** Dockify is published at `/dockify`; an export form posts to the proxy root `/api/backup/export` and returns 404 or hits another service.
- **Existing mitigations:** Relative links work through `<base>`; a correctly configured proxy may rewrite selected paths.
- **Minimum safe fix:** Central URL helper for every link/action/redirect and ignore forwarded prefix unless the proxy is trusted.
- **Recommended durable fix:** Configure one normalized base path, mount the router under it, generate URLs centrally, and define trusted forwarded-header handling.
- **Alternatives and trade-offs:** Removing the feature is acceptable if clearly documented; partial support is worse than an explicit unsupported state.
- **Files likely involved:** router/auth/handlers, template helpers/templates, config/docs/tests.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** End-to-end tests at `/` and `/dockify`, including auth, forms, HTMX, static assets, and WebSockets.
- **Acceptance criteria:** Every flow remains under the configured prefix; direct spoofed prefix headers do not change generated control-plane URLs.

### `DCK-021` Mobile overflow and accessibility gaps obstruct core workflows

- **Status:** Confirmed
- **Category:** UI/UX / Accessibility
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** Browser
- **Evidence:** At a 390×844 viewport, add-server measured `document.scrollWidth=502` and server list `651`; screenshots confirm clipped navigation/tables. `layout.html` has one 600px grid breakpoint, no semantic `<nav>`/`<main>`, glyph-only theme labeling, removed default outline, no reduced-motion rule, and status changes lack live regions.
- **Preconditions:** Phone/narrow window, keyboard navigation, screen reader, or reduced-motion preference.
- **Problem:** Core server/app management requires horizontal page scrolling and lacks consistent semantic/focus/state cues.
- **Root cause:** Responsive work addresses grids but not navigation/tables/forms, and components lack an accessibility contract.
- **Current impact:** Controls move off-screen; tables are hard to inspect; assistive-technology users receive incomplete navigation/status feedback.
- **Failure or abuse scenario:** A phone user cannot reach a table action without scrolling the entire page sideways; a deployment status change is not announced.
- **Existing mitigations:** Strong text labels in many forms, word-based status badges, good color tokens, visible empty states, some local overflow on code/log areas.
- **Minimum safe fix:** Wrapping/mobile nav, table scroll wrappers or mobile cards, preserved focus-visible styles, semantic landmarks, labeled icon buttons, reduced-motion guard.
- **Recommended durable fix:** Accessibility component checklist and automated/manual viewport, keyboard, screen-reader, and zoom regression suite.
- **Alternatives and trade-offs:** Scroll wrappers are the quick win; card transformations improve comprehension but require more template maintenance.
- **Files likely involved:** `internal/http/templates/layout.html` and list/detail/form templates, template tests.
- **Dependencies:** None.
- **Estimated implementation complexity:** M
- **Validation plan:** 320/390/600/1024 px, 200% zoom, keyboard-only, accessible-name/landmark checks, reduced-motion check.
- **Acceptance criteria:** No page-level horizontal overflow at 320/390 px; all core actions are keyboard reachable and named; status/errors are announced appropriately.

### `DCK-022` CI and tests do not gate the highest-risk behavior

- **Status:** Confirmed
- **Category:** Testing / DevOps
- **Severity:** Medium
- **Confidence:** High
- **Affected scope:** CI/CD / All Apps
- **Evidence:** `.github/workflows/build.yml` runs only for version tags; packages for backup, Caddy, Cloudflare, server, SSH, settings, and webhook have no tests; deploy/undeploy/move/import/auth/console paths lack integration tests; local `gofmt -l` lists 13 files.
- **Preconditions:** PR/main change or release defect in an uncovered path.
- **Problem:** The first automated repository gate is the release tag, and it does not exercise the lifecycle defects that dominate this audit.
- **Root cause:** Tests grew around pure helpers/repositories rather than risk boundaries; workflow is release-oriented.
- **Current impact:** Broken main and production regressions can merge undetected; passing race tests provide false comfort because concurrent paths are not invoked.
- **Failure or abuse scenario:** A backup migration or deploy concurrency regression merges, then tag CI passes existing unit tests and publishes it.
- **Existing mitigations:** Current unit/template tests are fast and useful; local build/vet/test/race all pass; smoke-test script exists.
- **Minimum safe fix:** PR/push workflow with format, vet, test, race, and smoke gates; add regression tests for P0/P1 fixes before implementation.
- **Recommended durable fix:** Risk-based test pyramid with fake SSH fault injection, real Caddy/SQLite integration, HTTP/WS tests, migration fixtures, scanner tooling, and release gate consuming prior CI results.
- **Alternatives and trade-offs:** A coverage percentage is secondary; require coverage of invariants and failure modes rather than chasing a global number.
- **Files likely involved:** workflows, tests across core packages, formatter/linter config.
- **Dependencies:** None; tests should precede each fix.
- **Estimated implementation complexity:** M
- **Validation plan:** Intentionally break each gate; run concurrency/failure matrices; verify tag publication depends on green commit CI.
- **Acceptance criteria:** Every PR runs required gates; P0/P1 paths have regression tests; release cannot publish an unvalidated commit.

### `DCK-023` Documentation, formatting, and dev-mock drift reduce auditability

- **Status:** Confirmed
- **Category:** DX / Documentation
- **Severity:** Low
- **Confidence:** High
- **Affected scope:** Other
- **Evidence:** `gofmt -l` lists 13 Go files; `SPEC.md` project structure names files that do not exist; settings copy says SSH keys are not exported while backup code/export UI includes them encrypted; stop/start route behavior differs from implementation; `internal/ssh/mock.go` matches commands through map iteration and produced inconsistent UI metrics; `export.html:69-78` relies on implicit global `event`.
- **Preconditions:** Contributor follows docs, operator relies on backup/lifecycle wording, dev mock command matches multiple keys, or browser lacks implicit `event`.
- **Problem:** Several small sources of truth disagree and deterministic development behavior is not enforced.
- **Root cause:** Documentation and mocks are not covered by consistency tests/checklists.
- **Current impact:** Contributor confusion, unreliable UI demonstrations/tests, and minor browser breakage.
- **Failure or abuse scenario:** An operator assumes keys are excluded from a backup; or the mock dashboard alternates resource values for the same command.
- **Existing mitigations:** Documentation is generally substantive; template tests catch parse errors; AGENTS.md lists standard commands.
- **Minimum safe fix:** Run gofmt, make mock matching ordered, pass the click event explicitly, and correct lifecycle/backup docs.
- **Recommended durable fix:** Docs-to-config checklist in PRs, deterministic fixtures, format CI, and small browser behavior tests.
- **Alternatives and trade-offs:** Keep SPEC aspirational only if clearly labeled; otherwise describe actual files/behavior.
- **Files likely involved:** Go sources, `SPEC.md`, README/settings/export templates, SSH mock, CI.
- **Dependencies:** None.
- **Estimated implementation complexity:** S
- **Validation plan:** Format gate, deterministic repeated mock calls, doc review, export copy browser test.
- **Acceptance criteria:** `gofmt -l` is empty; documented backup/lifecycle behavior matches code; mock output is stable; copy button works without a global event.

## 12. Security analysis

The controller is a privileged management plane, so secure defaults and boundary authentication matter more than multi-tenant authorization. DCK-001 and DCK-002 are the clearest security blockers: an omitted password can open the entire product, and every worker identity is accepted. DCK-003 makes the public webhook path an availability boundary before authentication. DCK-005, DCK-014, DCK-016, DCK-017, and DCK-018 form the next defense-in-depth layer around browser requests, webhook state, supply-chain identity, client-side code, and secret lifetime.

The prior audit labeled administrator-supplied compose, domains, paths, and file contents as Critical RCE. That is not the correct trust model for the current product: the same authenticated administrator already has interactive root/server and container consoles. These inputs are still dangerously brittle because mistakes can escape an app directory or corrupt shell scripts; they are retained as Medium correctness/containment work in DCK-015. If Dockify later adds untrusted tenants, project-scoped roles, or externally editable deployment specs, this boundary must be reclassified and the same primitives become high-severity privilege escalation paths.

No evidence supports a Critical rating under the prompt's definition. The High findings require realistic configuration/network/supply-chain preconditions and have broad impact, but none is a demonstrated unauthenticated, default, low-complexity compromise in every deployment state.

## 13. Correctness and lifecycle analysis

The app lifecycle is not an atomic transaction; that is expected across SSH, Docker, Caddy, DNS, and SQLite. What is missing is an explicit saga/reconciliation model. DCK-006 is the prerequisite: without operation serialization and identity, attempts to fix move, undeploy, and status truthfulness remain vulnerable to stale concurrent completion.

After serialization, implement placement migration (DCK-007), idempotent cleanup (DCK-008), and phase-aware status (DCK-011). DCK-010 should be validated early against the pinned Caddy version because it appears to make the basic-auth feature nonfunctional. DCK-015 should then make compose routing and remote files deterministic. Stop/start semantics must be decided explicitly: either preserve routes and surface a stopped/upstream-unavailable state, or remove/recreate routes as current docs claim.

Rollback currently restores a compose snapshot but shares the same unsynchronized deployment machinery and does not represent route/DNS/secret state as a consistent release. A durable operation model should define exactly which deployment artifacts a rollback restores.

## 14. Reliability and operational analysis

The process has liveness but not readiness: `/health` always returns `ok` without checking SQLite or background health. There is no graceful drain, no durable operation ownership, no command deadline, and no reconciliation loop for incomplete work. DCK-013 addresses the immediate control-plane lifecycle. DCK-008 and DCK-011 address truthful state after partial failures.

Operational visibility is limited to `log.Printf` and deployment text logs. This is workable for a small MVP, but operation IDs, phase outcomes, structured fields, and cleanup/retry status should be introduced with the lifecycle changes rather than as a separate logging rewrite. A lightweight `/ready` DB check and metrics for active/stuck operations, worker refresh age, and cleanup backlog would materially improve day-two operation.

The global `docker image prune -af` and `docker builder prune -af` during undeploy are cross-app performance side effects. They do not delete images used by running containers, so the previous High security framing was exaggerated. Replace them with app-scoped cleanup or an explicit maintenance action while implementing DCK-008.

## 15. Database and persistence analysis

SQLite is appropriate for the intended controller size, and `MaxOpenConns(1)` plus WAL/foreign keys is a good safety baseline. The main persistence risk is semantic, not engine choice: multi-step workflows ignore errors, topology FKs do not encode cleanup policy, and migrations have no ledger.

DCK-012 should precede schema-dependent recovery work. High-risk migration steps are adding constraints/cascades to existing data and introducing uniqueness where duplicates may already exist. Ship a preflight report, backup the DB, identify duplicates/orphans, migrate through new tables when SQLite ALTER limitations require it, run `foreign_key_check`, and retain a documented rollback copy.

DCK-009 needs a versioned backup contract. A whole-document MAC, proposed by the prior audit, is not inherently required because exported YAML is explicitly designed to be edited; sensitive encrypted fields already have AEAD integrity. What is required is schema completeness, validation, clear plaintext/encrypted handling, staged mutation, and explicit reconciliation of remote side effects.

## 16. Performance analysis

No measured repository evidence shows a current throughput bottleneck at the documented small scale. The main performance risks are derived from unbounded behavior: one SSH connection with several sequential commands per monitored worker, possible monitor overlap, unbounded webhook deploy goroutines, repeated PBKDF2 derivation per encrypted field, and unbounded deployment/log history growth.

Fix correctness and bounding first (DCK-003, DCK-006, DCK-013, DCK-019). Then measure worker count, command latency, DB query latency, deployment-history size, and backup field count before adding connection pools or database tuning. Adding indexes with DCK-012 is low risk once real query plans justify them. A worker agent or horizontal controller is not warranted by current evidence.

## 17. Code quality and architecture analysis

The package boundaries are understandable and the connector/repository abstractions are valuable. The main quality issue is responsibility concentration: `deployWithCommit`, `Undeploy`, server initialization, and backup import mix orchestration, persistence, remote execution, error policy, and user-visible status.

Refactor only in support of the roadmap:

- Extract an operation coordinator and typed phases before changing deploy semantics.
- Introduce typed Caddy configuration and a rooted file-transfer abstraction while fixing DCK-010/DCK-015.
- Add transactional repository methods/migration runner for DCK-009/DCK-012.
- Replace router positional dependencies with a struct only when browser security/base-path middleware is reworked.

Do not begin with a worker-agent rewrite, generic DI framework, or package reshuffle. Those changes create review noise without first protecting invariants. The immediate format failure should be corrected and gated as DCK-023/DCK-022.

## 18. Testing analysis

Existing tests are fast and establish useful patterns, especially compose helper tables, in-memory SQLite repositories, scheduler behavior, and template render checks. Coverage is misaligned with risk: the packages that mutate workers, authenticate webhooks, generate Caddy JSON, restore backups, and bridge WebSockets have no focused tests.

Required test layers:

1. Deterministic fake SSH with blocking/failure injection and an ordered command log.
2. SQLite migration fixtures from every released schema and FK/orphan assertions.
3. HTTP/WS tests for auth defaults, body limits, CSRF/origin, base paths, and webhook states.
4. Real pinned Caddy container/API integration for route JSON and persistence.
5. Two-worker lifecycle tests for move, undeploy retry, stale deploy completion, and restart recovery.
6. Backup golden fixtures and failure at every mutation boundary.
7. Browser viewport/keyboard/accessibility smoke coverage for core flows.

`go test -race ./...` passing remains mandatory but is not sufficient until tests execute the goroutine/channel paths. `govulncheck`, `staticcheck`, and `gosec` should be added to a reproducible CI toolchain; their absence means dependency and static-analysis status is unknown, not clean.

## 19. UI/UX and accessibility analysis

The visual system is restrained and coherent. Empty states, status badges, dark/light tokens, and the deploy form make the product understandable without a framework. The desktop primary flow completed successfully in mock mode and the app-detail page produced no observed browser console error.

The main verified issue is narrow-screen layout. At 390 px, navigation and tables create page-level horizontal overflow. Mobile screenshots show clipped server-table content and an over-wide header. Fix this before adding visual polish.

Accessibility work should prioritize function: semantic navigation/main landmarks, `:focus-visible`, explicit accessible names/state on theme/console controls, proper labels for dynamic file/domain controls, live error/status regions, table headers/captions, and reduced-motion. Existing whole-page meta refresh during deploy/initialize should become targeted status polling so it does not reset focus, scroll, or an open console.

Additional current client defects worth including in DCK-021/DCK-023 regression work: duplicate `domains-list` IDs, `var(--muted)` token typos, `TextEncoder` scoped inside `onopen` but referenced by resize handlers, error template omitting shared scripts, confirm strings containing unescaped names, and the export copy helper's implicit `event` dependency.

## 20. DevOps and developer experience analysis

The tag workflow builds, vets, tests, smoke-tests, publishes a release, and pushes a container, which is a solid release skeleton. It is too late in the lifecycle: PRs and `main` have no automated quality gate. Add commit CI first, then make release jobs consume an already validated commit.

The documented Docker install path requires an immediate correction under DCK-004. Release hardening under DCK-016 should pin action SHAs and image digests, publish checksums/SBOM/provenance, verify artifacts in scripts, and remove mutable network-to-shell execution. Use a pinned version variable throughout instead of raw `main`/`latest`.

Developer experience quick wins are format enforcement, deterministic mock behavior, accurate docs, a complete `.env.example`, and consistent local/CI commands. Avoid adding broad lint policy until the existing tree is formatted and the team agrees on rules.

## 21. Previous audit reconciliation

`AUDIT.md` was inspected only after the independent pass. References below group duplicated statements and roadmap entries rather than reproducing all 901 lines.

| Previous ref | Previous theme | Verification status | Severity/remediation reconciliation | Current disposition |
|---|---|---|---|---|
| PA-01 | SSH shell injection, file traversal, Caddy interpolation (Critical/C1/C5/H3) | Partially confirmed | Brittle shell/file construction is real; external-attacker Critical framing does not match the trusted single-admin boundary. SFTP/validation remains proportionate; worker agent is optional. | Modified/merged into DCK-015; host identity separate in DCK-002 |
| PA-02 | `InsecureIgnoreHostKey` (Critical/C2) | Confirmed | High, not Critical: requires network/address impersonation but has all-worker blast radius. TOFU/fingerprint fix remains appropriate. | Retained as DCK-002 |
| PA-03 | CSRF, cookie flags, permissive WS origin (Critical/C4/C7) | Confirmed with qualification | Browser default Lax mitigates ordinary cross-site flows; same-site/legacy/proxy cases and root console justify Medium. Strict origin + explicit cookie/CSRF is appropriate; DB sessions are not required. | Modified/merged into DCK-005 |
| PA-04 | Empty/disabled webhook secret accepts unsigned deploys (Critical/C3) | Modified | Normal startup generates a secret; disable deletes it and the next lookup regenerates it, so "disabled means unsigned" is false. DB lookup errors do produce empty/fail-open behavior. | Replaced by DCK-014 |
| PA-05 | Self-update and `curl|sh` supply chain (Critical/C6) | Confirmed | High rather than Critical because compromise of upstream/delivery or explicit operator invocation is required. Checksums/pinning are minimum; signatures/provenance durable. | Retained/expanded as DCK-016 |
| PA-06 | Compose mounts host `~/.ssh` (Critical #9) | Confirmed, impact modified | Key-exposure concern is valid, but the stronger direct defect is that the non-root container cannot write its configured read-only key directory. | Modified into DCK-004 and DCK-018 |
| PA-07 | No deploy mutex, unbounded goroutines, abrupt shutdown (C8/H6) | Confirmed | Previous Critical rating combined distinct likelihoods. Deploy interleaving is High; lifecycle bounding is Medium. | Split into DCK-006 and DCK-013 |
| PA-08 | Missing cascades/migrations/delete cleanup (H2/H8) | Confirmed | Valid, but `ON DELETE CASCADE` alone must not erase remote cleanup state. Durable tombstones/reconciliation are safer. | Split into DCK-008 and DCK-012 |
| PA-09 | Plaintext secrets and `.env` 0644 (C5/H10) | Confirmed | Worker mode is Medium. DB encryption is durable hardening, not the minimum blocker; permissions and lifecycle first. | Retained as DCK-018 |
| PA-10 | CPU metric/scheduler inputs (H1/A6-A8) | Confirmed | High was inflated for an optional scheduler in a small deployment; Medium correctness. Error-ignoring and freshness are as important as weights. | Retained/expanded as DCK-019 |
| PA-11 | Tag-only CI, unpinned supply chain (H7) | Confirmed | Valid. Separate test-gating from artifact trust so each has clear acceptance criteria. | Split into DCK-016 and DCK-022 |
| PA-12 | Backup partial import, lost routes, whole-document MAC (H5/H9/A9-A15) | Partially confirmed | Destructive/lossy restore is High. Whole-document MAC is unnecessary for intentionally editable YAML; field AEAD already authenticates encrypted secrets. A DB transaction alone cannot roll back remote undeploys. | Modified into DCK-009 |
| PA-13 | UI bugs/responsiveness/a11y (M3/M8) | Confirmed in part | Mobile overflow and several source defects verified. Subjective "premium/world-class" comparisons and unmeasured contrast claims are not findings. | Merged into DCK-021/DCK-023 |
| PA-14 | Caddy/security headers/CDN | Partially confirmed | Missing control-plane browser policy is Medium. Worker-app HSTS/CSP cannot be imposed generically without product policy. | Retained narrowly as DCK-017 |
| PA-15 | Handler size, router args, structured logging, worker agent | Confirmed as observations | These are maintainability/strategic options, not independently harmful defects at current size. Refactor only as prerequisite to lifecycle work. | Roadmap/deferred, no standalone finding |
| PA-16 | Backup KDF per field/performance | Requires measurement | Derived cost is plausible, but no benchmark or representative backup size was provided. | Deferred performance measurement |
| PA-17 | Global Docker prune on undeploy | Confirmed | Cross-app cache/performance side effect, not High security. | Folded into DCK-008 operational cleanup |
| PA-18 | UI private key rendered in server detail | Rejected | Handler passes the stored key path, not private-key contents; template does not read/display the key file. | Rejected |
| PA-19 | Scheduler pointer-to-slice concern | Rejected by prior audit itself | `&servers[i]` is valid and stable for the existing slice. | Rejected |
| PA-20 | Version comparison/update cache | Partially confirmed | Cache exists; version comparison is semantically naive. Low operational impact and can be handled with release tooling cleanup. | Folded into P2 DevOps work |
| PA-21 | Missing request body limits | Confirmed | Public webhook path is stronger than prior generic Medium treatment. | Elevated/retained as DCK-003 |
| PA-22 | Base-path header concerns | Confirmed and expanded | Independent pass found absolute redirects/forms make the documented feature broadly nonfunctional. | Retained/expanded as DCK-020 |
| PA-23 | Basic-auth route behavior | Previously missed | Independent pass found Caddy JSON schema mismatch; requires real Caddy validation. | New DCK-010 |
| PA-24 | Moving app between workers | Previously missed | Independent path trace confirmed old worker is never cleaned. | New DCK-007 |
| PA-25 | Default Compose key persistence | Previously underemphasized | Prior report focused on key theft, not the direct read-only onboarding failure. | New/expanded DCK-004 |

### Reconciliation status summary

| Status | Count | Meaning |
|---|---:|---|
| Retained substantially | 7 | Current evidence and proportionate remediation agree |
| Modified / merged / split | 11 | Core issue valid, but scope, severity, trust model, or solution changed |
| Rejected / unsupported | 2 | Not a defect under current code/evidence |
| Deferred pending measurement/runtime | 2 | Plausible but not sufficiently verified for roadmap priority |
| Important new/previously missed | 3 | Basic-auth JSON, app worker migration, default Compose key persistence |

Counts overlap where a previous group maps to more than one disposition; the detailed table is authoritative.

## 22. Rejected, stale, or unsupported previous findings

- **Critical shell injection from administrator fields:** rejected at that severity. The administrator already has root-capable consoles. Retain containment and exact file transfer as DCK-015; reclassify if untrusted tenants are introduced.
- **Webhook disable always enables unsigned deploys:** false in normal operation. Missing settings are regenerated; the actual defect is broken disable semantics plus DB-error fail-open (DCK-014).
- **Private SSH key displayed on server detail:** unsupported. The model stores a filesystem path and the template renders server metadata, not file contents.
- **Whole-document backup MAC as a required fix:** unnecessary because the format explicitly supports editing. Authenticate/encrypt sensitive material and validate the complete schema instead.
- **In-memory sessions must move to SQLite:** overly complex for the current single-process architecture. Explicit cookie/origin/CSRF controls and expiry cleanup are sufficient unless multi-instance support becomes a product goal.
- **Worker agent as immediate remediation:** strategic, not prerequisite. SFTP, typed Caddy JSON, command contexts, validation, and operation coordination address current verified failures at lower cost.
- **Global Docker prune as High security:** exaggerated. It disrupts caches and shared-worker performance but does not remove images used by running containers; fold into cleanup correctness.
- **PBKDF2 per field as High:** unsupported without representative benchmarks. Measure and, if needed, evolve the backup envelope format compatibly.
- **Horizontal scaling/connection pooling:** no current scale or latency measurements justify them.
- **Stale line numbers:** many prior references no longer line up exactly after intervening edits; current findings cite present symbols/ranges and should be used instead.

## 23. Dependency-aware prioritized roadmap

### Wave 0 — Safety containment and regression harness (P0)

1. DCK-001: fail closed on unauthenticated public bind.
2. DCK-002: worker fingerprint enrollment and verification.
3. DCK-003: body/header/time limits that preserve WebSockets.
4. DCK-004: writable dedicated key storage and create compensation.
5. Start DCK-022: PR CI plus regression harness for all Wave 0 behavior.

These items can run in parallel except shared edits to main/router/CI should be coordinated. Gate: documented native and Compose startup matrix passes; oversized/slow requests fail safely; a mismatched worker key receives no command; worker key survives container restart.

### Wave 1 — Deterministic lifecycle and browser boundary (P1)

1. DCK-006 operation coordinator and generation model.
2. In parallel after coordinator interfaces stabilize: DCK-005 browser controls, DCK-014 webhook state, DCK-017 embedded/pinned browser assets.
3. On DCK-006: DCK-011 phase/status truthfulness and DCK-015 deterministic compose/file handling.
4. Validate and fix DCK-010 against pinned Caddy.
5. On the operation/phase model: DCK-007 staged worker migration and DCK-008 retryable undeploy.

Gate: concurrency/failure-injection suite passes; Caddy plain/basic-auth routes validate; cross-origin requests fail; move/undeploy leave no unexplained resources.

### Wave 2 — Persistence, recovery, and operational lifecycle (P1)

1. DCK-012 migration ledger, preflight, constraints, and upgrade fixtures.
2. DCK-013 command/background/shutdown lifecycle, building on operation coordinator.
3. DCK-009 backup v2 and staged restore, building on migrations/reconciliation.
4. DCK-018 permissions/credential lifecycle; schedule optional at-rest encryption after recovery key design.
5. DCK-016 immutable checksums/pins first, then signed provenance/SBOM.

Gate: upgrade/rollback fixtures pass; SIGTERM/hung SSH tests leave recoverable state; restore is lossless under injected failures; tampered artifacts are rejected.

### Wave 3 — Product and engineering quality (P2)

1. DCK-019 correct/fresh metrics and scheduling eligibility.
2. DCK-020 base-path contract or explicit feature removal.
3. DCK-021 mobile/accessibility pass.
4. Complete DCK-022 scanner/integration/browser gates.
5. DCK-023 deterministic mocks, formatting, and documentation consistency.

Gate: viewport/keyboard checks pass; root and subpath E2E pass; scheduling fixtures are deterministic; `gofmt`, vet, test, race, scanners, and smoke are green.

### Quick wins that must not displace P0/P1

- Remove host SSH mount and point Compose at `/var/lib/dockify/keys` (part of DCK-004).
- Add body caps/server timeouts (DCK-003).
- Make webhook lookup errors return 503 and make disable explicit (DCK-014).
- Correct Caddy JSON and add one integration test (DCK-010).
- Set `.env` and installer secret files to `0600` (DCK-018).
- Run gofmt, order mock matches, pass copy event explicitly (DCK-023).
- Add mobile table wrapper/nav wrapping and `:focus-visible` (DCK-021).

### Roadmap dependency table

| Work item | Depends on | Can run with | Migration/rollback concern |
|---|---|---|---|
| DCK-001 | — | DCK-002–004 | Preserve explicit local/dev override |
| DCK-002 | — | DCK-001,003,004 | Existing servers need enrollment/transition mode |
| DCK-003 | — | DCK-001,002,004 | Ensure WS/large legitimate backup compatibility |
| DCK-004 | — | Other Wave 0 | Move/copy existing keys with ownership check |
| DCK-006 | Regression harness | DCK-005,014,017 | Recover in-flight legacy `deploying` rows |
| DCK-011/015 | DCK-006 | Each other | Preserve legacy status display/compose behavior where safe |
| DCK-007/008 | DCK-006,011 | Each other after interfaces settle | Tombstones and explicit force-cleanup path |
| DCK-010 | DCK-011 status policy | DCK-005/014 | Pin Caddy version; preserve old config backup |
| DCK-012 | — | DCK-013/016 | Preflight duplicates/orphans; backup DB; migration rollback copy |
| DCK-009 | DCK-012, DCK-008 semantics | DCK-018 | Maintain v1 import compatibility; never auto-delete rollback backup |
| DCK-013 | DCK-006 | DCK-012/016 | Configurable timeouts; recover interrupted operations |
| DCK-018 | DCK-004,012 | DCK-009/016 | Master-key loss/rotation if encryption added |
| DCK-016 | — | Most work | Keep previous signed/checksummed version available |
| DCK-019–023 | Core P1 stable | Mostly parallel | Base-path compatibility and UI snapshot updates |

## 24. Validation strategy

Every wave should satisfy four gates:

1. **Static/build:** clean gofmt, `go vet`, `go test`, `go test -race`, `go mod verify`, `govulncheck`, agreed linter/static checks, and reproducible build.
2. **Invariant tests:** operation serialization, stale-completion rejection, authenticated SSH identity, bounded ingress/commands, truthful phases, lossless restore, and retryable cleanup.
3. **Integration:** pinned SQLite upgrade fixtures, real Caddy API, fake/real SSH worker, Cloudflare HTTP stub, HTTP/WS browser boundary, and Compose installation persistence.
4. **Operational acceptance:** install/upgrade/rollback dry run, SIGTERM during deploy, offline worker undeploy/move, backup replace failure, responsive/accessibility flow, and tampered artifact rejection.

Release candidates should run a real two-worker soak with repeated webhook retries, overlapping operator actions, worker disconnect/reconnect, Caddy restart, controller restart, and DNS API faults. Capture command/phase logs and verify the final DB/worker/Caddy/DNS inventory converges.

## 25. Suggested implementation sequencing

Use small vertical changes rather than a broad rewrite:

1. Land regression tests and safe-default changes with no schema changes.
2. Add operation IDs/coordination while preserving current deploy phases.
3. Make phase errors/status explicit, then implement move and cleanup on that substrate.
4. Introduce the migration ledger and cleanup tombstones before backup v2.
5. Add graceful lifecycle/timeouts after operations are cancellable and observable.
6. Harden artifact trust and secret lifecycle with compatibility/rollback paths.
7. Finish base-path/UI/a11y/metrics and expand quality gates.

High-risk changes should be feature-flagged or migration-versioned where practical. Retain the previous binary and DB backup for every migration; never automatically roll back remote state by merely restoring an old DB without reconciliation.

## 26. Deferred strategic opportunities

- Worker agent or structured remote RPC instead of SSH shell orchestration, only if feature growth or untrusted tenancy justifies it.
- SSH connection pooling after measurements show connection setup dominates latency.
- Blue/green or canary deployment after deterministic single-app lifecycle is proven.
- Persistent audit log and metrics dashboard once operation records exist.
- Multi-user roles, project isolation, and tenant authorization; these would materially change the trust model and reclassify DCK-015 inputs.
- External secret manager/KMS integration after a clear backup/disaster-recovery key story.
- Horizontal controller/multi-instance sessions only if product scale requires it.
- Backup envelope optimization after benchmarks; keep v1/v2 compatibility.
- SSE/live deployment logs and finer HTMX status updates after lifecycle statuses are trustworthy.

## 27. Open questions and runtime-verification needs

1. Validate DCK-010 against the exact Caddy image/version used in production and inspect existing saved configs.
2. Confirm intended stop/start route behavior: retain route and show stopped, or remove/re-add route/DNS.
3. Define whether Cloudflare DNS deletion is expected on undeploy and what force-delete means during API outage.
4. Decide the supported reverse-proxy/base-path contract and trusted forwarded-header sources.
5. Inventory existing installations for duplicate server/app names, orphaned DNS rows, and partially migrated schemas before adding constraints.
6. Measure realistic worker count, SSH command latency, deployment history, and backup secret/file counts before performance redesign.
7. Decide whether unencrypted programmatic backup export remains supported; the current UI always generates a passphrase.
8. Define supported Caddy/Docker/Compose versions and whether floating `latest` deployments exist in the field.
9. Run dependency vulnerability scanning; current audit could not execute `govulncheck` or equivalent.
10. Verify install-script `.env` modes under supported host umasks and existing deployments' key ownership.

## 28. Final readiness verdict

**Not suitable for production exposure.**

The verdict is driven by four verified P0 blockers—optional authentication on a public bind, disabled SSH host verification, unbounded public ingress/HTTP lifetimes, and a broken default Compose key-storage path—plus P1 lifecycle defects that can leave duplicate deployments, orphaned resources, partial restores, false success states, or nonfunctional basic-auth routes. Passing build, vet, tests, race tests, module verification, and template checks is encouraging, but the riskiest workflows have little or no test coverage and were not validated against real workers/Caddy/Cloudflare.

Dockify can be used for local development and controlled evaluation with non-valuable workloads. Production exposure should wait until Wave 0 and Wave 1 are complete, migration/recovery work is underway, and the validation gates pass on real two-worker infrastructure.
