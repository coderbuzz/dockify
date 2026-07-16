# Dockify Audit Hardening — Waves 0-1 (P0 + P1)

## Context

Two independent engineering audits (`audits/DOCKIFY-AUDIT-GLM-5.2.md` and `audits/DOCKIFY-AUDIT-GPT-5.6-SOL.md`) reviewed the Dockify codebase against a production-readiness bar. The GPT audit (§21-22) explicitly reconciles with the GLM audit, applies the correct **trusted single-admin trust model**, and **downgrades** several GLM "Critical" findings (shell injection from admin fields, webhook disable=unsigned, DB sessions, whole-document backup MAC, worker agent) to Medium or rejected. Both audits **agree** on the four P0 blockers and the P1 lifecycle defects.

The GPT audit's **Wave 0 + Wave 1** roadmap is the canonical remediation sequence because it is dependency-aware and trust-model-correct. This plan implements **Waves 0-1** (DCK-001 through DCK-017, excluding DCK-012/DCK-016/DCK-018 which are Wave 2+). Implementer follows the ordered steps below; no design decisions remain.

### Trust model (load-bearing)

The single administrator is **trusted** and already has root-capable SSH/console access. Admin-supplied compose, domains, paths, and file contents are **correctness/containment inputs**, not hostile multi-tenant input. Containment fixes (SFTP, path validation, deterministic compose) prevent **operator mistakes** from escaping the app directory — they are not RCE defense against an attacker who already has root. This matters: don't over-engineer to a multi-tenant bar.

### Findings implemented (GPT IDs)

**P0 (Wave 0):** DCK-001 (fail-closed unauth bind), DCK-002 (SSH host-key TOFU), DCK-003 (bounded ingress/HTTP timeouts), DCK-004 (writable key storage)
**P1 (Wave 1):** DCK-005 (CSRF + cookie + WS origin), DCK-006 (per-app deploy serialization), DCK-007 (worker migration cleanup), DCK-008 (idempotent undeploy + DNS cleanup), DCK-010 (Caddy basic-auth JSON fix), DCK-011 (phase-aware status), DCK-013 (background lifecycle bounds + graceful shutdown), DCK-014 (webhook fail-closed), DCK-015 (deterministic compose + SFTP + path validation), DCK-017 (embedded/pinned browser assets + CSP)

### Findings explicitly deferred to Wave 2+ (not in this plan)

- DCK-009 (backup v2 + staged restore) — depends on DCK-012 migrations
- DCK-012 (migration ledger) — Wave 2, prerequisite for backup v2
- DCK-016 (supply-chain signing/SBOM) — Wave 2
- DCK-018 (secrets at-rest encryption) — Wave 2, depends on DCK-004+DCK-012
- DCK-019-023 (metrics, base-path, mobile/a11y, CI scanners, docs) — Wave 3 (P2)
- Caddy direct HTTP client, worker agent, DB sessions, SSH connection pool — strategic/deferred

### Quick wins folded in (no standalone steps, called inline where natural)

- `.env` mode `0644`→`0600` (DCK-018 partial, done in DCK-015 step)
- GitLab token constant-time compare (DCK-014)
- Tag-push rejection in webhook (DCK-014)
- `docker image prune -af` → app-scoped `down --rmi all` (DCK-008)

---

## Approach

### Wave 0 — Safety containment (P0)

Steps 1-8 are **independent** unless noted. Steps 4-5 (CI) should land early but gate nothing in Wave 0.

---

#### Step 1 — DCK-001: Fail-closed on unauthenticated public bind

**Files:** `internal/config/config.go`, `cmd/dockify/main.go`, `internal/config/config_test.go`

1. Add `AllowUnauthenticated` to `Config` struct (`internal/config/config.go:8-21`), loaded from `getEnv("DOCKIFY_ALLOW_UNAUTHENTICATED", "false")` and parsed as bool (reuse the pattern from `DevMock`: `strings.EqualFold(v, "true")`).
2. In `cmd/dockify/main.go` after `cfg := config.Load()` (line 40) and before `srv` setup, add a guard:
   ```go
   if !cfg.AuthEnabled() && !isLoopback(cfg.Host) && !cfg.AllowUnauthenticated {
       log.Fatal("DOCKIFY_ADMIN_PASSWORD is required when binding to a non-loopback address. " +
           "Set DOCKIFY_HOST=127.0.0.1 for local development, or " +
           "DOCKIFY_ALLOW_UNAUTHENTICATED=true to override (not recommended).")
   }
   ```
   Add helper `isLoopback(host string) bool` that checks the host equals `127.0.0.1`, `::1`, `localhost`, or starts with `127.`.
3. When `AllowUnauthenticated` is `true`, log a prominent `WARNING: ...` line (reuse the existing warning style at `main.go:83`).
4. Add table-driven tests in `internal/config/config_test.go` for the matrix: `{loopback, public} × {password set, empty} × {override true, false}` → expect `{ok, fatal}`. Test `isLoopback` directly.

**Edge cases:** `0.0.0.0` is public (not loopback). IPv6 `::` is public. If `AllowUnauthenticated` is set but `DevMock` is also true, still allow (dev mock needs no auth). No migration needed (config-only).

---

#### Step 2 — DCK-002: SSH host-key verification (TOFU enrollment)

**Files:** `internal/ssh/client.go`, `internal/ssh/interface.go`, `internal/ssh/mock.go`, `internal/server/service.go`, `internal/server/handler.go`, `internal/db/schema.sql`, `internal/db/db.go`, `internal/server/model.go`

1. Add `host_key TEXT` column to the `servers` table in `internal/db/schema.sql` (line 7, after `ssh_key TEXT NOT NULL`). Add migration in `internal/db/db.go` (after line 50): `db.Exec("ALTER TABLE servers ADD COLUMN host_key TEXT")`. This is an additive migration — safe to ignore the "column exists" error (matching the existing pattern).
2. Change `ssh.Connect` signature in `internal/ssh/client.go:20` to accept a host-key verifier:
   ```go
   func Connect(host string, port int, user, keyPath, expectedHostKey string) (*Client, error)
   ```
   - If `expectedHostKey` is empty: use `gossh.FixedPublicKey` callback that captures the key on connect (TOFU first-connect). Store it via a return value.
   - If `expectedHostKey` is non-empty: use `gossh.FixedPublicKey` with a parsed key; reject mismatch.
   - Return the captured/verified host key from `Connect` so the caller can persist it.
   - **Delete** `gossh.InsecureIgnoreHostKey()` (line 36).
3. Add `HostKey() string` method to `*Client` returning the base64-encoded public key marshalled via `gossh.MarshalAuthorizedKey` → strip the key-type prefix.
4. Update `internal/ssh/interface.go` `Factory` signature (line 31):
   ```go
   type Factory func(host string, port int, user, keyPath, expectedHostKey string) (Connector, error)
   ```
5. Update `internal/ssh/mock.go` to accept and ignore the `expectedHostKey` parameter.
6. Update `Connect` callsites — use grep: `grep -rn 'ssh.Connect\|connFactory(' --include='*.go' internal/ cmd/`:
   - `internal/app/service.go:155` (`deployWithCommit`): `s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey, svr.HostKey)`
   - `internal/app/service.go:289` (`FetchLogs`): same pattern.
   - `internal/app/service.go:319` (`Undeploy`): same.
   - `internal/app/service.go:374` (`CleanupFromServer`): same.
   - `internal/server/service.go:179` (`RefreshResources`): same.
   - `internal/server/service.go` `TestConnection` / `InitWorker` (grep for exact lines): pass `svr.HostKey`; on first successful connect where `svr.HostKey == ""`, persist the captured key via `repo.UpdateHostKey(id, key)`.
7. Add `UpdateHostKey(id int64, hostKey string) error` to `internal/server/repository.go` (follow the `Update` pattern).
8. Add `HostKey string` field to the `Server` struct in `internal/server/model.go` (or wherever `Server` is defined — grep `type Server struct`).
9. Add `ScanFor` to the repository's `Get`/`List` queries to populate `host_key` from the new column (grep `SELECT` in `internal/server/repository.go`).
10. Update `internal/ssh/client.go` `RealFactory()` (line 149) to pass through the new param.

**Transition for existing servers:** servers with empty `host_key` → first connect captures and persists the key (TOFU). No migration data needed. The UI does not need a "confirm fingerprint" step for this wave (single-admin trusted model — the key is captured on first connect after upgrade).

**Test:** New `internal/ssh/hostkey_test.go` — but since SSH tests need a real SSH server, instead test the `HostKeyCallback` logic via a unit test on the callback function: feed it an expected key and a mismatched key, assert rejection. Use `ssh.ParsePublicKey` to construct test keys.

---

#### Step 3 — DCK-003: Bounded ingress + HTTP server timeouts

**Files:** `cmd/dockify/main.go`, `internal/webhook/handler.go`, `internal/backup/handler.go`, `internal/app/service.go`

1. In `cmd/dockify/main.go` (line 88-91, where `srv` is configured), replace the bare `&http.Server{Addr: ...}` with:
   ```go
   srv := &http.Server{
       Addr:              cfg.Addr(),
       Handler:           router,
       ReadHeaderTimeout: 10 * time.Second,
       ReadTimeout:       30 * time.Second,
       WriteTimeout:      30 * time.Second,
       IdleTimeout:       120 * time.Second,
   }
   ```
   **Critical:** `WriteTimeout` must not break WebSockets. The chi router's WS upgrade happens inside the handler; once upgraded, the connection hijacks from the server, so `WriteTimeout` no longer applies. This is safe — verified by gorilla/websocket behavior (Upgrade hijacks the connection).
   - Add `"time"` import to main.go if not present.
2. In `internal/webhook/handler.go` line 35 and line 78, replace `io.ReadAll(r.Body)` with a bounded read:
   ```go
   r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB webhook payload
   body, err := io.ReadAll(r.Body)
   ```
   Apply to both `GitHub` (line 35) and `GitLab` (line 78) handlers.
3. In `internal/backup/handler.go`, find the import-upload handler (grep `io.ReadAll` in `internal/backup/handler.go`), wrap with `http.MaxBytesReader(w, r.Body, 50<<20)` (50MB — backups can be large). This is **after** authentication (per GPT audit).
4. In `internal/app/service.go` `FetchLogs` (line 297), cap the `tail` parameter: `if tail > 500 { tail = 500 }` before the SSH exec. Add this guard before the `client.Exec` call.

**Test:** Add test in webhook handler test (new file `internal/webhook/handler_test.go`) that a body exceeding 1MB returns 413 or is truncated-error. For server timeouts, a manual smoke test (curl with slow body) confirms the timeout triggers.

---

#### Step 4 — DCK-004: Writable key storage in Docker Compose

**Files:** `docker-compose.yml`, `internal/server/handler.go`

1. In `docker-compose.yml` (line 10-11), **remove** the `${HOME}/.ssh:/home/dockify/.ssh:ro` volume mount (line 11).
2. Change `DOCKIFY_SSH_KEY_DIR` env (line 16) from `/home/dockify/.ssh` to `/var/lib/dockify/keys`.
   The `dockify_data` named volume (line 10) already mounts `/var/lib/dockify`, so the keys directory at `/var/lib/dockify/keys` is writable. The container's `Dockerfile` already creates this path; `config.go:39` `os.MkdirAll(cfg.SSHKeyDir, 0700)` ensures it exists.
3. In `internal/server/handler.go` `Create` (line 66-69), if `saveKeyFile` fails, **delete the server row** to avoid leaving a `pending` orphan:
   ```go
   path, err := saveKeyFile(h.sshKeyDir, server.ID, input.SSHKey)
   if err != nil {
       h.service.Delete(server.ID)
       JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
       return
   }
   ```
   Add `Delete` to the `server.Service` if not already exported (grep `func (s \*Service) Delete` in `internal/server/service.go` — it exists at line 103).

---

#### Step 5 — DCK-005 (partial): CI regression harness foundation

**Files:** `.github/workflows/ci.yml` (new), `.golangci.yml` (new — minimal)

1. Create `.github/workflows/ci.yml`:
   ```yaml
   name: CI
   on:
     push:
       branches: [main]
     pull_request:
       branches: [main]
   concurrency:
     group: ci-${{ github.ref }}
     cancel-in-progress: true
   jobs:
     test:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-go@v5
           with:
             go-version: '1.25'
         - run: go vet ./...
         - run: go test -race ./...
         - run: go build ./...
   ```
   Do NOT add golangci-lint/govulncheck yet (requires `.golangci.yml` config the team hasn't agreed on — deferred to DCK-022 Wave 3). Keep this minimal: vet + race + build.
2. Run `gofmt -w` on all Go files as part of this step (GPT audit found 13 unformatted files). Add a format check to the workflow:
   ```yaml
         - run: |
             if [ -n "$(gofmt -l .)" ]; then
               echo "gofmt issues found"; gofmt -d .; exit 1
             fi
   ```

**This is the first step that touches CI.** It should be committed on main (or the feature branch) before Wave 0 code changes land, so regressions are caught.

---

### Wave 1 — Deterministic lifecycle + browser boundary (P1)

Steps 6-16 depend on the operation coordinator (Step 6) landing first. After that, steps 7-11 can proceed in parallel; steps 10 and 11 are coupled.

---

#### Step 6 — DCK-006: Per-app deploy serialization (operation coordinator)

**Files:** `internal/app/service.go`

1. Add a per-app mutex map to `Service` struct (line 24-30):
   ```go
   type Service struct {
       // ... existing fields ...
       muMap   sync.Map // map[int64]*sync.Mutex per app ID
   }
   ```
   Import `"sync"`.
2. Add a method to get/create a per-app mutex:
   ```go
   func (s *Service) appMutex(id int64) *sync.Mutex {
       m, _ := s.muMap.LoadOrStore(id, &sync.Mutex{})
       return m.(*sync.Mutex)
   }
   ```
3. In `deployWithCommit` (line 139), acquire the mutex at the top, hold for the entire deploy:
   ```go
   func (s *Service) deployWithCommit(id int64, commitSHA string, removedDomains ...string) {
       mu := s.appMutex(id)
       mu.Lock()
       defer mu.Unlock()
       // ... existing body ...
   }
   ```
4. Any other mutating lifecycle paths must also acquire the same mutex. Apply to:
   - `Undeploy` (line 304): `mu := s.appMutex(id); mu.Lock(); defer mu.Unlock()` at the top.
   - `CleanupFromServer` (line 361): same.
   - `Stop` and `Start` (grep `func (s \*Service) Stop` and `func (s \*Service) Start` in `internal/app/service.go`): same.
5. Bounded concurrency for webhook deploys: in `DeployByGit` (line 133-136), the per-app mutex already serializes same-app deploys. Different apps can deploy in parallel (acceptable — they hit different remote dirs). No global semaphore needed (GPT audit does not require one for Wave 1).

**This is the foundation** — all P1 lifecycle steps (7, 8, 10, 11, 13) build on this serialization.

---

#### Step 7 — DCK-007: Staged worker migration (clean old placement after new deploy)

**Files:** `internal/app/handler.go`, `internal/app/service.go`

The current code at `handler.go:661-680` already does `CleanupFromServer` when server changes, but it fires **before** the new deploy completes and does **not** remove the old app folder. Fix:

1. In `handler.go` around line 674, change `go h.service.CleanupFromServer(id, oldServerID)` to store the `oldServerID` for post-deploy cleanup: pass it as a parameter to `deployWithCommit` or store it on the app/service.
   - **Simpler approach:** add `oldServerID` as a new field on a deploy-request struct, OR add an optional parameter to `deployWithCommit`. Since `deployWithCommit` already takes variadic `removedDomains`, add a new optional parameter pattern or a dedicated cleanup method.
   - **Cleanest:** In `handler.go`, after `h.service.Update(app)` succeeds, call `go h.service.DeployWithMigration(id, oldServerID)` instead of the fire-and-forget cleanup.
2. Add `DeployWithMigration(id, oldServerID int64)` to `internal/app/service.go`:
   ```go
   func (s *Service) DeployWithMigration(id, oldServerID int64) {
       s.deployWithCommit(id, "")
       if oldServerID != 0 {
           s.CleanupFromServer(id, oldServerID)
       }
   }
   ```
   This deploys to the new server first, then cleans up the old server. The per-app mutex (Step 6) ensures these are serialized.
3. Change `CleanupFromServer` (line 385-398) to also remove the app folder from the old server (currently it preserves it with a comment). Replace the comment line and add:
   ```go
   client.Exec(fmt.Sprintf("rm -rf %s", remoteDir))
   ```
   after `SaveConfig()`.

**Edge case:** If the new deploy fails, old placement is already cleaned up → app is down. Acceptable for trusted-admin model (the operator will see the failed status and can rollback). Do not over-engineer blue/green here.

---

#### Step 8 — DCK-008: Idempotent undeploy + DNS record cleanup

**Files:** `internal/app/service.go`, `internal/app/repository.go`

1. In `Undeploy` (line 304-355), replace the global `docker image prune -af` and `docker builder prune -af` (lines 337-338) with app-scoped cleanup:
   ```go
   client.Exec(fmt.Sprintf("%s -f %s down --rmi all --volumes 2>&1 || true", dc, composePath))
   ```
   Remove lines 337-338 entirely.
2. After removing Caddy routes (line 340-347), add DNS record cleanup **before** `s.repo.Delete(id)`:
   ```go
   // Delete stored DNS records
   dnsRecords, _ := s.repo.GetDNSRecords(app.ID)
   if s.cf != nil && s.cf.Enabled() {
       for _, rec := range dnsRecords {
           s.cf.DeleteRecord(rec.RecordID) // best-effort
       }
   }
   s.repo.DeleteDNSRecords(app.ID)
   ```
   - Add `GetDNSRecords(appID int64) ([]DNSRecord, error)` to `internal/app/repository.go` if it doesn't exist (grep `GetDNSRecords` — the `DeleteDNSRecords` method exists at repository.go:276, so follow that pattern for the `Get`).
   - Verify `cloudflare.Client` has a `DeleteRecord` method (grep `func.*DeleteRecord` in `internal/cloudflare/client.go`). If it doesn't exist, add one following the `UpdateRecord` pattern.
3. In the `Undeploy` SSH-failure branches (lines 319-326), when the server/SSH is unavailable, **do not silently delete local state**. Instead, return an error explaining the worker is unreachable:
   ```go
   client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey, svr.HostKey)
   if err != nil {
       // Keep local state so operator can retry; return error
       return fmt.Errorf("undeploy: SSH to %s failed (worker may be offline): %w — local state preserved, retry when worker is available", svr.Name, err)
   }
   ```
   Remove the `s.repo.DeleteRoutes` / `s.repo.DeleteDeployments` / `s.repo.Delete` from the SSH-failure branch (lines 320-325). Only the server-not-found branch (lines 310-317) should clean local state, since the server row is gone.

**Edge case:** `DeleteDNSRecords` exists (repository.go:276). Cloudflare `DeleteRecord` — verify it exists; if not, add a minimal method that DELETEs to the record endpoint, following the `UpdateRecord` pattern. If Cloudflare API fails, log warning and continue (best-effort, matching existing pattern).

---

#### Step 9 — DCK-014: Webhook fail-closed + constant-time GitLab token + tag-push rejection

**Files:** `internal/webhook/handler.go`, `internal/settings/settings.go`, `cmd/dockify/main.go`

1. In `webhook/handler.go` `GitHub` (line 57-63): when `secret == ""`, return 503 and do **not** deploy:
   ```go
   secret := h.webhookSecretFn()
   if secret == "" {
       http.Error(w, "webhook secret not configured", http.StatusServiceUnavailable)
       return
   }
   sig := r.Header.Get("X-GitHub-Signature-256")
   if !verifyHMACSHA256(sig, body, secret) {
       http.Error(w, "invalid signature", http.StatusUnauthorized)
       return
   }
   ```
   Remove the `secret != "" &&` guard — verification is **always** required.
2. Same for `GitLab` (line 100-106): return 503 when empty, always verify.
3. Replace GitLab token compare (line 102) with constant-time: `subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1`. Import `"crypto/subtle"`.
4. Reject tag pushes: after computing `branch` from `payload.Ref` (line 53 in GitHub, line 96 in GitLab), add:
   ```go
   if strings.HasPrefix(payload.Ref, "refs/tags/") {
       log.Printf("Webhook: ignoring tag push %s", payload.Ref)
       w.Write([]byte("ignored"))
       return
   }
   ```
5. Fix `GetWebhookSecret` in `settings/settings.go:136-151`: the current behavior regenerates a secret when the row is missing. This is the "disabled regenerates" bug. Change `GetWebhookSecret` to return `("", nil)` (empty string, no error) when the row is missing (meaning "disabled"), **not** regenerate it. Move the auto-generation to a separate `EnsureWebhookSecret()` method called only at startup (grep for where `ensureWebhookSecret` is called — line 129; check if it's called from `NewService`). If startup calls `ensureWebhookSecret`, keep that for first-boot initialization but do **not** re-create after explicit disable.
6. In `cmd/dockify/main.go` line 79, change `webhookSecretFn` to propagate the error:
   ```go
   webhookSecretFn := func() string {
       s, err := settingsSvc.GetWebhookSecret()
       if err != nil {
           log.Printf("webhook secret lookup error: %v", err)
           return ""
       }
       return s
   }
   ```
   Empty string + error both result in 503 (fail-closed).

**Test:** New `internal/webhook/handler_test.go` — test: empty secret → 503, mismatched signature → 401, tag push → "ignored", valid HMAC → "ok". Use `httptest.NewRecorder` with a mock `WebhookService`.

---

#### Step 10 — DCK-010: Fix Caddy basic-auth JSON handler shape

**Files:** `internal/caddy/client.go`

The current code (line 66) emits `{"handler":"basic_auth","username":...,"hash":...}`. The standard Caddy JSON API expects `{"handler":"authentication","providers":{"http_basic":{"hash":...,"realm":"..."}}}` with credentials array. Specifically ([Caddy JSON docs](https://caddyserver.com/docs/json/apps/http/servers/routes/handle/authentication/)):

```json
{
  "handler": "authentication",
  "providers": {
    "http_basic": {
      "hash": "bcrypt",
      "credentials": [{"username": "...", "password": "..."}]
    }
  }
}
```
Wait — the `hash` field in credentials is the **bcrypt hash**, not the algorithm name. The Caddy `http_basic` provider expects `credentials` with `username` and `password` (the bcrypt hash). Verify the exact schema:

The correct Caddy JSON for basic auth is:
```json
{
  "handler": "authentication",
  "providers": {
    "http_basic": {
      "credentials": [
        {"username": "user", "password": "$2a$14$..."}
      ]
    }
  }
}
```

1. Replace the `handle` struct `basic_auth` handler (line 66) in `AddRouteWithAuth`:
   ```go
   route := Route{
       ID: sanitizeID(domain),
       Match: []match{{Host: []string{domain}}},
       Handle: []handle{
           {
               Handler: "authentication",
               Providers: &providers{
                   HTTPBasic: &httpBasic{
                       Credentials: []credential{
                           {Username: user, Password: string(hash)},
                       },
                   },
               },
           },
           {
               Handler:   "reverse_proxy",
               Upstreams: []upstream{{Dial: target}},
           },
       },
   }
   ```
2. Update the `handle` struct (line 24-29) to include the new fields:
   ```go
   type handle struct {
       Handler    string      `json:"handler"`
       Upstreams  []upstream  `json:"upstreams,omitempty"`
       Providers  *providers  `json:"providers,omitempty"`
       // Remove Username and Hash fields — they were for the wrong schema
   }
   type providers struct {
       HTTPBasic *httpBasic `json:"http_basic,omitempty"`
   }
   type httpBasic struct {
       Credentials []credential `json:"credentials"`
   }
   type credential struct {
       Username string `json:"username"`
       Password string `json:"password"`
   }
   ```
3. Remove the old `Username` and `Hash` fields from `handle`.

**Verification:** This requires a real Caddy instance to validate. The implementer should start `caddy:2-alpine` via Docker, POST the route JSON to `/config/apps/http/servers/srv0/routes` on port 2019, and verify a 401 is returned for unauthenticated requests. This is a manual verification step (see Verification section).

---

#### Step 11 — DCK-011 + DCK-015 (coupled): Phase-aware status + deterministic compose + SFTP + path validation

**Files:** `internal/app/service.go`, `internal/app/compose.go`, `internal/ssh/client.go`, `internal/app/handler.go`, `internal/ssh/interface.go`, `internal/ssh/mock.go`

##### 11a — SFTP replaces WriteFile heredoc (DCK-015)

1. Add `golang.org/x/crypto/ssh/sftp` to `go.mod` (it's already available via `golang.org/x/crypto`; the SFTP package is at `golang.org/x/crypto/ssh/sftp` — verify it's in the module: `go list golang.org/x/crypto/ssh/sftp`). If not present in the module cache, run `go get golang.org/x/crypto/ssh/sftp`.
2. Change `WriteFile` in `internal/ssh/client.go:67-74` to use SFTP:
   ```go
   func (c *Client) WriteFile(path, content string, mode os.FileMode) error {
       sc, err := sftp.NewClient(c.conn)
       if err != nil {
           return fmt.Errorf("sftp client: %w", err)
       }
       defer sc.Close()
       dir := filepath.Dir(path)
       if err := sc.MkdirAll(dir); err != nil {
           return fmt.Errorf("sftp mkdir %s: %w", dir, err)
       }
       f, err := sc.Create(path)
       if err != nil {
           return fmt.Errorf("sftp create %s: %w", path, err)
       }
       defer f.Close()
       if _, err := f.Write([]byte(content)); err != nil {
           return fmt.Errorf("sftp write %s: %w", path, err)
       }
       if err := f.Chmod(mode); err != nil {
           return fmt.Errorf("sftp chmod %s: %w", path, err)
       }
       return nil
   }
   ```
   Import `"path/filepath"` and `"github.com/pkg/sftp"` — wait, the correct import is `golang.org/x/crypto/ssh/sftp` if available, otherwise `github.com/pkg/sftp`. **Confirm which is in go.mod** — `golang.org/x/crypto v0.53.0` includes `ssh/sftp`? Actually no: `github.com/pkg/sftp` is the separate module. Run `go list -m github.com/pkg/sftp` to check. If not present, `go get github.com/pkg/sftp`.
   Actually — `golang.org/x/crypto/ssh` does NOT include SFTP; `github.com/pkg/sftp` is the standard. The mock client's `WriteFile` must also be updated (it currently just logs — keep it as a no-op or store in memory for tests).
3. Update `internal/ssh/interface.go` — `WriteFile` signature is unchanged (good).
4. Update `internal/ssh/mock.go` `WriteFile` to store written content in a map for test assertions (currently it appears to be a no-op or log; grep `func.*MockClient.*WriteFile`).

##### 11b — Path validation for files (DCK-015)

1. In `internal/app/handler.go` `SetFile` (grep `func.*SetFile` in handler.go — line ~240), validate the `path` before passing to service:
   ```go
   if strings.Contains(path, "..") || strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\x00\n\r") {
       JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file path"})
       return
   }
   ```
   Or in the web handler's form-save path (`saveFormFiles` — grep `func saveFormFiles` in handler.go), add the same check.
2. In `internal/app/service.go:184` (`deployWithCommit` file write), the `f.Path` is already stored in DB; add the same validation at write time:
   ```go
   if strings.Contains(f.Path, "..") || strings.HasPrefix(f.Path, "/") {
       log.Printf("Warning: skip file %q — invalid path", f.Path)
       continue
   }
   filePath := filepath.Join(remoteDir, f.Path)
   ```
   Use `filepath.Join` instead of `fmt.Sprintf("%s/%s", ...)` (line 184) for correct path joining + containment.

##### 11c — Deterministic compose service name (DCK-015)

1. `getServiceName` in `compose.go:30-36` uses Go map iteration (nondeterministic). Parse YAML and pick the **first** service by map-key sorted alphabetically, OR by the document order (preserved by `yaml.Node`). Since `composeFile.Services` is `map[string]yaml.Node`, iteration is nondeterministic. Change to deterministic: read services as `yaml.Node` and iterate the mapping node's content (which is ordered in YAML):
   ```go
   func getServiceName(compose string) string {
       var doc struct {
           Services yaml.Node `yaml:"services"`
       }
       if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
           return ""
       }
       if doc.Services.Kind != yaml.MappingNode || len(doc.Services.Content) == 0 {
           return ""
       }
       // Content is pairs: [key, value, key, value, ...]
       // First key (index 0) is the first service in document order
       return doc.Services.Content[0].Value
   }
   ```
   Also fix `parseServiceNames` similarly to return sorted names for deterministic output.
2. `ensureDockifyNetwork` in `compose.go:77-113` uses `strings.Contains(compose, "dockify")` — any incidental mention of "dockify" in comments/labels skips network injection. Replace with a structural check: parse the YAML, check if any service is already on the `dockify` network, if not, add it (keep the existing append pattern but check `svc["networks"]` structurally).
   ```go
   func ensureDockifyNetwork(compose string) string {
       var doc map[string]interface{}
       if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
           return compose
       }
       services, ok := doc["services"].(map[string]interface{})
       if !ok { return compose }
       needsNetwork := false
       for _, svcRaw := range services {
           svc, _ := svcRaw.(map[string]interface{})
           if svc == nil { continue }
           nets, _ := svc["networks"].([]interface{})
           hasDockify := false
           for _, n := range nets {
               if s, ok := n.(string); ok && s == "dockify" { hasDockify = true }
           }
           if !hasDockify {
               svc["networks"] = append(nets, "dockify")
               needsNetwork = true
           }
       }
       if !needsNetwork { return compose }
       doc["networks"] = map[string]interface{}{
           "dockify": map[string]interface{}{"external": true},
       }
       out, _ := yaml.Marshal(doc)
       return string(out)
   }
   ```

##### 11d — Phase-aware deploy status (DCK-011)

1. In `deployWithCommit` (line 139-251), classify phases as required vs. optional:
   - **Required (failure → `StatusFailed`):** write compose file, `docker compose pull`, `docker compose up -d`, Caddy route creation (if domain configured).
   - **Optional (failure → warn but continue, record as warning):** secret `.env` write (currently logged warning — keep), config file writes (same), DNS update (Cloudflare).
   - **Current bugs to fix:**
     - Line 177: `.env` write failure is a warning. **Keep as warning** but write `.env` with mode `0600` (not `0644`). Change line 177:
       ```go
       if err := client.WriteFile(envPath, strings.Join(envLines, "\n")+"\n", 0600); err != nil {
       ```
     - Line 200: compose write failure already → `StatusFailed` (keep).
     - Lines 239-246: Caddy route failures are warnings. Change: if **all** routes fail (Caddy unreachable), set deploy to a new `StatusDegraded = "degraded"` status. If at least one route succeeds, mark `running` with warnings. Add `StatusDegraded` to the const block (line 16-22).
     - Cloudflare DNS failures (line 429): keep as warning (DNS is optional/integration).
   - Add `StatusDegraded` constant and ensure the dashboard/status badges handle it (grep `StatusRunning` usages in templates — add `degraded` badge color).

2. After `docker compose up` succeeds but route/DNS fails: if all routes failed → `s.repo.UpdateStatus(id, StatusDegraded)` and `recordDeployment(id, svr.ID, "degraded", ...)`. If routes succeeded → `StatusRunning` (current behavior at line 248) but include warnings in the deployment log.

---

#### Step 12 — DCK-013: Graceful shutdown + background lifecycle bounds

**Files:** `cmd/dockify/main.go`, `internal/server/service.go`, `internal/app/service.go`, `internal/ssh/client.go`

1. In `cmd/dockify/main.go` (line 107), replace `srv.Close()` with graceful shutdown:
   ```go
   <-quit
   log.Println("Shutting down...")
   
   ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
   defer cancel()
   if err := srv.Shutdown(ctx); err != nil {
       log.Printf("HTTP shutdown error: %v", err)
   }
   svc.StopMonitor() // stops the 60s ticker goroutine
   ```
   Import `"context"` (already imported). Add `"time"` if needed.
2. In `internal/server/service.go` `StartMonitor` (line 199-202) and `Monitor.Run` (line 272-295), add a context for cancellation:
   ```go
   func (s *Service) StartMonitor() {
       s.monitorCtx, s.monitorCancel = context.WithCancel(context.Background())
       s.monitor = NewMonitor(s)
       go s.monitor.Run(s.monitorCtx)
   }
   func (s *Service) StopMonitor() {
       if s.monitorCancel != nil {
           s.monitorCancel()
       }
   }
   ```
   Add `monitorCtx context.Context` and `monitorCancel context.CancelFunc` to the `Service` struct.
3. Change `Monitor.Run` (line 272) to accept `ctx context.Context`:
   ```go
   func (m *Monitor) Run(ctx context.Context) {
       ticker := time.NewTicker(60 * time.Second)
       defer ticker.Stop()
       for {
           select {
           case <-ctx.Done():
               return
           case <-ticker.C:
               // ... existing body ...
           }
       }
   }
   ```
4. On startup, reset stuck `deploying`/`initializing` rows to `failed` (grep `UpdateStatus` in `internal/server/repository.go` — add a `ResetStuckStatuses()` method):
   ```go
   func (r *Repository) ResetStuckStatuses() error {
       _, err := r.db.Exec("UPDATE apps SET status = ? WHERE status IN (?, ?)", StatusFailed, StatusDeploying, StatusStopped)
       // servers: reset initializing/pending similarly
       return err
   }
   ```
   Call this from `main.go` after DB open:
   ```go
   appRepo.ResetStuckStatuses()
   ```
5. In `ssh/client.go` `Exec` (line 49-65), add a context parameter for deadline. This is a larger change — the `Connector.Exec` interface (line 24) must change. **Defer the full context-aware Exec to Wave 2** (DCK-013 recommends it but the GPT roadmap places it as "durable fix"; the minimum safe fix is graceful shutdown + monitor cancel). For Wave 1, only add context to `Monitor.Run` and graceful `srv.Shutdown`.

---

#### Step 13 — DCK-005 (full): CSRF token + cookie hardening + WebSocket origin check

**Files:** `internal/http/auth.go`, `internal/http/console.go`, `internal/http/router.go`, all POST form templates

##### 13a — Cookie hardening

1. In `auth.go:46-52` (`setSession`), add `Secure` and `SameSite` attributes:
   ```go
   http.SetCookie(w, &http.Cookie{
       Name:     sessionName,
       Value:     id,
       Path:     "/",
       HttpOnly:  true,
       Secure:    r.TLS != nil, // true when served over HTTPS
       SameSite:  http.SameSiteLaxMode,
       MaxAge:    int(sessionMaxAge.Seconds()),
   })
   ```
   Use `SameSiteLaxMode` (not Strict — allows top-level navigation GETs after login). `Secure` is set when `r.TLS != nil` (auto-detect). If behind a reverse proxy that terminates TLS, `r.TLS` is nil — document this limitation. For the `__Host-` prefix: only use if always HTTPS; skip for now (reverse-proxy complexity).
2. Use `subtle.ConstantTimeCompare` for password check in `auth.go:100`:
   ```go
   if subtle.ConstantTimeCompare([]byte(user), []byte(cfgUser)) == 1 &&
      subtle.ConstantTimeCompare([]byte(pass), []byte(cfgPass)) == 1 {
   ```
   Import `"crypto/subtle"`.

##### 13b — WebSocket origin check

3. In `console.go:18-22`, replace the upgrader:
   ```go
   var upgrader = websocket.Upgrader{
       CheckOrigin:     checkOrigin,
       ReadBufferSize:  4096,
       WriteBufferSize: 4096,
   }
   func checkOrigin(r *http.Request) bool {
       origin := r.Header.Get("Origin")
       if origin == "" {
           return true // non-browser clients have no Origin header
       }
       host := r.Host
       if cfg := os.Getenv("DOCKIFY_HOST"); cfg != "" && cfg != "0.0.0.0" {
           host = cfg // use configured host/port if non-wildcard
       }
       return strings.Contains(origin, host)
   }
   ```
   Import `"strings"` and `"os"` (already imported in console.go). The `checkOrigin` compares the Origin header's host against the request's Host header — a same-origin check. For `DOCKIFY_HOST=0.0.0.0` (wildcard bind), fall back to `r.Host` (the actual Host header of the request).

##### 13c — CSRF token middleware

4. Implement a **double-submit cookie** CSRF middleware (simplest, no server-side state needed beyond the session):
   - In `internal/http/router.go`, add a CSRF middleware:
     ```go
     func CSRFMiddleware(next http.Handler) http.Handler {
         return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
             if isStateChanging(r.Method) { // POST, PUT, PATCH, DELETE
                 cookieToken, err := r.Cookie("dockify_csrf")
                 if err != nil || cookieToken.Value == "" {
                     http.Error(w, "missing CSRF token", http.StatusForbidden)
                     return
                 }
                 headerToken := r.Header.Get("X-CSRF-Token")
                 formToken := r.FormValue("_csrf")
                 token := headerToken
                 if token == "" { token = formToken }
                 if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(cookieToken.Value)) != 1 {
                     http.Error(w, "invalid CSRF token", http.StatusForbidden)
                     return
                 }
             }
             // Set CSRF cookie on safe-method requests (GET/HEAD/OPTIONS)
             if r.Method == "GET" || r.Method == "HEAD" {
                 setCSRFCookie(w, r)
             }
             next.ServeHTTP(w, r)
         })
     }
     ```
   - `setCSRFCookie` generates a 32-byte random hex token, sets it as a cookie (`SameSite=Lax`).
   - `isStateChanging` returns true for POST/PUT/PATCH/DELETE.
   - **Exempt routes:** `/api/webhook/github` and `/api/webhook/gitlab` (they use HMAC/token auth, not cookies — exempting is correct). `/login` is also state-changing but must be accessible before session — exempt `/login` too.
5. Add the middleware in the router: `r.Use(CSRFMiddleware)` in the protected group (line ~63). The public routes (webhook, login, health) are outside the group and not affected.
6. Add `X-CSRF-Token` to the CORS allowed headers in `CORSMiddleware` (line 245): `"Content-Type, Authorization, X-CSRF-Token"`.
7. **Every POST form template** must include a hidden `_csrf` input or the JS must read the cookie and send `X-CSRF-Token` header. For HTMX forms, add a hidden input:
   - In `layout.html`, add a `<script>` that reads `dockify_csrf` cookie and sets an HTMX `hx-headers` with `X-CSRF-Token`. Or add a hidden input to each form template:
     ```html
     <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
     ```
   - Inject `CSRFToken` into every template render call that has POST forms. This is the broadest change — grep `render(w, r,` in `internal/` handlers and add `"CSRFToken": csrfTokenFromCookie(r)` to the data map.
   - **Alternative (simpler for HTMX):** use a JavaScript interceptor that reads the cookie on every request and adds the header. In `layout.html` `{{template "scripts"}}` block (grep the scripts template):
     ```javascript
     document.addEventListener('htmx:configRequest', function(evt) {
         const csrf = getCookie('dockify_csrf');
         if (csrf) evt.detail.headers['X-CSRF-Token'] = csrf;
     });
     ```
     This covers all HTMX POST/AJAX requests. For non-HTMX form POSTs (if any use plain form submit), add `<input type="hidden" name="_csrf">` — but HTMX is the primary form mechanism. **Use the JS interceptor approach** to avoid touching every form template.

---

#### Step 14 — DCK-017: Embedded/pinned browser assets + CSP headers

**Files:** `internal/http/templates/layout.html`, `internal/http/router.go`, `web/static/` (new dir for downloaded assets)

1. Download HTMX and xterm.js assets from the pinned CDN URLs and place them in `web/static/vendor/`:
   - `htmx.min.js` (v2.0.4) from `https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js`
   - `xterm.min.js` (v5.3.0) and `xterm.css` from `https://cdn.jsdelivr.net/npm/@xterm/xterm@5.3.0/`
   - `xterm-addon-fit.min.js` from `https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.8.0/`
   - Compute and record SHA-256 hashes of each file (for future SRI if not self-hosting).
2. In `layout.html` (lines 6-9 — grep `<script src="https://unpkg` and `<script src="https://cdn.jsdelivr`):
   - Replace CDN `<script src="...">` with local paths: `<script src="/static/vendor/htmx.min.js"></script>`
   - Replace CDN `<link rel="stylesheet" href="https://cdn.jsdelivr...xterm.css">` with `<link rel="stylesheet" href="/static/vendor/xterm.css">`
3. The static file server at `router.go:59-60` already serves `/static/*` from `http.Dir("web/static")`. The new vendor files will be served automatically.
4. Add security headers middleware in `router.go`:
   ```go
   func SecurityHeaders(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           w.Header().Set("X-Content-Type-Options", "nosniff")
           w.Header().Set("X-Frame-Options", "DENY")
           w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
           // CSP: allow inline scripts (templates use inline JS) + self
           w.Header().Set("Content-Security-Policy", 
               "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data:;")
           next.ServeHTTP(w, r)
       })
   }
   r.Use(SecurityHeaders) // add before CORS
   ```
   The `'unsafe-inline'` for scripts is needed because templates use inline `<script>` blocks. The `connect-src 'self' ws: wss:` allows WebSocket console connections.

---

### Branch and commit strategy

Work on feature branch `feat/audit-hardening-waves01`. Commit in logical groups matching the steps above (Wave 0 first, then Wave 1). PR at the end with a description mapping each DCK-nn to its commits.

---

## Critical files & anchors

1. **`internal/ssh/client.go:20-74`** — `Connect` (host-key callback), `WriteFile` (heredoc→SFTP). The injection/TOFU root.
2. **`internal/app/service.go:139-356`** — `deployWithCommit` (deploy pipeline, mutex, phase-aware status), `Undeploy` (DNS cleanup, app-scoped prune). Lifecycle correctness root.
3. **`internal/caddy/client.go:24-77`** — `AddRouteWithAuth` (basic-auth JSON shape fix), `sanitizeID` (already safe for IDs; the `handle` struct is the fix target). Route correctness root.
4. **`internal/webhook/handler.go:27-125`** — `GitHub`/`GitLab` (fail-closed, constant-time, tag rejection). Webhook security root.
5. **`cmd/dockify/main.go:82-107`** — unauth guard, HTTP server timeouts, graceful shutdown, stuck-status reset. Entry-point reliability root.

---

## Verification

### Build + static checks

```bash
cd /Users/indra/Dev/github.com/amg-id/dockify
go build ./...
go vet ./...
go test -race ./...
gofmt -l .
```
All must pass with no errors.

### Wave 0 specific checks

1. **Unauth bind (DCK-001):**
   ```bash
   DOCKIFY_HOST=0.0.0.0 DOCKIFY_ADMIN_PASSWORD="" go run ./cmd/dockify serve
   # Expected: fatal exit with actionable error
   DOCKIFY_HOST=127.0.0.1 go run ./cmd/dockify serve
   # Expected: starts successfully (loopback, no password)
   DOCKIFY_HOST=0.0.0.0 DOCKIFY_ALLOW_UNAUTHENTICATED=true go run ./cmd/dockify serve
   # Expected: starts with prominent warning
   ```

2. **SSH host-key (DCK-002):** `grep InsecureIgnoreHostKey internal/ssh/client.go` → returns nothing. Manual: add a server, verify first connect persists host_key in DB; change the server SSH key and re-connect → expected rejection.

3. **Bounded ingress (DCK-003):** `curl -X POST http://localhost:8080/api/webhook/github -H "X-GitHub-Event: push" -d "$(head -c 2MB /dev/zero)"` → 413 or 400 (not OOM). Verify WebSocket console still works after timeout change.

4. **Key storage (DCK-004):** `docker compose config | grep ssh` → no `~/.ssh` mount. `docker compose up`, add a server → key file written to `/var/lib/dockify/keys/<id>.pem` inside container, persists across restart.

### Wave 1 specific checks

5. **Deploy serialization (DCK-006):** With `DOCKIFY_DEV_MOCK=true`, trigger two concurrent deploys of the same app (webhook + manual) → verify `compose down`/`up` do not interleave (check deploy logs for ordered phase output).

6. **Worker migration (DCK-007):** Edit an app, change server → old server's containers stop and folder is removed after new server deploy succeeds.

7. **Undeploy DNS (DCK-008):** Deploy an app with a domain (Cloudflare configured), then undeploy → verify Cloudflare DNS record is deleted (check Cloudflare dashboard or API). If worker offline, verify undeploy returns error and local state is preserved.

8. **Webhook fail-closed (DCK-014):** `DOCKIFY_DEV_MOCK=true`, delete webhook secret via settings, `curl -X POST .../api/webhook/github -H "X-GitHub-Event: push" -d '{}'` → 503. Tag push with valid secret → "ignored".

9. **Caddy basic-auth (DCK-010):** Start `caddy:2-alpine` via Docker, POST the fixed route JSON via Caddy Admin API (port 2019), `curl -I https://domain/` → 401, `curl -u user:pass ...` → 200 proxy. Manual verification against a real Caddy instance.

10. **CSRF (DCK-005):** In browser with dev mock mode, submit a POST form without CSRF cookie → 403. Normal HTMX operations → succeed (JS interceptor sends token). WebSocket console from a different origin → 403 on upgrade.

11. **Browser assets (DCK-017):** Load the UI and verify no external CDN requests (check browser DevTools Network tab — only `/static/vendor/` requests). CSP header present in response headers.

### End-to-end smoke (manual UI flow)

```bash
DOCKIFY_DEV_MOCK=true DOCKIFY_ADMIN_PASSWORD=test123 go run ./cmd/dockify serve
```
Login → add server → deploy app (simple + advanced mode) → view detail (status badge, console toggle) → edit app (change server) → rollback → stop/start → delete → backup export (with passphrase) → backup import (merge mode). All flows should work without browser console errors.

---

## Assumptions & contingencies

1. **`r.TLS` for `Secure` cookie:** If Dockify runs behind a TLS-terminating reverse proxy (Caddy in the compose stack), `r.TLS` is nil and `Secure` won't be set. This is acceptable for Wave 1 — the reverse proxy handles TLS. If the operator needs `Secure` behind a proxy, they can use `X-Forwarded-Proto` detection (deferred to Wave 3, DCK-020 base-path/work). **Fallback:** if testing reveals cookie not set as Secure over HTTPS, add `X-Forwarded-Proto: https` check.

2. **`github.com/pkg/sftp` availability:** The `golang.org/x/crypto` module does not include SFTP directly. If `go get github.com/pkg/sftp` is needed, it's a new dependency — acceptable (single small pure-Go module). If the dependency cannot be resolved, **fallback:** keep WriteFile as a shell command but use `shellescape`-style quoting for the path and base64-encode the content → decode on remote: `echo '<base64>' | base64 -d > 'path'`. This is less clean than SFTP but removes heredoc injection.

3. **Caddy basic-auth schema (DCK-010):** The confidence is "Medium" per the GPT audit — the exact JSON shape must be validated against the pinned Caddy version. If the `authentication`/`http_basic` shape is rejected by Caddy, **fallback:** generate a Caddyfile fragment and use Caddy's config adapter (`caddy adapt`) — simpler authoring. The implementer **must** validate against a real Caddy instance before marking this step done.

4. **CSRF + HTMX:** The JS interceptor approach (`htmx:configRequest` event) covers all HTMX-driven POSTs. If any forms use plain `<form method="POST" action="...">` without HTMX (grep `method="post"` in templates and check for `hx-` attributes), those need a hidden `_csrf` input. **Fallback:** if plain form POSTs are found, add `<input type="hidden" name="_csrf" value="...">` to those specific templates.

5. **Stop/start route behavior:** The GPT audit (§13, open question 2) notes that stop/start route behavior is ambiguous. This plan does **not** change stop/start route semantics (retains current behavior: stop changes container state, routes remain). If the team decides stops should remove routes, that's a separate decision — not in this plan.

6. **`Exec` context/deadline (DCK-013):** Only `Monitor.Run` gets a context in Wave 1. Full context-aware `Exec` (per-command deadline) is deferred to Wave 2. If a hung `docker pull` blocks a deploy goroutine, the per-app mutex will block that app's deploys but won't crash the process. The graceful shutdown timeout (15s) handles SIGTERM.