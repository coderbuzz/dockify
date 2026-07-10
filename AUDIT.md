# Dockify — World-Class Engineering Audit

> Comprehensive engineering review plan. No code was modified, no patches generated, no fixes implemented.
> Produced by a Principal/Staff-level review covering architecture, security, performance, UI/UX, accessibility, testing, DevOps, and DX.
> Reference repo: `github.com/coderbuzz/dockify` (Go 1.25, SQLite, HTMX, SSH-based workers, Caddy + Cloudflare).

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Architecture Overview](#2-architecture-overview)
3. [Repository Understanding](#3-repository-understanding)
4. [Product Understanding](#4-product-understanding)
5. [Detailed Findings](#5-detailed-findings)
6. [UI/UX Findings](#6-uiux-findings)
7. [Security Findings](#7-security-findings)
8. [Performance Findings](#8-performance-findings)
9. [Architecture Findings](#9-architecture-findings)
10. [Code Quality Findings](#10-code-quality-findings)
11. [Testing Findings](#11-testing-findings)
12. [Developer Experience Findings](#12-developer-experience-findings)
13. [Optimization Opportunities](#13-optimization-opportunities)
14. [World-Class Gap Analysis](#14-world-class-gap-analysis)
15. [Prioritized Master Roadmap](#15-prioritized-master-roadmap)

---

## 1. Executive Summary

Dockify is a single-binary, self-hosted Docker deployment platform in the spirit of Coolify. Its architecture is cohesive and the developer ergonomics of its core flow (SSH → Caddy Admin API → Cloudflare DNS) are sensible. For a solo/small-team project the **breadth** of features (mock dev mode, encrypted backups, auto-select scheduler, web terminal, git CI/CD) is impressive and the **code is largely readable** with thin service/handler layers.

However, Dockify is **not yet production-grade** for a security-sensitive audience. The codebase carries a cluster of high-severity security issues, several correctness bugs, a fragile data model, weak concurrency hygiene, and thin test coverage of its riskiest paths. The UI/UX is tasteful but has accessibility gaps and a handful of real client-side bugs.

### Headline risks (must-fix before production)
- **Pervasive shell injection** across SSH `Exec` calls — `WriteFile` heredoc can be broken by content, Caddy admin ops use unquoted `%s` with user-supplied domains, app `files.path` permits path traversal to arbitrary remote-file writes.
- **`InsecureIgnoreHostKey`** disables SSH host-key verification entirely (MITM); compounded by `curl | sh` bootstraps and `docker image prune -af` global cleanup.
- **No CSRF protection** — combined with cookie auth and no `SameSite` flag, state-changing POSTs (import `replace`, roll-secret, self-update) are CSRF-reachable.
- **WebSocket console has no origin check** (`CheckOrigin: true`) — cross-site WS hijacking of interactive root shells.
- **Optional webhook secret** disabled = unauthenticated deploys → `git clone` of attacker-supplied repos → RCE.
- **Self-update pipes `curl | bash` from the internet into `systemd-run`** with no checksum/signature pinning (supply-chain RCE primitive).
- **No per-app deploy mutex** — concurrent deploys (UI + webhook + rollback) interleave `compose down`/`up` and corrupt remote state.
- **No graceful shutdown** — `srv.Close()` drops in-flight requests and does not cancel background goroutines nor the monitor; a mid-deploy exit leaves apps in `deploying` forever.
- **No migrations framework** — `db.go` accumulates `ALTER TABLE` strings with swallowed errors; missing `ON DELETE CASCADE` leaves orphaned rows and "delete server" does not undeploy its apps.
- **Secrets at rest are plaintext** (`app_secrets.value`, `apps.auth_pass`, ssh key files), and the worker `.env` is written `0644` (world-readable).
- **CPU-usage metric is cumulative-since-boot, not instantaneous** — the auto-scheduler scores against a misleading input and will pile deploys onto the wrong host.
- **Supply-chain**: GitHub Actions only run on tag push (zero CI on `main`/PRs), floating `:latest` image tags, unpinned Actions, missing SLSA provenance/SBOM/signing.

### What's already done well
- Clear, documented architecture (SPEC.md, DECISIONS.md, AGENTS.md, README.md).
- Single CSS design system with tokens, dark/light, consistent component taxonomy.
- Sensible `MaxOpenConns(1)` SQLite pragmas (WAL, FK, busy_timeout).
- Multi-stage Dockerfile with non-root user, static build, `CGO_ENABLED=0`.
- Good repository test pattern (in-memory SQLite, table-driven tests, template-render tests).
- The backup AES-256-GCM + PBKDF2-SHA256 600k parameters are OWASP-aligned.
- Smoke test with `mktemp`/`trap` cleanup; release rollback logic in `update.sh`.

### Verdict
Dockify is a strong MVP that needs a focused **1–2 month hardening pass** (sections 7 & 10 and the Critical roadmap items below) on security and correctness before it can be called "world-class production-grade."

---

## 2. Architecture Overview

```
Controller VM (single Go binary)
  ├── HTTP server (:8080, chi v5)
  │     ├── REST JSON API   /api/...
  │     ├── HTML UI         (html/template + HTMX + custom CSS)
  │     └── WebSocket console (xterm.js + gorilla/websocket + SSH PTY)
  ├── SQLite (modernc.org/sqlite, WAL, 8 tables, MaxOpenConns=1)
  ├── Service layer
  │     ├── server.Service   (CRUD, TestConnection, InitWorker, RefreshResources, Monitor)
  │     ├── app.Service       (CRUD, Deploy, Rollback, Stop/Start, FetchLogs, Secrets, Files, Routes)
  │     ├── scheduler.Scheduler (PickServer — 0.5*CPU + 0.5*RAM)
  │     ├── caddy.Client     (SSH → `docker exec caddy curl` against Admin API)
  │     ├── cloudflare.Client (HTTP to api.cloudflare.com)
  │     ├── settings.Service (webhook secret, update check, self-update)
  │     ├── backup.Service   (YAML export/import + AES-GCM encryption)
  │     └── webhook.Handler  (GitHub HMAC + GitLab token)
  ├── Background goroutines
  │     ├── monitor.Run      (60s tick → per-server SSH stats)
  │     └── deployWithCommit goroutines (unbounded, no serialization)
  └── Config                 (env vars; optional auth, mock, Cloudflare)

Worker VM
  └── Docker + Caddy container (80/443, Admin API on 127.0.0.1:2019)
        + app containers (internal `dockify` network)
        + /opt/dockify/apps/<id>/{docker-compose.yml,.env,files...}
```

### Characteristics
- **Process model**: single binary; in-memory sessions; background goroutines.
- **Persistence**: single SQLite file; no transaction layer in multi-step domains; no migrations framework.
- **Cross-process communication**: SSH only (no worker agent). Caddy Admin API accessed via `docker exec ... curl` over SSH.
- **State**: servers + apps + routes + dns + secrets + files + settings (8 tables).
- **Concurrency**: ad-hoc goroutines; no worker pool; no per-resource mutex; single DB conn.
- **Auth**: optional admin cookie (in-memory map, 24h), no CSRF, no rate limit.

### Architecture strengths & weaknesses
| | Strength | Weakness |
|---|---|---|
| Coupling | `app.ServerRepo` interface keeps app ↔ server decoupled | Router takes 14 positional args; `RenderFunc`/handler signatures duplicated across packages |
| Boundaries | Clean `internal/` package organization | `handler.go` files mix JSON + HTML + helpers (800+ lines); expected `web_handler.go`/`monitor.go` files do not exist |
| State | Schema is small and readable | Missing `ON DELETE CASCADE`; plaintext secrets; migration strategy is fragile |
| Concurrency | `MaxOpenConns(1)` serializes DB writes safely | Background goroutines have no cancellation; no per-app deploy lock; SSH client has a `session.Close()` race |
| Operations | Single binary, non-root container, smoke test | No CI on `main`/PRs; `curl|sh` installers; missing `HEALTHCHECK`, security headers, gcd signing/provenance/SBOM |

---

## 3. Repository Understanding

### Structure (actual on disk)

```
dockify/
├── cmd/dockify/main.go                 # Entry; wires deps; abrupt shutdown
├── internal/
│   ├── config/config.go               # Env config; MkdirAll errors ignored
│   ├── db/db.go + schema.sql          # SQLite; ad-hoc ALTER migrations
│   ├── scheduler/scheduler.go         # PickServer (40 lines)
│   ├── ssh/
│   │   ├── interface.go                # Connector/Input/Output/Factory
│   │   ├── client.go                   # Real SSH; InsecureIgnoreHostKey
│   │   └── mock.go                     # Dev mock; non-deterministic map
│   ├── server/{service.go,handler.go} # CRUD+Init+Monitor; ~770 lines combined
│   ├── app/{service.go,handler.go,compose.go,repository.go}
│   │                                   # Deploy/Parent pipeline; ~1700 lines combined
│   ├── caddy/client.go                # SSH→docker exec curl; injection-prone
│   ├── cloudflare/client.go           # DNS; TOCTOU in UpsertRecord
│   ├── webhook/handler.go             # GitHub HMAC + GitLab token
│   ├── settings/handler.go + settings.go # Secret/Update; string version compare
│   ├── backup/handler.go + backup.go  # YAML import/export + AES-GCM
│   └── http/
│       ├── router.go   # 14-arg NewRouter; CORS *; wildcard Prefix trust
│       ├── auth.go     # In-memory sessions (leak); no SameSite; constant-time miss
│       ├── console.go  # WS console; CheckOrigin: true; goroutine hang
│       ├── templates.go # Embeds + FuncMap; partial-render bug
│       └── templates/*.html  # 18 templates; layout.html holds all CSS + JS
├── scripts/  # install.sh (not +x), setup-worker.sh, update.sh, update-latest.sh (dup), release.sh, smoke-test.sh
├── docs/dockify-webhook-ci.md         # Indonesian; hardcodes maintainer's prod URL
├── Dockerfile, docker-compose.yml, Caddyfile, .air.toml, .env.example
├── .github/workflows/build.yml        # Tag-pushed only; missing main/PR pipeline
├── README.md, SPEC.md, AGENTS.md, DECISIONS.md, LICENSE
└── prompts/                           # Untracked by git; dangling
```

### Technologies & versions
| | |
|---|---|
| Go | 1.25.0 (`go.mod`); toolchain on review machine is 1.26.0 darwin/arm64 |
| Router | `chi v5.3.0` |
| DB | `modernc.org/sqlite v1.53.0` (pure-Go SQLite) |
| SSH | `golang.org/x/crypto v0.53.0/ssh` |
| WebSocket | `gorilla/websocket v1.5.3` |
| Extras | `humanize`, `uuid`, `yaml.v3` (indirect) |
| Frontend | html/template + HTMX 2.0.4 + xterm.js 5.3.0 (CDN, no SRI) |
| Crypto | `crypto/aes`, `crypto/cipher` (GCM), `crypto/pbkdf2` (Go 1.24+ stdlib) |

### Patterns
- Repository pattern (`*Repository`) for DB CRUD.
- Thin service layer (`*Service`) over repos.
- Handler split between JSON (`/api/...`) and HTML (web) using `RenderFunc`.
- Interface driven for cross-package deps (`app.ServerRepo`).
- Background work via raw goroutines, no context cancellation.

### Missing or weak
- **Migration framework**: `int schema_version` + ordered files.
- **Structured logging**: only `log.Printf` strings; no levels, no request IDs.
- **Metrics/tracing**: none.
- **Secrets manager**: secrets live in env + plaintext DB columns.
- **Rate limiting / CSRF / CORS**: naive or absent.
- **Worker agent**: none — all orchestration is "shell out over SSH," which is the root cause of the shell-injection class.

---

## 4. Product Understanding

### Purpose
Self-hosted infrastructure-light alternative to Coolify: deploy Docker Compose stacks onto user-owned VMs with auto-HTTPS (Caddy), auto-DNS (Cloudflare), and Git-based CI/CD, managed from a browser.

### Target users
Self-hosting operators / solo DevOps / small teams who own a few VMs and want a UI for the loop: add VM → deploy app → route + DNS → git auto-deploy.

### Business workflow (happy path)
1. Install Dockify (curl|bash install.sh — mode 1/2/3).
2. Set `DOCKIFY_ADMIN_PASSWORD` (optional — miss it = open UI).
3. Prepare worker (`setup-worker.sh` → paste private key).
4. Add server form → Save → background `TestConnection` → `InitWorker` (install Docker, network, Caddy) → `RefreshResources` → status `online`.
5. Deploy app form (Simple/Advanced, secrets, files, basic auth, git repo) → background pipeline writes compose + .env + files, `docker compose up -d`, inject Caddy route, create DNS A record, record deployment snapshot.
6. Edit redeploy; Stop/Start pause resume; Rollback to last successful snapshot; Logs via SSH tail; SSH console via WS+PTY; Auto-select least-loaded server.
7. Git push → webhook → matching app → `deployWithCommit`.
8. Backup/Restore: YAML export with optional AES-GCM passphrase; import merge or replace.

### User journey
- **Onboarding**: good (empty-state CTAs on dashboard). Strengthen: `.env.example` is incomplete; mode-3 setup nudges people to expose SSH mounts.
- **Daily ops**: detail page auto-refresh on `initializing`/`deploying` (whole-page meta refresh); status badges; resource card HTMX.
- **Break-glass**: SSH console, rollback, encrypted export.

### Major features
- Server pool, app CRUD, deploy/rollback/stop/start, secrets, config files, basic auth, multi-domain, git CI/CD, cloudflare, backup/restore, self-update, dev mock.

### Hidden / underspecced
- Self-update runs `curl|bash` via `systemd-run` (`settings.go:113`) on the controller — privileged operation with no confirmation token or audit log.
- `SET_DOCKIFY_DEV_MOCK=true` + prod: every deploy succeeds against a mock — apps appear "running" with nothing deployed.
- `docker image prune -af` and `docker builder prune -af` run on every `Undeploy` (`service.go:324-326`) — kills cached layers for *other* apps on the same worker.
- Caddy migration in `InitWorker` may stop/start the Caddy container on every Init when config files are missing — drops TLS for all apps.

### Unfinished features
- Edit App form can't clear the chosen server (`serverID==0 → app.ServerID`) and rebuilds app secrets via delete-then-restore (data loss if a row is omitted by the template).
- Multi-domain apps lose extra routes + all DNS records on backup/restore.
- "Replace" import runs synchronous SSH undeploys (can take minutes, request timeout).
- Logs not available in `failed`/`deploying` states where they're most needed.

### Duplicated / inconsistent
- `update.sh` ≈ `update-latest.sh` (110-line near-duplicate).
- Server console `toggleConsole()` ≈ app console `toggleAppConsole()` (~120 line near-duplicate).
- 3 error response styles: `http.Error`, `jsonResponse`, `render(error.html)`.
- `RenderFunc` redeclared as inline func literals in `settings` and `backup` instead of importing `http.RenderFunc`.
- Time formatting: server-side `relativeTime` vs client-side `<time>` for same populations.
- Login.html duplicates CSS palette and inverts theme-toggle icon semantics vs layout.html.

---

## 5. Detailed Findings

Critical correctness and security issues are consolidated here (with `file:line`). Security-specific ones appear in §7, performance in §8.

### Logic bugs
- **`server/service.go:234` `parseCPUUsage`** uses cumulative-since-boot `/proc/stat` columns: `($user+$nice)*100/($user+$nice+$idle)`. Idle long-uptime servers trend toward near-zero; busy-since-boot toward near-100. The displayed "CPU usage" is misleading and the scheduler scores against it → wrong host selection.
- **`app/service.go:115-124, 244, 262, 445`** no per-app deploy serialization → concurrent `deployWithCommit` interleave `compose down`/`up` on the remote.
- **`server/service.go:396-402`** unconditional `UpdateStatus(Online)` on every refresh even if every metric command errored → 0-core servers still show "online."
- **`settings/settings.go:93`** string-compare version check (`latest != current`) reports "update available" when latest is older (`1.10.0` vs `v1.2.0`).
- **`settings/settings.go:153-160`** `RegenerateWebhookSecret` UPDATE on missing (deleted) row → returns a secret that was **never persisted**.
- **`backup/backup.go:343`** relies on `Server.ID` being back-filled from `LastInsertId`; otherwise multiple imported servers all write to `<keyDir>/0.pem`.
- **`internal/scheduler/scheduler.go:35`** `best = &servers[i]` takes the address of a slice element in a per-index loop. Safe on Go 1.22+ and `go.mod` is `1.25.0` → OK, but should be copied for hygiene.
- **`layout.html:419` vs `:453` and `apps_detail.html:222` vs `:256`** — `var enc = new TextEncoder()` is declared inside `ws.onopen`; the window resize handler references `enc` outside that closure → `ReferenceError: enc is not defined` on any window resize while a console is open. Resize messages are silently dropped.
- **`error.html`** — does not include `{{template "scripts"}}`; the page renders `{{template "nav"}}` which has the theme-toggle button (`onclick="toggleTheme()"`) but the script that defines `toggleTheme` is never included → on every error page, the theme toggle throws `ReferenceError: toggleTheme is not defined` and dark-mode init never runs.
- **`apps_add.html:22-23`** duplicate `id="domains-list"` (`<div id="domains-list"><div id="domains-list">`) — invalid HTML.
- **`login.html:92,110`** theme-toggle icon is inverted vs `layout.html:321` (light=moon in login, sun in layout).
- **`smoke-test.sh:16`** `PORT=$((10000 + RANDOM % 55000))` — `RANDOM` is 0–32767, so the upper end of the range never reaches 65000.

### Race conditions & async issues
- **`ssh/client.go:123-133`** two goroutines both call `session.Close()` (the "remote exit" goroutine and the "ctx cancel" goroutine) — not documented safe for concurrent use.
- **`ssh/client.go:126`** `outCh <- Output{Closed:true}` blocks if the WS consumer stops reading; the goroutine leaks.
- **`ssh/client.go:113-117`** `WindowChange` and `Write` return values ignored → resize/keystroke drops are silent.
- **`http/console.go:84-95, 188-199`** `outCh` close branch returns without signaling `errCh`; main `select` waits only on `errCh`/`ctx.Done()` → handler can hang indefinitely if SSH closes cleanly while WS stays open.
- **`http/console.go:109`** `inCh <- ssh.Input{...}` has no `select` on `ctx.Done()` → goroutine deadlock if SSH stops reading.
- **`webhook/handler.go:66, 109`** unbounded `go DeployByGit(...)` — no semaphore; rapid webhook retries spawn unlimited concurrent deploys.
- **`cloudflare/client.go:69-84`** `UpsertRecord` non-atomic list-then-create → duplicates under concurrency.
- **`server/service.go:264-295`** monitor runs with 60s tick, spawns N goroutines (no cap), no jitter → thundering herd on DB/SSH.
- **`settings/settings.go:136-151`** concurrent `GetWebhookSecret` create-if-missing → INSERT race / duplicates.

### Data corruption & integrity
- **`db/schema.sql:23,39,50,60`** missing `ON DELETE CASCADE` on `apps.server_id`, `deployments.app_id`, `routes.app_id`, `dns_records.app_id` → delete-server leaves orphans; delete-app must manually cascade.
- **`server/handler.go:179-185`** delete-server does NOT undeploy apps on that server nor delete `<id>.pem` or routes/dns rows → orphans and key-file accumulation.
- **`backup/backup.go:290-303`** replace-mode ignores `Undeploy` and `server.Delete` errors → partial destruction then import.
- **`backup/backup.go:343`** non-atomic Create(SSHKey="pending") + WriteKey + Update; crash mid-flight leaves a `pending` server with no key file.
- **`backup/backup.go:323-394`** no DB transaction around the per-app/per-server Create + secret/file inserts → partial imports.
- **`backup/backup.go`** import does not restore multi-domain `routes` or `dns_records` (export only carries primary `Domain`) → multi-domain apps lose routing on restore, Cloudflare records orphan.

### Edge cases & null handling
- `repositories` and services consistently conflate `err != nil` and `entity == nil` — single-branch `if err != nil || x == nil { return }` — so 500 (DB error) and 404 (not found) are indistinguishable.
- `strconv.ParseInt` used across ~35 sites with `_ =` ignored errors; a non-numeric chi URL param yields `id=0` → 404 silently.
- `.env` generation escapes only `$` (not newlines/quotes) → newline-bearing secrets produce a multi-line `.env` that docker-compose parses wrong.
- `DockerComposeCmd` re-runs `command -v docker-compose` over SSH on every deploy.

### Authentication / authorization hints
- `AdminPass==""` defaults to **no auth** — the entire UI and `/api/servers` POST/GET are open. README documents this, but the danger is silent.
- No CSRF token; no rate limit; no login `Secure`/`SameSite` cookie attributes; `subtle.ConstantTimeCompare` not used for password compare.
- API unauthenticated request returns 302→HTML `/login`, breaking API clients.

---

## 6. UI/UX Findings

### Visual design
- Tasteful monospace aesthetic, clean dark/light, design tokens consistent with SPEC. Compared to Linear / Stripe Dashboard / Vercel, Dockify's restraint is a virtue but reads **flat and dense** (no iconography, low-density hero space, no animations beyond CSS hovers and a blinking mock-mode bar). Registered: "premium" but also "bare."
- Typography is hamstrung: `Berkeley Mono` / `IBM Plex Mono` are listed first in the stack but never `@font-face`-loaded → most users get `ui-monospace` silently.
- Color system is well-tuned for both themes. A few typos: `var(--muted)` (should be `--text-muted`) in `dashboard.html:53` and `apps_list.html:8` resolves to nothing → inherited color. Dead CSS: `layout.html:295` (`#0d0d0d`) contradicts `:307` (`#1a1a1a` for light mode) — light terminal fails to get its intended background until `.console-container` cascades.

### Component & interaction patterns
- **`apps_add.html:21-40`** wraps an entire Domains region in a single `<label>` with multiple `name="domain"` inputs + buttons — invalid label-to-input association and click side-effects.
- **`apps_add.html:136`** secrets injected into a `<script>` literal with `{{.Key}}`/`{{.Value}}` and no JS-string escaping → an unescaped `"`/newline breaks the script and leaks secret content into page source on every edit-page render.
- **`servers_detail.html:3` & `apps_detail.html:3`** use `<meta http-equiv="refresh" content="2">` while status is `initializing`/`deploying` → page reloads kill the xterm console, reset scroll/focus, and waste requests. Should poll a small status fragment via HTMX and swap only the badge.
- **`servers_detail.html:52` & `apps_detail.html:113`** console status badge is initialized to `badge-online`/`"Connected"` before any WS opens → misleading.
- **`servers_detail.html:25`** the full SSH private key is rendered in `<code>{{.Server.SSHKey}}</code>` inside the DOM — unless the server-side handler masks/truncates, this is private-key exposure on view-source, screenshots, and history. **Verify the handler truncates the key before passing to the template.**
- **Confirm dialogs** use `confirm('Delete {{.Name}}?...')` with the server/app name interpolated into a JS string literal unescaped — an apostrophe in `O'Brien` produces `confirm('Delete O'Brien?...')` → JS syntax error → **the form submits with confirmation bypassed**. Pattern appears in `servers_list.html:19`, `servers_detail.html:43`, `apps_list.html:24`, `apps_detail.html:104`.

### Forms
- **`servers_add.html`** `<label>Name<input></label>` wrapping — inconsistent with `servers_edit.html` which uses text+newline+input siblings.
- **No port range validation** in `servers_add.html` / `servers_edit.html` — `22` default but no `min="1" max="65535"`.
- **`apps_add.html:99,101`** file upload is a hidden `<input type="file">` with no accessible label — keyboard/AT users cannot trigger it. Remove buttons (`addEventListener`, `28`/`101`) are icon-only SVGs with no `aria-label`/`title`.
- **`apps_add.html:360`** `FileReader.readAsText` into a hidden `file_content` `<input>` has no size or type limit — a 50MB binary upload bloats and corrupts the POST.
- **Error & success blocks** (`card style=color:var(--red)` etc.) across all forms lack `role="alert"` / `role="status"` — screen readers won't announce.
- **`settings.html:77`** copy-secret has no feedback ("Copied!" for 2s) — inconsistent with `export.html:73-78` which does.
- **`settings.html:81,90,94`** `fetch(..., method:POST)` with no CSRF token, no `.catch`, no loading state; `.then(r=>r.json())` on 500/403 (`r.ok` unchecked) throws uncaught.
- **Login page (`login.html`)** signals good a11y by using `<label for>` + `id=` (unique in the codebase) but breaks everything else by duplicating the CSS palette with hardcoded values that drift from `layout.html`, and inverts the theme-toggle icon semantics (light = ☾ in login vs ☀ in layout).

### Empty / loading / error states
- Dashboard empty-state CTAs are the **best** in the app — clean copy, primary buttons.
- HTMX Refresh (`servers_detail.html:38`) has **no `hx-indicator`** → on a slow SSH fetch the UI looks frozen.
- HTMX log viewer (`apps_detail.html:146-150`) — Tail 50/200/500 buttons with no `hx-indicator`, no error target, racing requests, no tail/follow mode; logs unavailable for `failed`/`deploying` states where they're most needed.
- HTTP error handlers return 200 with the template rendered (e.g., `backup/handler.go:55-59,65-69,74-79`); status code indistinguishable from success.

### Responsiveness
- Single breakpoint at 600px collapses `.grid` only. **List tables do not get a horizontal scroll wrapper** — at mobile widths the 8-column server table and 6+ column app table clip badly. Add `.table-wrap { overflow-x:auto }`.

### Time consistency
- `servers_resources_card.html` uses server-side `relativeTime`. `apps_detail.html:130` uses `<time datetime="...Z">` + JS formatter. Some timestamps in `apps_detail.html` are raw `{{.CreatedAt.Format "2006-01-02 15:04:05"}}` (UTC, no `<time>`) — adjacent rows get localized vs not depending on which field. Two strategies coexist.

### A11y (target WCAG AA — see §7-ish consolidated)
- **Theme toggle** (`layout.html:325`) has `title` only, no `aria-label`/`aria-pressed`.
- **Console toggle** (`servers_detail.html:39`, `apps_detail.html:101`) no `aria-expanded`.
- **`layout.html:278`** .dev-mock-bar blink animation — no `@media (prefers-reduced-motion)` guard.
- Tables lack `<caption>`; `<th>` lacks `scope`.
- Status badges convey meaning by word (good) but `+N more domains` badge lacks a tooltip listing the extras.
- Login is the most accessible form (label-for).

---

## 7. Security Findings

Severity-tagged. Critical = immediate fix; High = before production; Medium = hardening; Low = hygiene.

### 🚨 Critical
1. **`ssh/client.go:36`** `HostKeyCallback: gossh.InsecureIgnoreHostKey()` disables SSH host-key verification entirely → MITM on every worker connection.
2. **`ssh/client.go:67-74` `WriteFile`** builds `cat > 'path' << 'DOCKIFY_EOF'\ncontent\nDOCKIFY_EOF`. `path` is single-quoted but not escaped (single-quote breaks it). The heredoc delimiter is fixed — content containing a line `DOCKIFY_EOF` terminates early and the remainder executes as shell. Compose/app file content is user-supplied.
3. **`app/handler.go:240-259 SetFile` + `app/service.go:170-176`** — `files.path` is stored verbatim and joined as `remoteDir + "/" + path` via `client.WriteFile → mkdir -p $(dirname '...')`. No `..` traversal check → arbitrary file write on the worker, as root. **RCE primitive.**
4. **`app/service.go:165`** `.env` written with `0644` mode → world-readable secrets on shared workers.
5. **`caddy/client.go:81-84, 101-104, 117-119, 131-135, 159-163`** `sanitizeID` only replaces `.` and `*`; route IDs and `https://domain` are interpolated raw/unquoted into `docker exec ... curl` shell commands → command injection from an attacker-controllable app domain.
6. **`http/console.go:18-22`** `Upgrader.CheckOrigin: func(r *http.Request) bool { return true }` — WebSocket accepts upgrades from any origin → CSWSH: a malicious page can open a WS to the console and run shell commands as the admin's browser auto-sends the session cookie.
7. **`webhook/handler.go:57-63`** when webhook secret is disabled/empty, signature verification is skipped entirely → unauthenticated deploys. Deploy clones `payload.Repo.CloneURL` (attacker-controlled) → `git clone` arbitrary repos on the worker → RCE.
8. **`settings/settings.go:107-127` + `settings/handler.go:73`** self-update writes `/tmp/dockify-upgrade.sh` (0755, world-writable dir → symlink race) and `curl -fsSL https://raw.githubusercontent.com/.../update.sh | bash` via `systemd-run`. No checksum, signature, or pinning. Repo compromise or TLS/DNS hijack = RCE as root.
9. **`docker-compose.yml:11`** `${HOME}/.ssh:/home/dockify/.ssh:ro` mounts the host user's ENTIRE `~/.ssh` (incl. `id_rsa`, `known_hosts`, config) into the container — container compromise = host SSH key material theft.

### 🚠 High
10. **No CSRF protection anywhere** (all POST routes). `backup/handler.go` import `replace` wipes all data; `settings/handler.go:73` `RunUpdate` triggers self-update. Combined with cookie auth lacking `SameSite` and no CSRF token, these are CSRF-reachable.
11. **`auth.go:13`** in-memory session store: lost on restart, not multi-instance, expired entries never reaped (memory leak).
12. **`auth.go:46-52`** cookie missing `Secure`, explicit `SameSite`, and `__Host-` prefix.
13. **`auth.go:100`** password compare uses `==` — non-constant-time timing leak.
14. **`webhook/handler.go:102`** GitLab token compared with `!=` — same leak (GitHub uses `hmac.Equal`, good).
15. **`server/service.go:108`** `command -v docker || curl -fsSL https://get.docker.com | sh` over SSH with `InsecureIgnoreHostKey` — supply-chain RCE on the worker at install time.
16. **`app/service.go:324-326`** `docker image prune -af` and `docker builder prune -af` on every Undeploy → global cleanup on a shared worker; affects other apps' caches.
17. **`backup/backup.go:77`** PBKDF2 (600k iters) re-derived **per encrypted field** → multi-second exports/imports; not a security issue per se but slows the hot path.
18. **`backup/backup.go`** no whole-document integrity (only per-field GCM) → unencrypted fields (server host, app compose, domain → port) are freely mutable. A maliciously edited export could silently redirect a domain.
19. **`backup/handler.go:24-29`** export filename identical for encrypted vs unencrypted → users accidentally export plaintext secrets to disk.
20. **`backup/handler.go:63`** unbounded `io.ReadAll(file)` on import → memory DoS.
21. **`backup/backup.go`** export with no passphrase writes plaintext SSH keys + secrets to YAML → trivial secret theft if file is committed/shared.
22. **`webhook/handler.go:35`** unbounded `io.ReadAll(r.Body)` on webhook POST → DoS.
23. **`router.go:227` `PrefixMiddleware`** trusts `X-Forwarded-Prefix` header from any client unless an upstream strips it → BasePath manipulation vector.
24. **`router.go:55-57`** `/api/settings/update/check` is public (no auth) → leaks current version.
25. **`http/console.go:176-177`** builds `compose exec <serviceName> sh -c '...'` with `serviceName` derived from the app name via `ContainerServiceName()`. If sanitization misses single quotes, `'` breaks the inner `sh -c` single-quoted string → command injection at exec time. **Verify `ContainerServiceName()` rejects `'`**.
26. **Background deploy with no host-key check** — `InsecureIgnoreHostKey` on the deploy SSH path means a spoofed worker can capture `.env` secrets.
27. **`webhook/handler.go:53`** tag pushes (refs/tags/...) accepted as `event=="push"`; `strings.TrimPrefix(ref,"refs/heads/")` leaves `refs/tags/x` → bogus branch input passed to deploy.

### ⚠️ Medium
28. **`server/handler.go:34-85 Create`** on `saveKeyFile` failure, row stays with `ssh_key="pending"` and **no cleanup**.
29. **`server/handler.go:158-174 Refresh`** fire-and-forget; no in-flight dedup → N refreshes = N SSH connections.
30. **`server/handler.go:440-446 / 448-464`** synchronous SSH operations in request handler → up to ~60s staleness.
31. **`app/service.go:266-290 FetchLogs`** `tail` unbounded → `?tail=999999999` streams entire log history into memory; DoS.
32. **`app/handler.go:199-218 SetSecret`** accepts arbitrary `key` (allows `=`, newlines) → breaks `.env` generation.
33. **`backup/backup.go:102-104`** `decrypt` silently passes through non-`enc:` values → passphrase-unused silent success.
34. **`backup/backup.go:296`** `replace` import calls synchronous `Undeploy` per app → minutes-long request, no per-app failure surface.
35. **No request body limits** anywhere (`io.ReadAll` in webhooks, uploads, cloudflare responses) → DoS surface.
36. **`http/console.go:34-226`** ~90% of the code is duplicated for server vs app console; no connection cap; no read/write deadlines.
37. **`settings.go:52-57`** update-check misses the cache → N concurrent requests hit GitHub Releases API → rate limit.
38. **`settings.go:136-151`** webhook secret creation race.
39. **`server/service.go:123-137`** Caddy init writes a default global config that opens `:80` and `:443` on the worker with no auth by default.

### Low
40. **`auth.go:87-105`** login logs success not failure; no IP/UA; no brute-force protection.
41. **`auth.go:83`** API requests get HTML redirect (302→`/login`) instead of 401 JSON.
42. **`cloudflare/client.go:175`** `?name=<name>` not URL-encoded.
43. **`cloudflare/client.go`** all responses use `io.ReadAll` with no `LimitReader` cap.
44. **`caddy/client.go:56`** bcrypt cost 14 ⇒ 1–2s per `AddRouteWithAuth` call.
45. **`settings/handler.go:42-49`** `GetWebhookSecret` returns the raw secret in the API JSON.
46. **Secret-on-disk permissions**: `.env` is `0644`; key files `<id>.pem` are `0600` (good); `install.sh:84` writes `.env` with default world-readable perms.
47. **Static assets**: no `Cache-Control`/immutable headers; relative `http.Dir("web/static")` breaks if cwd changes.

---

## 8. Performance Findings

| Hot path | Issue | Impact |
|---|---|---|
| **`db/db.go:20`** `SetMaxOpenConns(1)` + `MaxIdleConns` unset | Every request serialized on one conn; a slow `RefreshResources` (6 SSH round-trips) blocks dashboard `SELECT` for seconds. | Single-user OK; multi-tenant ceiling unclear. |
| **`server/monitor.go` (in service.go:264-295)** | 60s tick spawns N goroutines with no cap/jitter; each opens a fresh SSH connection and runs 6 sequential commands. | With many servers, thundering herd on SSH + DB. |
| **`server/service.go:186-191 RefreshResources`** | 6 sequential SSH `Exec` round-trips per refresh. | For 60s per server, each adds ~150ms; with 50 servers adds 7.5s every minute of bridge time. |
| **`app/service.go:561` `DockerComposeCmd`** | Re-runs `command -v docker-compose` over SSH on **every deploy**. | Extra ~100ms round-trip per deploy. |
| **`backup/backup.go:77`** | PBKDF2 600k iterations re-derived **per field**. | Export with 50 secrets × 600k iters = several seconds. |
| **`backup/backup.go:428-452 findServerByName/findAppByName`** | O(n²) name lookups during import. | Slow for large exports. |
| **`app/service.go:508-540 DashboardStats`** | Loads all apps + servers into memory to count. | Fine for small N; should be `SELECT COUNT(*)`. |
| **`app/service.go:542-554 recordDeployment`** | Stores full compose snapshot per deploy; no retention/prune. | DB grows linearly with deploy frequency. |
| **`caddy/client.go:180-190 WaitForCertificate`** | Busy-poll every 2s, no backoff, no context cancellation; holds SSH session open up to `timeout`. | 60s blocking during each deploy. |
| **`caddy/client.go:81`** `docker exec caddy curl` over SSH for every admin API call | Per-request SSH connection + exec round-trip. | Latency floor for all routing ops. |
| **`http/console.go`** | WS read/write with no deadlines. | Stalled clients pin goroutines. |
| **CSS/JS** | htmx + xterm loaded on every page; no conditional loading. | Conservative ~50–150 KB paid on pages that don't use xterm (settings, about, login). |
| **`db.go` ad-hoc migrations** | `UPDATE apps SET compose_mode = 'simple' WHERE unique_service_name = 1;` runs **every boot** → full table scan. | Linear in app count. |
| **`db.go` no `mmap_size`, `cache_spill`, `synchronous=NORMAL`** | Default `FULL` is slow under WAL. | Minor write-throughput loss. |
| **No indexes** on `apps.server_id`, `routes.app_id`, `deployments.app_id ORDER BY created_at`, `dns_records.name`. | | Schedulers and history queries full-scan as data grows. |
| **`server/service.go:204-262 parseCPUUsage`** samples once (no delta) | Cumulative-since-boot metric — barely moves. | Not perf, but feeds scheduler (correctness). |
| **HTMX refresh** without `hx-indicator` | UI appears frozen on slow SSH fetch. | UX perf, not throughput. |

### Estimated impact
- A 10-server, 50-app deployment will see:<br>
  • Refresh burst: ~60s every minute pinned by per-server SSH round-trips; up to 10 simultaneous connections; DB blocked for >10s.
  • Each deploy: +100ms for compose-cmd probe; recordDeployment row growth; 60s cert poll.
  • Backup/import: with ≥30 secrets, several seconds of pure KDF compute; with 50+ rows, O(n²) lookups.

---

## 9. Architecture Findings

1. **Worker orchestration via "shell out over SSH"** is the root cause of the injection class and a performance ceiling. Recommend either (a) a small worker agent (gRPC/HTTP) or (b) SFTP for all file ops and structured `docker` CLI escaping for command ops.
2. **Caddy Admin API via `docker exec caddy curl` over SSH** is brittle and injection-prone. If Caddy's admin API is reachable on a forwarded port from the controller, a direct HTTP client (`net/http`) eliminates the shell-escaping class.
3. **No per-resource deploy serialization.** A `sync.Mutex` per `app.ID` (or a per-app "deploying" flag in DB backed by a lease) is essential before webhook+UI+rollback can coexist safely.
4. **No graceful shutdown.** `http.Server.Shutdown(ctx)` + `context.WithCancel` at root propagated to monitor/deploy goroutines via a package-level `Context`.
5. **No migrations framework.** Adopt `PRAGMA user_version` + ordered `*.sql` files (or `golang-migrate`/`goose`).
6. **Schema integrity** — add `ON DELETE CASCADE` to all FKs; add an `ssh_key_status` enum or nullable ssh-key column to remove the `"pending"` sentinel; consider `UNIQUE(servers.name)` to make merge imports idempotent.
7. **Plaintext secrets** — if SQLiteCipher isn't desired, add app-level encryption with a per-instance root key (env/keystore).
8. **In-memory sessions** — move to a `sessions` table (signed token, expiry, last-seen) and a reaper goroutine; enables multi-instance.
9. **Background deploys** returning no error to caller — deploy failures only land in the deployments table and `log.Printf`; no alerting, no UI banner on the dashboard.
10. **Handler file shape** — the audited `server/handler.go` (481 lines) and `app/handler.go` (869 lines) mix JSON + HTML + helpers. Split into `api_handler.go` + `web_handler.go` (which is what users expected from the SPEC).
11. **HTTP router `NewRouter` 14 args** — replace with a `RouterDeps` struct (or a small wire-DI helper).
12. **RenderFunc duplication** — `settings/handler.go` and `backup/handler.go` inline the signature; import `http.RenderFunc`.
13. **CORS misconfigured** — `Access-Control-Allow-Origin:*` on cookie-auth routes; `Allow-Methods` lists `PUT` which no route uses (only `PATCH`). Remove or restrict to known origins.
14. **No request IDs / structured logging** — link traces between background deploy and HTTP request.
15. **Backup/restore** — restore is destructive without a confirmation token or dry-run; no audit log of who exported/imported.
16. **No feature/version compatibility story for exports** — strict version equality rejects future versions; consider forward-compatible schema with default-fallback.

---

## 10. Code Quality Findings

### Duplicated code
- `update.sh` ≈ `update-latest.sh` (~110 lines).
- Server console `toggleConsole()` ≈ app console `toggleAppConsole()` (~120 lines).
- `ssh/client.go` `ExecPTY` ≈ `Shell` (~70 lines).
- `ssh/mock.go` `ExecPTY` ≈ `Shell` (~70 lines).
- 3 error response styles (`http.Error`, `jsonResponse`, `render(error.html)`).
- `RenderFunc` redeclared inline in `settings` & `backup`; `render(w,r,status,name,data)` 4-arg helper duplicated.

### Dead code
- `app/service.go:564` `var _ = time.Now` (suppressed unused import; `time` is used elsewhere, so this is leftover).
- `ssh/mock.go:52,123` `time.NewTicker(30ms)` inside `select` with empty body — useless timer.
- `backup/handler.go:24-29` if/else with identical arms (`Content-Disposition` filename).
- `layout.html:295` `console-container` background contradicted by `:307`.
- `router.go:244` `PUT` in CORS allow-list unused by any route.

### Anti-patterns
- `handler.go` everywhere: `if err != nil || x == nil { return }` — conflates 404 and 500.
- `strconv.ParseInt(... .URLParam("id")...)` with `_ =` on ~35 sites.
- `_ =` ignored returns for `UpdateStatus`, `SaveRoute`, `SetSecret`, `SetFile`, `WindowChange`, `conn.WriteMessage`, `client.Exec`.
- Pervasive `fmt.Sprintf` shell string construction (root of injection class).
- `InsecureIgnoreHostKey`, `CheckOrigin: true` — "easiest working at first launch" defaults that silently reduce security.
- `confirm('{{.Name}}?...')` JS-string interpolation from user input.
- `.env` generation with single-character `$` escaping only.

### God / long methods
- `app/service.go` `deployWithCommit` (~125 lines) and `setupRouteAndDNSForDomain` (~90 lines) mix many responsibilities: status update, secret env write, file write, network ensure, compose parse, down, pull, up, prune, route, DNS, cert-wait, deploy record. Worth decomposing into named phases.

### Tight coupling / poor abstractions
- `caddy.Client` depends on `ssh.Connector` for an HTTP-API-style operation. Better: a generic "remote HTTP-over-SSH-tunneled-port" client.
- `app.ServerRepo` interface is narrow (`List()` only) — good; but `app.Service` still reaches into the `server` package for `*server.Server`. Narrow via a typed DTO.

### Inconsistencies
- `_ = ` results vs explicit `return` for the same kind of error in adjacent files.
- `recordDeployment` takes a free-form status string (`"success"`) instead of a constant.
- Status vocabulary: schema default `"created"` for apps, `"pending"` for servers, plus ad-hoc `"initializing"`, `"success"`, `"failed"` used inconsistently between schema defaults, `const` blocks (`server.StatusOnline`, etc.), and literal strings.
- `repository_test.go:110-114` asserts `Get(1)` returns ID=1 — brittle to schema migrations.
- `login.html` CSS palette drifts from `layout.html`.

---

## 11. Testing Findings

### What exists (5 test files)
| File | Covers | Verdict |
|---|---|---|
| `internal/http/templates_test.go` | Parse + render all 16 templates | Strong — rare in Go projects |
| `internal/scheduler/scheduler_test.go` | `PickServer` least-loaded | Good — empty/offline/single/mixed/load ordering |
| `internal/config/config_test.go` | Env defaults, override, `Addr`, `DBPath` | Good |
| `internal/app/compose_test.go` | `generateCompose`, `parseServiceNames`, `sanitizeAppName`, `renameFirstService`, `parseSimpleFields` | Very good — table-driven + edge cases |
| `internal/app/repository_test.go` | App + Deployment CRUD on in-memory SQLite | Good |

### Critical gaps
- **No tests for the deploy pipeline** (`app.Service.deployWithCommit`), the most complex+racy code in the project.
- **No tests for `server.Service`** (InitWorker, RefreshResources, Monitor, parseCPUUsage).
- **No tests for `ssh.Client`** intended integration (Connect, Exec, WriteFile, ExecPTY race).
- **No tests for `caddy.Client`**, `cloudflare.Client`, `webhook.Handler`, `settings.Service`, `backup.Service` (incl. crypto round-trip).
- **No HTTP-level integration tests** (auth, CSRF, `/api/servers` CRUD).
- **No race detector in CI** — `go test ./...` without `-race` misses the `session.Close()` race, channel-send blocking, monitor map writes.
- **No coverage gate** — `go test -cover` uncovered.

### Flakiness risks
- `ssh/mock.go` `Exec` uses map-key substring matching with **non-deterministic** iteration → mock-based tests can return different responses per call.
- `smoke-test.sh:16` port range formula is broken; collision-prone on shared CI.

### Recommended test surface
1. Deploy pipeline end-to-end against a mock SSH that asserts receive-order and contents.
2. Path-traversal regression for `files.path`.
3. HMAC + GitLab token constant-time compare tests.
4. Backup encrypt/decrypt round-trip; partial/failing import; multi-domain route preservation.
5. Concurrent deploy race (`go test -race`) with a per-app mutex assertion.
6. Auth flow: login→cookie→logout→expired→n-conn; CSRF token round-trip.
7. Caddy route sanitizeID shell-escape regression.
8. Console WS origin-check rejection.

---

## 12. Developer Experience Findings

### What's good
- `AGENTS.md`, `SPEC.md`, `DECISIONS.md` are substantive and accurate.
- `.air.toml` live-reload works; `stop_on_error=false` keeps the old server running on build error.
- In-memory SQLite test pattern is fast and isolated.
- Clear branch + commit message conventions in AGENTS.md.
- Smoke test gates releases.

### Gaps
- **CI runs only on tag push** (`build.yml:4-5`) → no `go vet`/`go test`/smoke on `main` or PRs; a broken main merges undetected.
- **`.env.example`** is missing `DOCKIFY_BASE_PATH`, `DOCKIFY_DEV_MOCK`, `DOCKIFY_SSH_KEY_DIR` (mismatch with SPEC/install.sh).
- **`.gitignore`** misses `build-errors.log` (Air log); `prompts/` is untracked and unignored (dangling).
- **`.dockerignore`** is too thin — `COPY . .` pulls docs/scripts/templates into the builder context; cache-busts unnecessarily.
- **`install.sh`** is `rw-r--r--` (not executable) while sibling scripts are `+x` — inconsistent.
- **`go.mod`** uses `crypto/pbkdf2` (Go 1.24+) — portability note for older toolchains.
- **`release.sh:11`** uses `sort -V` (GNU-only) — fails on macOS default `sort`.
- **`docs/dockify-webhook-ci.md:104`** hardcodes the maintainer's production URL (`dockify.amg.id`) in user-shipped docs.
- **`DECISIONS.md`** ADRs lack dates/status; ADR-007 says "Pico CSS" but the implementation is fully custom CSS (`layout.html`) — drift.
- **`docs/`** Indonesian alongside English elsewhere — audience inconsistency.
- **GitHub Actions** are `@vN` not SHA-pinned; no `dependabot.yml`; missing `govulncheck`, `golangci-lint`, `-race`, SBOM, cosign signing, provenance.
- **No `concurrency:` block** in workflow → parallel tag runs.
- **`Dockerfile`** lacks `HEALTHCHECK`; Alpine 3.21 reaching EOL; no OCI labels; no `LABEL` for version.
- **`docker-compose.yml`** mounts host `~/.ssh` (security) and uses floating `:latest` tags.
- **`Caddyfile`** has no security headers (`HSTS`, `X-Content-Type-Options`, `Referrer-Policy`, CSP), no `encode gzip`, no Caddy `/config` persistence volume.
- **Logging** is `log.Printf` only — no levels, no request IDs, no JSON option.
- **No `version` wiring on default `go build`** — `var version = "0.1.0"` only overridden via `-ldflags`. Document the CI override for reproducibility.

---

## 13. Optimization Opportunities

| # | Opportunity | Impact | Complexity |
|---|---|---|---|
| 1 | Replace `WriteFile` heredoc with SFTP transfer | Eliminates injection class; removes delimiter collision; binary-safe | M |
| 2 | Single Caddy Admin API HTTP client (forward the port once per InitWorker) instead of `docker exec caddy curl` | Latency -100ms/op; removes shell surface | M |
| 3 | Per-app `sync.Mutex` (or DB deploy-leasing column) | Eliminates deploy races | S |
| 4 | `RefreshResources` via one remote script returning all six metrics | 6 round-trips → 1 | S |
| 5 | Cache compose-cmd detection per server | -1 SSH round-trip/deploy | S |
| 6 | Add SQLite indexes + `synchronous=NORMAL` + `mmap_size` | Faster scheduler/history/load-list | S |
| 7 | Adopt migration framework (`PRAGMA user_version`) | Predictable upgrades; no boot-time full table scans | M |
| 8 | Real CPU usage via two `/proc/stat` samples 1s apart | Scheduler correctness; UX honesty | S |
| 9 | Move sessions to DB token table + `SameSite=Lax` + `Secure`; reaper goroutine | Multi-instance; no leak; CSRF-resistant | M |
| 10 | CSRF token middleware + double-submit cookie | Eliminates CSRF surface | M |
| 11 | Structured logging (e.g. `log/slog`) with request IDs | Production observability | S |
| 12 | `io.LimitReader`/`http.MaxBytesReader` on all body reads | DoS hardening | S |
| 13 | Constant-time compares + `Secure`/`__Host-` cookie | Auth hardening | S |
| 14 | Conditional HTMX+xterm loading via `{{template}}` blocks | Reduces JS payload on irrelevant pages | M |
| 15 | Hoist `enc = new TextEncoder()` out of `ws.onopen` | Fixes resize ReferenceError | S |
| 16 | Replace `<meta http-equiv="refresh">` with HTMX badge polling | Preserves console during init/deploy; cleaner UX | M |
| 17 | `.table-wrap { overflow-x:auto }` for all multi-col tables | Mobile readability | S |
| 18 | Whole-document verification token in backups (HMAC over the YAML) | Eliminates silent mutation of unencrypted fields | S |
| 19 | Decrypt-validate pass-once KDF (derive key once per export/import) | 10× faster backup/restore | S |
| 20 | Add `HEALTHCHECK` + security headers + gzip on Caddyfile | Production hardening | S |
| 21 | Pin image digests + Actions SHAs + Dependabot | Reproducibility + supply-chain | S |
| 22 | CI pipeline on `push`/`pull_request` + `-race` + `golangci-lint` + `govulncheck` | Catches broken main; SSA bugs detected | S |
| 23 | Per-server SSH connection pool held by the service | Removes connection churn; performance for high-N | M |
| 24 | Add `concurrency:` + `workflow_dispatch` to build.yml | Manual re-runs; no parallel tag races | S |
| 25 | Scoped `docker compose down --rmi all --volumes` instead of `image prune -af` | Doesn't kill other apps' caches | S |
| 26 | Realtime cert + log streaming via SSE/WebSocket | UX polish (live logs) | L |
| 27 | Worker agent (gRPC) replacing shell-out | Eliminates injection class; unlocks richer remote ops | XL |
| 28 | Audit log table for deploys, deletes, exports, imports, self-updates | Forensics/regulation | M |

---

## 14. World-Class Gap Analysis

Benchmarked against Google/Apple/Stripe/Airbnb/Figma/Notion/Vercel/Cloudflare.

| Dimension | Today | World-class bar | Gap |
|---|---|---|---|
| **Security** | Multiple RCE-class issues active; optional auth optional; no CSRF; CSWSH on WS; supply-chain `curl|bash`. | Defense in depth, least-privilege, SLSA-3 release pipeline, signed artifacts, pen-test passes. | Large. The injection class, host-key checking, CSRF, CSWSH, supply-chain signing must all be closed before claiming "production-grade." |
| **Reliability** | Abrupt shutdown; no per-app deploy serialization; monitor leaves stuck servers; failed deploys leave app fully down. | Graceful shutdown, on-call-ready telemetry, automatic rollback on deploy failure, blue/green deploys. | Medium. Per-app mutex + graceful shutdown + fail-forward semantics needed. |
| **Observability** | `log.Printf` only. | Structured logs, metrics, traces, request IDs. | Large. Basic observability absent. |
| **Performance** | SSH-driven orchestration; per-event 6 round-trips; no connection pool; SQLite bottleneck for high-N. | Sub-100ms control-plane ops; bounded request latency; horizontally scalable. | Medium. Fine at small scale; architectural ceiling clear. |
| **Architecture** | Thin and readable but tightly coupled to shell-out; no transactions; no migration story. | Clear domain boundaries, typed ports/adapters, transactional multi-step domains, versioned schema. | Medium. |
| **Code quality** | Mixed 500–800-line handler files; ignored errors pervasive; ad-hoc status strings; duplicated xterm script. | Consistent error handling, narrow handler files, single source of truth for status vocabulary. | Medium. |
| **UI/UX polish** | Restraint good; theme FOUC, console resize bug, error-page broken theme, meta-refresh killing terminal, no a11y on icons/forms, mobile tables clipped. | Pixel-tight, accessible, no regressions, instant feedback, offline graceful. | Medium. Specific bugs above are concrete blockers. |
| **A11y** | `var(--muted)` typos, missing `aria-*`, no `@media (prefers-reduced-motion)`, broken focus/escalation on error pages. | WCAG AA; full keyboard + screen reader; reduced-motion respected. | Medium. |
| **Testing** | ~5 test files; no deploy/SSH/auth/webhook/backup/HTTP coverage; `-race` absent. | Coverage gate (~80%), integration + E2E, race-clean, mutation testing. | Large. |
| **DevOps** | Tag-only CI, floating tags, unpinned actions, no SBOM/sign/provenance, `.dockerignore` thin, `~/.ssh` mount. | Trunk/PR CI with quality gates; SHA-pinned deps; signed/SBOM/provenance; least-privilege mounts. | Medium-Large. |
| **Docs** | SPEC/DECISIONS/README solid; partial docs in Indonesian; maintainer's prod URL in user docs; ADR status absent; `.env.example` incomplete. | Bilingual intentional choice or English-only; user-shipped docs sanitized; ADRs dated/statused. | Small-medium. |
| **Backup** | Crypto good (AES-256-GCM, OWASP-aligned KDF), but no whole-document integrity, replace-mode destructive without confirmation, plaintext path, partial import possible, multi-domain lossiness. | Whole-document auth, dry-run, confirmed destructive, lossless multi-domain/restore. | Medium. |
| **Day-2 ops** | Self-update is a root-of-trust footgun; monitor can't recover stuck init; no alerting; no healthchecks. | Signed updates, watchdog, alerting, idempotent re-init recovery. | Medium. |

---

## 15. Prioritized Master Roadmap

Each item: **Title → Category → Risk → Complexity → Benefit → Solution → Files → Dependencies → Acceptance Criteria.**

### 🚨 Critical — must fix immediately

#### C1. Eliminate the shell-injection class via SSH/SFTP + structured args
- **Category:** Security / Architecture
- **Root cause:** `WriteFile` heredoc with fixed delimiter; `client.Exec` builds shell strings with `fmt.Sprintf`; `caddy.Client` uses `docker exec caddy curl` with unquoted `%s`; `files.path` userspace joining without `..` guard.
- **Current impact:** Arbitrary command execution / arbitrary file write on worker (as root). Confidence: high.
- **Risk level:** Critical
- **Complexity:** L
- **Expected benefit:** Removes an entire category of RCE-class issues; unblocks worker agent future.
- **Suggested solution:** (a) Use `golang.org/x/crypto/ssh/sftp` for `WriteFile` (binary-safe, no shell). (b) For Caddy operations, prefer a direct HTTP call over a port-forwarded Caddy admin API; if staying with SSH, quote all interpolated values with `shellescape` and validate allowed char set. (c) Anchor `files.path` under the app dir; reject `..`, absolute paths, or control chars.
- **Files:** `internal/ssh/client.go`, `internal/caddy/client.go`, `internal/app/service.go`, `internal/app/handler.go`
- **Dependencies:** none
- **Acceptance criteria:** (1) No `fmt.Sprintf` shell string contains user-controllable substring unquoted. (2) `SetFile` rejects paths containing `..` / `/`-prefixed / control chars. (3) Existing tests + new path-traversal regression pass; `gosec` G204 cleared.

#### C2. Enforce SSH host-key verification (TOFU)
- **Category:** Security
- **Root cause:** `InsecureIgnoreHostKey()` (client.go:36).
- **Current impact:** MITM on every worker connection (incl. `.env` writes & `curl|sh` bootstrap).
- **Risk level:** Critical
- **Complexity:** S
- **Expected benefit:** Closes MITM; future-proof for self-update integrity.
- **Suggested solution:** On first `TestConnection`, capture `host.PublicKey()` and persist to `servers.host_key` (schema migration). Subsequent connections use a `HostKeyCallback` that compares against the stored key; first-seen is a flagged UI step (operator confirms fingerprint).
- **Files:** `internal/ssh/client.go`, `internal/server/service.go`, `internal/server/handler.go`, `schema.sql`
- **Dependencies:** C1
- **Acceptance criteria:** (1) `grep InsecureIgnoreHostKey` returns nothing. (2) First connect stores the key; second connect with a different key fails with a clear error. (3) New test verifies rejection.

#### C3. Validate webhook signatures always; reject empty-secret receives
- **Category:** Security
- **Root cause:** `webhook/handler.go:57-63` skips verification when secret is empty/disabled.
- **Current impact:** Unauthenticated deploys + `git clone` of attacker repos = RCE on worker.
- **Risk level:** Critical
- **Complexity:** S
- **Expected benefit:** Webhook endpoints are auth-gated even without operator secret.
- **Suggested solution:** (a) Disable webhook endpoints entirely when no secret is configured (reject all POSTs with 503 "Secret not configured"). (b) Compare GitLab token with `subtle.ConstantTimeCompare`. (c) Match the incoming `repo` against a registered app's `git_repo` before trusting the payload. (d) Reject `refs/tags/*` pushes.
- **Files:** `internal/webhook/handler.go`, `internal/app/service.go`
- **Dependencies:** none
- **Acceptance criteria:** (1) Empty secret ⇒ endpoints return 503 "secret not configured" and never fire `DeployByGit`. (2) GitLab uses `subtle.ConstantTimeCompare`. (3) Push with `refs/tags/...` returns `200 "ignored"`. (4) Tests cover empty-secret, mismatched, tag-push, token timing.

#### C4. WebSocket console origin allowlist
- **Category:** Security
- **Root cause:** `Upgrader.CheckOrigin: func(r *http.Request) bool { return true }` (console.go:18).
- **Current impact:** Cross-site WS hijacking of root shells via victim's browser.
- **Risk level:** Critical
- **Complexity:** S
- **Expected benefit:** Only Dockify origins can open the console.
- **Suggested solution:** Reject when `r.Header.Get("Origin")` doesn't match the configured `DOCKIFY_HOST`/explicit allowlist. Optionally add WS auth token signed by the session cookie.
- **Files:** `internal/http/console.go`
- **Dependencies:** none
- **Acceptance criteria:** (1) Cross-origin WS upgrade returns 403. (2) Same-origin upgrade still works.

#### C5. Path-traversal guard + worker `.env` 0600
- **Category:** Security / Hardening
- **Root cause:** `app/handler.go:240` `SetFile` stores path verbatim; `app/service.go:165` writes `.env` `0644`.
- **Current impact:** Arbitrary file write as root; world-readable secrets on shared workers.
- **Risk level:** Critical
- **Complexity:** S
- **Expected benefit:** Files anchored; secrets readable only by owner.
- **Suggested solution:** Validate `path` against `^[A-Za-z0-9_./-]+$`, reject `..` and absolute paths; join with `filepath.Join` and verify the result is under the app dir. Use `0640` or `0600` for the worker `.env`.
- **Files:** `internal/app/handler.go`, `internal/app/service.go`
- **Dependencies:** C1
- **Acceptance criteria:** (1) Path like `../../etc/cron.d/evil` returns 400. (2) `ls -l /opt/dockify/apps/<id>/.env` shows `-rw-------`.

#### C6. Hardening self-update pipeline (signed, checksum, atomic)
- **Category:** Security / DevOps
- **Root cause:** `settings.go:113` pipes `curl` to `bash` via `systemd-run`; written to `/tmp` (0755, world-writable) — symlink race.
- **Current impact:** Repo/DNS/TLS compromise → RCE as root.
- **Risk level:** Critical
- **Complexity:** M
- **Expected benefit:** Reproducible, integrity-checked updates; no symlink race.
- **Suggested solution:** (a) Ship a SHA256 checksum file alongside the release binary; verify before `mv`. (b) Write the helper script to `/var/lib/dockify/.upgrade/<nonce>/` with mode 0700 and use `mkstemp`-style private path. (c) Pin the script's commit SHA in the URL instead of `main`. (d) Sign binaries with cosign and verify at run time.
- **Files:** `internal/settings/settings.go`, `scripts/update.sh`, `.github/workflows/build.yml`
- **Dependencies:** C2 (drives trust in fetched artifacts)
- **Acceptance criteria:** (1) `curl|bash` is replaced with a downloaded-then-verified script. (2) Helper file lives under a 0700 root-only directory. (3) Checksum verification step exists in update.sh. (4) CI publishes `checksums.txt` and a cosign signature.

#### C7. CSRF protection + `SameSite=Strict` session cookie
- **Category:** Security
- **Root cause:** No CSRF token middleware; cookie has no `SameSite`/`Secure` attributes (`auth.go:46`).
- **Current impact:** CSRF can trigger `import replace` (data wipe), `RunUpdate` (RCE), roll-secret, undeploy, delete-app.
- **Risk level:** Critical
- **Complexity:** M
- **Expected benefit:** Defends all state-changing endpoints.
- **Suggested solution:** CSRF token middleware (double-submit cookie or gorilla/csrf). Set cookie `HttpOnly; Secure; SameSite=Strict; __Host-` prefix (when served over HTTPS).
- **Files:** `internal/http/router.go`, `internal/http/auth.go`, all POST handlers/templates
- **Dependencies:** none
- **Acceptance criteria:** (1) Every state-changing POST requires a CSRF token; absence returns 403. (2) Cookie has `Secure` + `SameSite=Strict`. (3) Cross-site form submit returns 403 in tests.

#### C8. Per-app deploy serialization + graceful shutdown
- **Category:** Concurrency / Reliability
- **Root cause:** Concurrent `go deployWithCommit(id)` from webhook/UI/rollback with no mutex; `main.go:107` uses `srv.Close()` and never cancels monitor/deploy goroutines.
- **Current impact:** Interleaved deploys corrupt remote state; SIGTERM mid-deploy leaves app in `deploying` forever (monitor skips non-online/offline).
- **Risk level:** Critical
- **Complexity:** M
- **Expected benefit:** Deterministic deploys; clean shutdown.
- **Suggested solution:** (a) `sync.Mutex` per `app.ID` (a `map[int64]*sync.Mutex` guarded by an outer mutex) or a DB-backed deploy lease column `apps.deploy_lease_(token, until)`. (b) Root `context.WithCancel` propagated to monitor/deploy; `http.Server.Shutdown(ctx)` on signal. (c) On startup, reset `deploying`/`initializing` rows to a recoverable state OR a re-recover monitor pass.
- **Files:** `internal/app/service.go`, `cmd/dockify/main.go`, `internal/server/monitor.go` (within service.go)
- **Dependencies:** none
- **Acceptance criteria:** (1) Two concurrent deploys of the same app serialize (no `compose down` overlap). (2) `kill -TERM` triggers `Shutdown(ctx)` cleanly; no goroutine leak after 5s. (3) Restart with rows in `deploying` recovers them via monitor or sets them to `failed` with a log entry.

### 🔴 High — strongly recommended before production

#### H1. Fix CPU-usage metric + scheduler inputs
- **Category:** Correctness / Performance
- **Root cause:** `parseCPUUsage` reads `/proc/stat` once → cumulative-since-boot ratio; scheduler scores against it.
- **Current impact:** Wrong "CPU usage" shown; auto-scheduler picks the wrong host.
- **Risk level:** High
- **Complexity:** S
- **Suggested solution:** Sample twice (1s apart), compute `(active2-active1) / (total2-total1) * 100`. Optionally weight the scheduler by `disk_usage` and `apps_count` and prefer servers with fresher `resources_updated_at`.
- **Files:** `internal/server/service.go`, `internal/scheduler/scheduler.go`

#### H2. Schema integrity: ON DELETE CASCADE + indexes + migrations framework
- **Category:** Database / Integrity
- **Root cause:** Missing CASCADE on most FKs → orphans; delete-server doesn't undeploy apps; no indexes; ad-hoc ALTERs with swallowed errors.
- **Expected benefit:** Atomic deletes; faster lookups; predictable upgrades.
- **Complexity:** M
- **Suggested solution:** (a) Add `ON DELETE CASCADE` to all child FKs. (b) Add indexes on `apps.server_id`, `routes.app_id`, `deployments(app_id, created_at DESC)`, `dns_records.name`. (c) Adopt `PRAGMA user_version` + ordered `migrations/*.sql` files; run inside a single transaction. (d) Add a `schema_version` table; reject running with a too-old schema.
- **Files:** `internal/db/db.go`, `internal/db/schema.sql`, new `internal/db/migrations/`

#### H3. Caddy route sanitization + direct Admin API client
- **Category:** Security / Performance
- **Root cause:** `caddy/client.go` uses `docker exec caddy curl` over SSH with unquoted `%s` for IDs and domains; `sanitizeID` permits shell metacharacters.
- **Expected benefit:** Removes injection surface and per-op SSH round-trip.
- **Complexity:** M
- **Suggested solution:** Replace `sanitizeID` with `[A-Za-z0-9-_]` whitelist. Long-term: forward Caddy admin API port over SSH and call it directly via `net/http`.
- **Files:** `internal/caddy/client.go`

#### H4. CSRF + destruct-action confirmation for `import replace`/`RunUpdate`
- **Category:** Security
- **Root cause:** Resulting from C7's defense; even with CSRF, these are footguns.
- **Suggested solution:** Require a typed confirmation token (user types "replace" / "update") plus a dry-run preview for replace import.
- **Files:** `internal/backup/handler.go`, `internal/backup/backup.go`, `internal/settings/handler.go`
- **Dependencies:** C7

#### H5. Backup integrity: per-document MAC + atomic transactions + multi-domain routes
- **Category:** Security / Integrity
- **Root cause:** Only per-field GCM; no MAC over the whole YAML; no transaction; import loses extra routes + DNS.
- **Complexity:** M
- **Suggested solution:** (a) HMAC+encrypt the whole export YAML as the root object; verify on import. (b) Wrap the import in a SQL transaction. (c) Add `Routes` / `DNSSync` to the export record; import reconstructs them.
- **Files:** `internal/backup/backup.go`, `internal/backup/handler.go`

#### H6. Background goroutine concurrency control + reaper
- **Category:** Reliability / Performance
- **Suggested solution:** (a) Bounded `errgroup`/semaphore for monitor refreshes and webhook deploys. (b) Session reaper goroutine runs every minute deleting expired entries. (c) Stuck-init recovery: monitor re-tests `initializing`/`error` rows after N ticks.
- **Files:** `internal/server/service.go`, `internal/http/auth.go`, `internal/app/service.go`

#### H7. CI on push/PR with quality gates
- **Category:** DevOps
- **Suggested solution:** Add `on: [push, pull_request]`; run `go vet`, `go test -race`, `golangci-lint`, `govulncheck`, `scripts/smoke-test.sh`. Add `workflow_dispatch` and a `concurrency:` group.
- **Files:** `.github/workflows/build.yml`, new `.golangci.yml`, `govulncheck` step.
- **Acceptance criteria:** (1) PR to `main` fails CI on `go vet`/`test` failure. (2) `-race` runs. (3) Dependabot opens PRs for action SHA bumps.

#### H8. Defined destructive actions: scoped cleanups; undeploy apps on server delete
- **Category:** Architecture / Correctness
- **Suggested solution:** (a) `server.Delete` lists apps on that server, calls `Undeploy` (best-effort), removes routes/dns_records, removes `<id>.pem`. (b) Replace `image prune -af` with `docker compose -f ... down --rmi all --volumes` scoped to this app. (c) `docker builder prune` becomes per-app or opt-in.
- **Files:** `internal/server/handler.go`, `internal/app/service.go`

#### H9. Backup/Restore confirmation token + audit log
- **Category:** Security / Operability
- **Suggested solution:** (a) Replace mode requires a typed confirmation (`mode=replace` + `confirm=REPLACE`). (b) Add a `backup_log` table (action, actor, mode, ts, exported_count, imported_count). (c) Settings/update visited.
- **Files:** `internal/backup/`, `internal/db/schema.sql`

#### H10. Secrets at-rest encryption + key-file hygiene
- **Category:** Security
- **Suggested solution:** (a) Per-instance root key (env `DOCKIFY_MASTER_KEY` or generated at first boot and stored with 0600). Encrypt `app_secrets.value` and `apps.auth_pass` columns with AES-256-GCM at write. (b) Reject plaintext key exports (refuse if `passphrase=="" && data has secrets`). (c) Replace `servers.ssh_key="pending"` sentinel with a nullable column or a status enum.
- **Files:** `internal/db/schema.sql`, `internal/app/service.go`, `internal/backup/backup.go`

### 🟠 Medium

#### M1. Auth hardening (constant-time, rate-limit, structured logging)
- **Suggested solution:** Use `subtle.ConstantTimeCompare` for password; `golang.org/x/time/rate` token-bucket per IP+user; log failed + successful logins with IP/UA.
- **Files:** `internal/http/auth.go`

#### M2. Request body size limits everywhere
- `http.MaxBytesReader` on webhooks, backups, cloudflare responses, log streaming.

#### M3. Fix template + UI bugs (enc scope, meta-refresh, label-wrap, var(--muted), error.html scripts)
- See §6.

#### M4. Structure handler files (`api_handler.go` / `web_handler.go`) + `RouterDeps` struct
- **Complexity:** M
- **Files:** `internal/server/`, `internal/app/`, `internal/http/router.go`

#### M5. Structured logging with request IDs
- Adopt `log/slog`.

#### M6. Paginate/dashboard counts → SQL

#### M7. Realtime log streaming (SSE/WebSocket) replacing HTMX tail buttons

#### M8. Accessibility pass — `aria-*`, `@media (prefers-reduced-motion)`, table `scope`/`caption`, `role="alert"`/`role="status"`, full keyboard upload

#### M9. Caddyfile hardening — HSTS, X-Content-Type-Options, Referrer-Policy, CSP, `encode gzip`, `/config` volume

#### M10. Dedup console JS (server + app), update scripts `update.sh` ≈ `update-latest.sh`

#### M11. Dev mock self-test guard — refuse to start with `AdminPass==""` and `DevMock=true`

#### M12. Session store migration to DB token table with `Secure`/`SameSite=Strict`/`__Host-`

### 🟡 Low

- L1. `.env.example` completeness; `.gitignore` add `build-errors.log`; `prompts/` decision.
- L2. `.dockerignore` cleanup; Dockerfile `HEALTHCHECK` + OCI labels + Alpine bump + digest pinning.
- L3. Dependabot for actions; SHA-pin actions.
- L4. Remove `var _ = time.Now`, dead `.console-container` CSS, dead if/else in `backup/handler.go:24-29`.
- L5. `RenderFunc` import unification; `recordDeployment` status constants.
- L6. `smoke-test.sh` port range + response body assertions.
- L7. Replace `location.reload(true)` (`about.html:101`) with `location.reload()`.
- L8. `release.sh` note GNU `sort -V` dependency or migrate to a script.
- L9. Docs: remove `dockify.amg.id` from shipped docs; ADRs dates/status; ADR-007 "Pico CSS" correction.
- L10. Time-rendering unification (server-side `relativeTime` only) — or all client-side `<time>`.
- L11. Caddy bcrypt cost 14 → 10 (or async-rotate).
- L12. Sanitize server-detail SSH key display (mask with fingerprint + reveal action).
- L13. `confirm()` JS injection: use `data-confirm-name` attribute and `dataset`.
- L14. Login CSS palette unification; theme-toggle icon parity.

---

### Acceptance criteria (cross-cutting, applies to all security items)
- `gosec`, `gosec G204`, `govulncheck` clean.
- A new `security.test.go` covers shell-injection regression, path traversal, HMAC verification, CSRF token absence, WS origin check, host-key mismatch, secret-on-disk permissions.
- A new `ssh_test.go` (race-clean via `-race`) covers `ExecPTY` and `Shell` channel-close semantics.
- A new `backup_test.go` covers encrypt/decrypt round-trip, partial-import transaction rollback, multi-domain route preservation, replace-mode confirmation.
- `go test -race ./...` passes with no races.
- `scripts/smoke-test.sh` asserts shell-escape attempts return 400/403.

---

### Sequencing (1–2 months)

| Week | Work |
|---|---|
| 1 | C1 (SFTP), C2 (host-key), C3 (webhook auth), C4 (WS origin), C5 (path/0600) — security close-out |
| 2 | C6 (self-update), C7 (CSRF), C8 (deploy mutex/graceful shutdown) |
| 3 | H1 (CPU metric), H2 (schema), H3 (Caddy), H4 (confirm tokens), H5 (backup integrity) |
| 4 | H6 (goroutine hygiene), H7 (CI), H8 (scoped cleanups), H9 (audit log), H10 (secrets at rest) |
| 5 | M1–M5 (auth, body limits, UI bugs, handler files, logging) |
| 6 | M6–M10 (counts, realtime logs, a11y, Caddyfile, dedup) |
| 7 | M11–M12 + L1–L14 |
| 8 | Final pen-test pass, security.test.go + ssh_test.go + backup_test.go shipped, CI gates green, `-race` clean. Tag `v1.0.0-rc1`. |

---

---

## Appendix A — Truncation Recovery Findings

> Errata / supplementary findings recovered from a truncated backend audit pass (`internal/app/handler.go`, `internal/scheduler/scheduler.go`, `internal/backup/backup.go`) that did not surface in the original compile of this document. Listed here for completeness, with the same severity tagging used in §5–§15. Each maps to a section above; include in the same roadmap priority buckets.

### Logic bugs / edge cases (→ §5, §15)

- **A1 — `app/handler.go:470-478 AppAddForm`** validates only `app.Name == "" || app.Compose == ""` but the rendered error message says *"name, domain, port, and either compose or image are required"* — **`domain` and `port` are not actually validated**. Misleading error → operators save invalid apps that fail at deploy. Category: Logic/UX. Risk: Medium. Complexity: S. Fix: validate `domain`/`port` (or align the message to match what's actually checked).
- **A2 — `app/handler.go:588-591 AppEditForm`** uses `serverID == 0 → serverID = app.ServerID`, which **prevents the operator from clearing the chosen server** via the edit form (shipping "Auto-select" is impossible after a server is bound — the form silently re-uses the previous server). Category: UX limitation. Risk: Low. Complexity: S. Fix: introduce an explicit "Clear server" action or sentinel distinct from "auto".
- **A3 — `app/handler.go:693-703 AppEditForm` route diffing** computes `removedDomains` (an unused intermediate slice) and runs a redundant second pass over `oldRoutes` to delete routes. The two passes do the same work; `removedDomains` is dead and could be dropped in favor of a single loop. Category: Code quality. Risk: Low. Complexity: S.
- **A4 — `app/handler.go:820-830 saveFormSecrets` / `:847-861 saveFormFiles`** rely on the **parallel slice ordering** of `r.Form["secret_key"]` vs `r.Form["secret_val"]` (and `file_path` / `file_content`). Go's `r.PostForm` preserves order per key — but the contract is implicit and brittle if anyone renames, reorders, or splits one of the slice inputs. Recommend paired form names like `secret[0].key` / `secret[0].val`. Category: Code quality / maintainability. Risk: Low. Complexity: S.
- **A5 — `app/handler.go:466 / :629`** accept `auth_pass` from the form and store it plaintext forever in `apps.auth_pass`. Caddy only needs the plaintext at deploy time (to bcrypt its own basic-auth entry) — Dockify **could store a hash and send the plaintext to Caddy only during `AddRouteWithAuth`**, then discard it. Mitigation option missing from §7 / §15. Category: Security hardening. Risk: Medium. Complexity: S. Add to H10.

### Performance / correctness — scheduler inputs (→ §8, §15 H1)

- **A6 — `scheduler/scheduler.go:32`** scoring ignores **disk usage** and **running-apps count**: a 99 %-full disk server with idle CPU/RAM is preferred; a server with 100 running apps scores same as one with 0 apps at the same CPU/RAM percentage.
- **A7 — `scheduler/scheduler.go:19` `ListOnline()`** does not consider `resources_updated_at` freshness — a server whose metrics refreshed hours ago still participates and may be picked over a fresher one. Recommend weighting by `now - resources_updated_at` or excluding anything older than `2 × tickInterval`.
- **A8 — `scheduler/scheduler.go` has no deterministic tie-breaking** — equal scores resolve by `servers[i]` index order, which itself depends on a `SELECT` without `ORDER BY`, so identical workloads distribute non-reproducibly. Recommend tie-break by `ID ASC` (or `created_at DESC` newest first) for reproducible deploys.

A6–A8 bundle into H1 in the roadmap; expand H1's solution to enumerate disk-usage factor + apps-count factor + freshness weight + deterministic tie-break.

### Crypto maturity — backup (→ §7, §13, §15 H5)

- **A9 — `backup/backup.go:98`** uses prefix `"enc:"` but stores **no version byte inside the encrypted blob**. If `pbkdf2` iteration count, key length, nonce size, or AEAD algorithm ever change, **old exports become undetectable** and decrypt silently fails (or worse, decrypts with wrong params). Recommend: prepend a 1-byte version into the `raw` slice before encryption and validate on decrypt.
- **A10 — `backup/backup.go:109`** uses magic number `28` (`16 salt + 12 nonce`) as the minimum length. The **real minimum ciphertext is `28 + 16 (GCM authentication tag) = 44 bytes`**; a 28-byte `raw` would pass this length check and then fail at `aead.Open` with an opaque "cipher: message authentication failed" error, hiding the actual cause. Recommend: `const minLen = len(salt) + aead.NonceSize() + aead.Overhead()` (which equals 44) and a wrapping error that names the length failure.
- **A11 — `backup/backup.go:133-145 validateDecrypt`** catches crypto failures but returns a generic "wrong passphrase or corrupted data" message, **swallowing the underlying error**. Hard to distinguish a malformed `enc:` prefix from a wrong passphrase to a GCM-tag mismatch. Recommend wrapping the underlying error (e.g. `fmt.Errorf("decryption failed: %s failed at field %q: %w", aeadName, fieldName, err)`) and surfacing it in the import log while still keeping the user-friendly message in the UI.

### Backup import edge cases (→ §7, §15 H5)

- **A12 — `backup/backup.go:169`** — a missing `<id>.pem` file (`os.ReadFile` fails) **fails the entire export** for all apps/servers. One corrupt server row blocks the whole backup. Recommend: skip the server with a log line and continue, surfacing a "skipped N servers" stderr note.
- **A13 — `backup/backup.go:347`** — imported servers are inserted with status `pending` and `serverSvc.Update` does NOT trigger `TestConnection` / `InitWorker`. The operator must visit each server detail page and click "Initialize Worker" manually. Either document this loudly on the Import page, or have the Import handler schedule `serverSvc.InitWorker` in a background goroutine for each new server.
- **A14 — `backup/backup.go:363-367`** — when the server name from the export isn't in the resolved `serverIDs` map, the matching app is **silently skipped** with only a `log.Printf`. The function returns success and the caller can't see a "skipped count". Recommend: collect skipped apps into the response/`backup_log` table and surface "Imported N servers, M apps, skipped K apps (server not found)" to the user.
- **A15 — `backup/backup.go:392`** — imported apps are inserted with status `created` and `appSvc.Create` does NOT trigger a deploy. The operator must manually `Redeploy` each restored app — a multi-step recovery that should be documented or batched ("Deploy all imported apps" button).

### Master roadmap integration

Fold A1–A15 into existing roadmap items (no new top-level tickets needed unless explicitly noted):

| Finding | Roadmap target |
|---|---|
| A1 (misleading error) | §15 Low (L-bucket): expand "UI bugs" pass |
| A2 (server clear) | §15 Medium M-bucket UX |
| A3 (dead `removedDomains`) | L4 (dead code sweep) |
| A4 (parallel slice forms) | M4 (handler restructuring) |
| A5 (AuthPass plaintext + hash option) | H10 (secrets at rest) — add "use reversible hash + send-to-Caddy-only" as alternative to full encryption |
| A6–A8 (scheduler inputs) | H1 — expand to enumerate 4 improvements (disk, app-count, freshness, tie-break) |
| A9–A11 (crypto maturity) | H5 (backup integrity) — add "version byte in ciphertext + min-len with Overhead() + wrapped errors" |
| A12 (export all-fail) | H5 |
| A13–A15 (import UX) | H5 / new L-bucket entry "Import UX: auto-init servers, surface skipped, document created-status redeploy" |

### Verification audit (this Appendix)

This appendix was produced by re-reading `/Users/indra/.local/share/opencode/tool-output/tool_f4ce9ea56001q849hc0Ee3S45q` (540 lines, the full un-truncated backend audit) and cross-checking each finding's presence in AUDIT.md via `grep`. All 15 A-num findings above are **net-new** vs the pre-appendix document; the rest of the truncated content was already represented in §§3–14.

---

*End of audit. No code was modified. This is a review plan, not an implementation.*