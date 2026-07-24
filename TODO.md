# Apps page: resource columns, sorting, domain link, IP in group header

## Context

The Apps page (`apps.html` → `apps_list` template) lists apps grouped per server. Today the table columns are `Name · Domain · Port · Status · Git` and is not sortable. The goal is to make it a quick resource-monitoring view: see which apps consume the most CPU/Memory/Disk without opening each app's detail page, and sort any column asc/desc. Concretely:

- Remove the **Git** display column (data stays on the detail page).
- Add **CPU**, **Memory**, **Disk** columns (latest collected snapshot — low effort / low resource; per-second realtime already lives on each app's "Resource Usage" section, no need to duplicate it here).
- Reorder columns professionally (identity → status → resources → access → port) and keep apps **grouped per server** (no flat cross-server table).
- Make the **Domain** value open the domain in a new browser tab when clicked (not navigate within the app).
- Show the server's **IP/host** in each server group header.
- Make **every column sortable, asc and desc**, within each server group.

## Approach

Steps 1–2 are independent (data layer vs. host wiring). Step 3 depends on 1+2. Steps 4–5 depend on 3. Build order: 1 → 2 → 3 → 4 → 5, so `go build` and `TestTemplates` pass after each.

### 1. Batch latest-stats accessor (low resource: 2 queries, not 2N)

The existing `GetStatsOverview(appID)` does 2 queries per app. For a list of N apps that is 2N queries. Add batched accessors so the list uses exactly 2 queries regardless of N. New code (no existing equivalent).

`internal/app/repository.go` — add two methods near `LatestAggregatedStats` (line ~601):

```go
// LatestStatsByApp returns the latest aggregated container stats per app
// (summed across all of an app's containers at the most recent collection tick),
// keyed by app ID. Apps with no collected stats are absent. Two queries total.
func (r *Repository) LatestStatsByApp() (map[int64]*ContainerStats, error) {
	rows, err := r.db.Query(`
		SELECT cs.app_id,
		       SUM(cs.cpu_percent), SUM(cs.mem_usage_bytes), SUM(cs.mem_limit_bytes), SUM(cs.mem_percent),
		       SUM(cs.net_io_rx_bytes), SUM(cs.net_io_tx_bytes), SUM(cs.block_io_read), SUM(cs.block_io_write)
		FROM container_stats cs
		JOIN (
			SELECT app_id, MAX(created_at) AS max_ts FROM container_stats GROUP BY app_id
		) m ON m.app_id = cs.app_id AND m.max_ts = cs.created_at
		GROUP BY cs.app_id
	`)
	if err != nil { return nil, err }
	defer rows.Close()
	out := make(map[int64]*ContainerStats)
	for rows.Next() {
		var appID int64
		var cpu, memPct sql.NullFloat64
		var memUse, memLimit, netRx, netTx, blkR, blkW sql.NullInt64
		if err := rows.Scan(&appID, &cpu, &memUse, &memLimit, &memPct, &netRx, &netTx, &blkR, &blkW); err != nil {
			return nil, err
		}
		out[appID] = &ContainerStats{
			AppID: appID, CPUPercent: cpu.Float64, MemPercent: memPct.Float64,
			MemUsageBytes: memUse.Int64, MemLimitBytes: memLimit.Int64,
			NetIORxBytes: netRx.Int64, NetIOTxBytes: netTx.Int64,
			BlockIORead: blkR.Int64, BlockIOWrite: blkW.Int64,
		}
	}
	return out, rows.Err()
}

// LatestDiskByApp returns the latest disk-usage sample per app, keyed by app ID,
// as bytes. Apps with no disk sample are absent.
func (r *Repository) LatestDiskByApp() (map[int64]int64, error) {
	rows, err := r.db.Query(`
		SELECT d.app_id, d.disk_usage_bytes
		FROM app_disk_stats d
		JOIN (
			SELECT app_id, MAX(created_at) AS max_ts FROM app_disk_stats GROUP BY app_id
		) m ON m.app_id = d.app_id AND m.max_ts = d.created_at
	`)
	if err != nil { return nil, err }
	defer rows.Close()
	out := make(map[int64]int64)
	for rows.Next() {
		var appID int64
		var b sql.NullInt64
		if err := rows.Scan(&appID, &b); err != nil { return nil, err }
		out[appID] = b.Int64
	}
	return out, rows.Err()
}
```

`internal/app/service.go` — add near `GetStatsOverview` (line ~696), mirroring it but batched:

```go
// StatsOverviewByApp returns the latest per-app resource snapshot for all apps
// in two batched queries (container stats + disk), keyed by app ID. Apps with no
// collected stats are absent. Never returns nil (always a non-nil, possibly empty map).
func (s *Service) StatsOverviewByApp() map[int64]*StatsOverview {
	out := make(map[int64]*StatsOverview)
	if stats, err := s.repo.LatestStatsByApp(); err == nil {
		for appID, cs := range stats {
			out[appID] = &StatsOverview{
				CPUPercent: cs.CPUPercent, MemPercent: cs.MemPercent,
				MemUsageBytes: cs.MemUsageBytes, MemLimitBytes: cs.MemLimitBytes,
				NetIORxBytes: cs.NetIORxBytes, NetIOTxBytes: cs.NetIOTxBytes,
				BlockIORead: cs.BlockIORead, BlockIOWrite: cs.BlockIOWrite,
			}
		}
	}
	if disk, err := s.repo.LatestDiskByApp(); err == nil {
		for appID, b := range disk {
			if ov, ok := out[appID]; ok {
				ov.DiskUsageBytes = b
			} else {
				out[appID] = &StatsOverview{DiskUsageBytes: b}
			}
		}
	}
	return out
}
```

Edge handling: empty tables → empty maps, no error; repo errors are swallowed (cells render `-`) so a failing collector never breaks page load. `SUM(...)` may return NULL → scanned via `sql.NullFloat64`/`sql.NullInt64` and defaulted to zero (more robust than existing single-app code which scans directly, but safe here because GROUP BY can aggregate all-NULL columns).

The existing `GetStatsOverview` (used by `AppDetailPage`/`Handler.Stats`/`AppStatsCard`) is left untouched.

### 2. Carry server Host into the group model

Show the server's IP/host in the group header. `ServerGroup` and `ServerInfo` currently carry only ID/Name/Status.

`internal/app/handler.go`:
- `ServerInfo` (line ~434): add `Host string`.
- `ServerGroup` (line ~373): add `Host string`.
- `GroupAppsByServer` (line ~397): when creating a new group set `Host: svrInfo.Host`.

`cmd/dockify/main.go` `serverLister.List` (line ~123): add `Host: s.Host` to the `app.ServerInfo` literal (the adapter has full `server.Server` via `a.svc.List()`).

`internal/http/router.go` dashboard inline build (line ~230): add `Host: sv.Host` for consistency (dashboard does not render it, so purely zero-value-safe).

No other callers of `ServerInfo`/`ServerGroup` exist (grep confirms `GroupAppsByServer` usages: `AppListPage`, dashboard). Default zero-value `Host:""` is harmless where unset.

### 3. AppListPage passes the stats map

`internal/app/handler.go` `AppListPage` (line ~444): build and pass the stats map. Keep `ServerGroups` (still grouped).

```go
apps, err := h.service.List()
if err != nil { apps = nil }
servers, _ := h.serverRepo.List()
groups := GroupAppsByServer(apps, servers)
appStats := h.service.StatsOverviewByApp()

render(w, r, http.StatusOK, "apps.html", map[string]interface{}{
	"Title":        "Apps",
	"Apps":         apps,
	"ServerGroups": groups,
	"AppStats":     appStats,
	"Flash":        r.URL.Query().Get("flash"),
})
```

`AppStats` is always a non-nil map (per step 1), so the template can safely `{{with $st := index $.AppStats .ID}}...{{end}}`.

### 4. Rewrite the `apps_list` template

`internal/http/templates/apps_list.html`. Columns (left-to-right, no Server column since grouping already shows it):

`Name · Status · CPU · Memory · Disk · Domain · Port · (Actions)`

Header (sortable cols get `data-sort`/`data-type`; Actions header is empty, not sortable):

```html
<thead>
  <tr>
    <th data-sort="name" data-type="string">Name</th>
    <th data-sort="status" data-type="string">Status</th>
    <th data-sort="cpu" data-type="number">CPU</th>
    <th data-sort="memory" data-type="number">Memory</th>
    <th data-sort="disk" data-type="number">Disk</th>
    <th data-sort="domain" data-type="string">Domain</th>
    <th data-sort="port" data-type="number">Port</th>
    <th></th>
  </tr>
</thead>
```

Group header (line ~6): append the IP/host after the name, before the badge. Show only when Host is non-empty:

```html
<h3 ...>
  {{if .ServerID}}<a href="servers/{{.ServerID}}" style="text-decoration:none">{{.ServerName}}</a>{{else}}{{.ServerName}}{{end}}
  {{if .Host}}<small style="color:var(--text-dim);font-weight:400;margin-left:0.3em">{{.Host}}</small>{{end}}
  {{if .Status}}<span class="badge badge-{{.Status}}" style="font-size:0.7rem">{{.Status}}</span>{{end}}
  <small style="color:var(--muted);font-weight:400">— {{len .Apps}} app(s)</small>
</h3>
```

Each row (inside `{{range .Apps}}`, `$` is the top-level data so `$.AppStats` works through the nested ranges):

```html
<tr data-name="{{.Name}}" data-domain="{{.Domain}}"
    data-sort-status="{{.Status}}"
    data-cpu="{{with $st := index $.AppStats .ID}}{{printf "%.3f" $st.CPUPercent}}{{end}}"
    data-memory="{{with $st := index $.AppStats .ID}}{{$st.MemUsageBytes}}{{end}}"
    data-disk="{{with $st := index $.AppStats .ID}}{{$st.DiskUsageBytes}}{{end}}"
    data-port="{{.Port}}">
  <td style="white-space:nowrap"><a href="apps/{{.ID}}">{{.Name}}</a></td>
  <td><span class="badge badge-{{.Status}}">{{.Status}}</span></td>
  <td>{{with $st := index $.AppStats .ID}}{{printf "%.1f" (clamp100 $st.CPUPercent)}}%{{else}}-{{end}}</td>
  <td>{{with $st := index $.AppStats .ID}}{{formatBytes $st.MemUsageBytes}}{{else}}-{{end}}</td>
  <td>{{with $st := index $.AppStats .ID}}{{formatBytes $st.DiskUsageBytes}}{{else}}-{{end}}</td>
  <td style="white-space:nowrap">{{if .Domain}}<a href="https://{{.Domain}}" target="_blank" rel="noopener noreferrer">{{.Domain}}</a>{{else}}-{{end}}</td>
  <td>{{.Port}}</td>
  <td style="width:1%;white-space:nowrap">
    <form method="POST" action="apps/{{.ID}}/undeploy" style="display:inline"
          onsubmit="return confirm('{{if eq .Status "draft"}}Delete draft {{.Name}}?{{else}}Undeploy {{.Name}}? This will stop and remove the app.{{end}}')">
      <button type="submit" class="btn btn-ghost btn-red">{{if eq .Status "draft"}}Delete{{else}}Undeploy{{end}}</button>
    </form>
  </td>
</tr>
```

Notes / edge handling:
- **Domain link**: absolute `https://{{.Domain}}` overrides `<base href>` (set in `layout.html:4`); pattern `target="_blank" rel="noopener noreferrer"` mirrors `about.html:38`. Empty domain → `-`. The single `app.Domain` field is used (matches current behavior; multi-domain collapsed display stays on the detail page).
- **Git column**: delete the `<th>Git</th>` and the `<td>{{if .GitRepo}}...{{end}}</td>`. `GitRepo`/`GitBranch` remain in the `App` model and the detail page.
- **stats cells**: missing stats → `{{else}}-`. `clamp100` and `formatBytes` are registered funcs (`templates.go:197,205`; the JS `formatBytes` in `live_charts.html` is a separate client util, irrelevant here).
- Memory/Disk display **absolute bytes** (`MemUsageBytes`/`DiskUsageBytes`), CPU as %. Sorting keys use the same absolute values (`data-memory`=bytes, `data-disk`=bytes, `data-cpu`=%), matching the goal "which app eats the most".
- Keep the existing `data-name`/`data-domain` attributes (the filter script at the bottom still uses them).

### 5. Sortable columns (client-side, low resource: no server calls)

Add sort to the existing `<script>` at the bottom of `apps_list.html`. Sorting is **per table** (each server group's table is sorted independently), consistent with the grouped structure.

Specification (implementer follows exactly — no decisions):
- Each sortable `<th>` with `data-sort`/`data-type` is clickable. A small indicator span (empty initially) holds the arrow: `<span class="sort-arrow" style="font-size:0.8em;opacity:0.6"></span>` appended inside the `<th>` text.
- State per table: `currentSort` = last clicked column key, `currentDir` = 'asc'|'desc'.
- Default direction on first click of a column: **string columns (`name`,`status`,`domain`) → asc**; **number columns (`cpu`,`memory`,`disk`,`port`) → desc**. Clicking the same column again toggles direction. Clicking a different column sets that column with its default direction.
- Comparator: read the relevant `data-*` attr from each `<tr>`. For `number` type parse float. Empty/missing numeric value → treated as missing → **sorted to the bottom regardless of direction** (so non-running/draft apps always sink, leaving resource hogs on top for desc). For `string` type compare case-insensitively via `localeCompare`; empty string also sinks to the bottom.
- Sorting reorders `<tr>` nodes within the clicked table's `<tbody>` (preserving each row's current `display` state set by the filter, since moving nodes keeps inline `style.display`).
- Update arrows: clear all `<th>` arrows in that table, set the active one to `▲` (asc) or `▼` (desc).
- Page-load default order is unchanged: server-rendered name-asc (repo `List()` is `ORDER BY name ASC`). No JS runs until a header is clicked.

Mirror the existing filter script's self-contained IIFE style and reuse selectors. No new endpoints, no htmx polling, no WebSockets.

### Realtime (decision, not a step)

The list shows the latest persisted snapshot (background collector writes `container_stats` every 10s and `app_disk_stats` every 5min — `internal/app/stats.go` `statsLoop`/`collectAppDiskUsage`). Per-second realtime is the existing `ServeLiveAppStats` WebSocket on the app detail "Resource Usage" card (`router.go:141`), intentionally not duplicated here to keep effort and resource use low.

## Critical files & anchors

- `internal/http/templates/apps_list.html` — the table; rewrite thead/body, group header (add `{{.Host}}`), Domain link, remove Git, add sort script.
- `internal/app/repository.go` (`:585-730`) — add `LatestStatsByApp` / `LatestDiskByApp` batch queries near the existing single-app accessors.
- `internal/app/service.go` (`:696`) — add `StatsOverviewByApp`, mirroring `GetStatsOverview`.
- `internal/app/handler.go` — `ServerInfo`(`:434`)+`ServerGroup`(`:373`)+`GroupAppsByServer`(`:397`) add `Host`; `AppListPage`(`:444`) pass `AppStats`.
- `cmd/dockify/main.go` (`:116`) — `serverLister.List` set `Host: s.Host`.

(Also touched: `internal/http/router.go:230` dashboard inline — add `Host: sv.Host`, zero-value-safe.)

## Verification

Working dir: repo root. Dev/mock mode needs no VMs.

1. **Build + templates parse + lint** (after each step):
   ```bash
   go build ./...
   go vet ./...
   go test ./internal/http/... -run TestTemplates
   ```
   `TestTemplatesRender` renders `apps.html` with empty `Apps` → exercises the "No apps" branch (table not reached), so it guards parse safety of the new template.

2. **Batch query smoke (new behavior proof)** — verify the batch SQL returns per-app latest stats without N+1. Quick programmatic check in the mock/empty-data case so that empty tables don't error:
   ```bash
   DOCKIFY_DEV_MOCK=true DOCKIFY_DATA_DIR=./data-tmp go run ./cmd/dockify serve
   ```
   Then open `http://localhost:8080/apps`.

3. **Manual UI (the primary proof)** — `DOCKIFY_DEV_MOCK=true` shows resources once the mock collector seeds `container_stats`/`app_disk_stats`. With mock mode the 10s collector + 5min disk collector populate the two tables; the "Resource Usage" card on a detail page already proves the collectors fire. On the Apps list confirm:
   - Columns render `Name · Status · CPU · Memory · Disk · Domain · Port · (Undeploy)`, no Git column.
   - Each group header shows the server name **and** its IP/host (e.g. `worker-1 1.2.3.4`).
   - A running app's Domain links to `https://<domain>` and opens a **new tab** (`target="_blank"`); empty-domain app shows `-`.
   - CPU shows `xx.x%`, Memory/Disk show human bytes; draft/stopped apps show `-`.
   - Click **CPU** header → group's rows reorder (most CPU on top, `-` apps sink). Click again → asc. Click **Memory** then **Disk** → each sorts independently within its group. Click **Name** → asc A→Z. Arrows appear only on the active column of each table.
   - The existing Name/Domain filter box still hides rows (display:none), and sorting still works after filtering.

4. **Regression** — `apps_detail.html` "Resource Usage" live view unchanged: `go test ./internal/http/... -run TestTemplatesRender` covers render; open any app detail and confirm the live WebSocket + gauges still update.

## Assumptions & contingencies

- **Realtime on the list**: latest persisted snapshot (10s) chosen over live streaming — low effort/low resource, and per-second realtime already exists on the detail page (per the feature's stated intent). If the user later wants near-real on the list, the fallback is a single batched REST endpoint `GET /api/apps/stats/latest` returning `StatsOverviewByApp()` JSON, polled by the existing script every ~15s to refresh only the CPU/Mem/Disk cells (one DB query, no SSH). Not implemented now.
- **Domain scheme**: links use `https://`. Caddy+Cloudflare auto-provisions TLS in this stack, so app domains are HTTPS in practice. If a deployment serves a domain on plain HTTP, the new tab will fail TLS — fallback the user can request is protocol-relative `//{{.Domain}}` (honors the visiting page's scheme).
- **Memory/Disk sort metric**: absolute bytes (usage consumption), not percent-of-limit — directly answers "which app eats the most". CPU is inherently percent. If percent-of-limit is later preferred for Memory/Disk, switch `data-memory`/`data-disk` display+sort to `MemPercent`/a disk percent; data already present in `StatsOverview`.
- **Group sorting scope**: sort applies within each server group (grouped structure chosen by user), so cross-server "fleet-wide #1 hog" ranking is not a single click — it's per group. Flat cross-server sort was explicitly declined.