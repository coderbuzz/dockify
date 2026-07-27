# Execution Plan: Apps Page Resource Columns, Group Header Host & Sortable Columns

## Overview & Objective

Upgrade the Apps page (`apps.html` / `apps_list` template) in **dockify** from a basic table view into a high-density resource monitoring view. 

Key changes:
1. **Server Group Header**: Display server's IP/Host (`{{.Host}}`) alongside Server Name in group headers.
2. **Resource Metrics Columns**: Add **CPU**, **Memory**, and **Disk** snapshot columns. Remove the cluttering **Git** column (which remains available on the detail page).
3. **Professional Column Hierarchy**: Order columns logically: `Name · Status · CPU · Memory · Disk · Domain · Port · Actions`.
4. **External Domain Access**: Make the **Domain** link open in a new browser tab (`target="_blank" rel="noopener noreferrer"` with `https://` prefix).
5. **Client-Side Per-Group Sorting**: Allow sorting by any column (asc / desc) independently within each server group. Smart sorting defaults (strings: asc, numbers: desc; missing values sink to bottom).
6. **Batched Stats Database Accessor**: Use exactly 2 SQL queries total to fetch the latest resource snapshots for all $N$ apps instead of $2N$ queries.

---

## Detailed Step-by-Step Implementation Plan

### Step 1: Batch Stats Repository & Service Accessors

#### 1.1 `internal/app/repository.go`
Add `LatestStatsByApp` and `LatestDiskByApp` to fetch stats for all apps in 2 total queries:

```go
// LatestStatsByApp returns the latest aggregated container stats per app,
// keyed by app ID. Apps with no collected stats are absent.
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
	if err != nil {
		return nil, err
	}
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
			AppID:         appID,
			CPUPercent:    cpu.Float64,
			MemPercent:    memPct.Float64,
			MemUsageBytes: memUse.Int64,
			MemLimitBytes: memLimit.Int64,
			NetIORxBytes:  netRx.Int64,
			NetIOTxBytes:  netTx.Int64,
			BlockIORead:   blkR.Int64,
			BlockIOWrite:  blkW.Int64,
		}
	}
	return out, rows.Err()
}

// LatestDiskByApp returns the latest disk usage sample per app in bytes, keyed by app ID.
func (r *Repository) LatestDiskByApp() (map[int64]int64, error) {
	rows, err := r.db.Query(`
		SELECT d.app_id, MAX(d.disk_usage_bytes)
		FROM app_disk_stats d
		JOIN (
			SELECT app_id, MAX(created_at) AS max_ts FROM app_disk_stats GROUP BY app_id
		) m ON m.app_id = d.app_id AND m.max_ts = d.created_at
		GROUP BY d.app_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]int64)
	for rows.Next() {
		var appID int64
		var b sql.NullInt64
		if err := rows.Scan(&appID, &b); err != nil {
			return nil, err
		}
		out[appID] = b.Int64
	}
	return out, rows.Err()
}
```

#### 1.2 `internal/app/service.go`
Add `StatsOverviewByApp`:

```go
// StatsOverviewByApp returns the latest per-app resource snapshot for all apps
// in two batched queries (container stats + disk), keyed by app ID.
func (s *Service) StatsOverviewByApp() map[int64]*StatsOverview {
	out := make(map[int64]*StatsOverview)
	if stats, err := s.repo.LatestStatsByApp(); err == nil {
		for appID, cs := range stats {
			out[appID] = &StatsOverview{
				CPUPercent:    cs.CPUPercent,
				MemPercent:    cs.MemPercent,
				MemUsageBytes: cs.MemUsageBytes,
				MemLimitBytes: cs.MemLimitBytes,
				NetIORxBytes:  cs.NetIORxBytes,
				NetIOTxBytes:  cs.NetIOTxBytes,
				BlockIORead:   cs.BlockIORead,
				BlockIOWrite:  cs.BlockIOWrite,
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

---

### Step 2: Pass Host in Group Model & Wire Handler

#### 2.1 `internal/app/handler.go`
- Add `Host string` to `ServerInfo` and `ServerGroup` structs.
- Update `GroupAppsByServer` to populate `Host: svrInfo.Host`.
- Update `AppListPage` handler to call `h.service.StatsOverviewByApp()` and pass `"AppStats": appStats` to `apps.html`.

#### 2.2 Server Lister Wiring
- In `cmd/dockify/main.go` (`serverLister.List`), add `Host: s.Host` to `app.ServerInfo`.
- In `internal/http/router.go` dashboard inline group build, add `Host: sv.Host`.

---

### Step 3: Template Refactoring (`internal/http/templates/apps_list.html`)

1. **Header Updates**:
   - Render `.Host` in group header:
     ```html
     {{if .Host}}<small style="color:var(--text-dim);font-weight:400;margin-left:0.3em">{{.Host}}</small>{{end}}
     ```

2. **Table Header & Sorting Markup**:
   - Set table columns: `Name`, `Status`, `CPU`, `Memory`, `Disk`, `Domain`, `Port`, `(Actions)`.
   - Add data attributes for sorting: `data-sort="..."` and `data-type="string|number"`. Add pointer style.

3. **Table Body Rows**:
   - Set `data-*` sorting keys on `<tr>`:
     - `data-name="{{.Name}}"`
     - `data-sort-status="{{.Status}}"`
     - `data-cpu="..."`
     - `data-memory="..."`
     - `data-disk="..."`
     - `data-domain="{{.Domain}}"`
     - `data-port="{{.Port}}"`
   - Format domain as clickable tab:
     ```html
     {{if .Domain}}<a href="https://{{.Domain}}" target="_blank" rel="noopener noreferrer">{{.Domain}}</a>{{else}}-{{end}}
     ```
   - Format CPU, Memory (`formatBytes`), Disk (`formatBytes`), fallback to `-` if stats are absent.

4. **Client-Side Sorting JavaScript**:
   - Attach click listeners to all `th[data-sort]`.
   - Track `currentSort` and `currentDir` (`asc` / `desc`) per table.
   - String default: `asc`, Number default: `desc`.
   - Null / empty values sorted to bottom.
   - Render sort indicators (`▲` / `▼`).
   - Preserve inline filter compatibility (`display: none` stays respected).

---

## Verification Plan

### Automated Checks
```bash
go build ./...
go vet ./...
go test ./internal/http/... -run TestTemplates
```

### Manual & Runtime Verification
1. Run local mock server:
   ```bash
   DOCKIFY_DEV_MOCK=true DOCKIFY_DATA_DIR=./data-tmp go run ./cmd/dockify serve
   ```
2. Open `http://localhost:8080/apps`:
   - Check Server Host IP next to Server Name.
   - Check CPU %, Memory (human readable bytes), Disk (human readable bytes).
   - Click Domain links to verify `https://` and `target="_blank"`.
   - Click headers (CPU, Memory, Disk, Name, Port) to verify asc/desc sorting within groups.
   - Verify filter input box works alongside column sorting.

---

## Direct Instructions Prompt for AI Execution

> **Task**: Implement the refined Apps page resource columns, server host header, domain tab link, and table sorting feature in `dockify`.
>
> **Instructions**:
> 1. Edit `internal/app/repository.go` to implement `LatestStatsByApp()` and `LatestDiskByApp()` batched queries.
> 2. Edit `internal/app/service.go` to add `StatsOverviewByApp()`.
> 3. Update `internal/app/handler.go`, `cmd/dockify/main.go`, and `internal/http/router.go` to include `Host` in `ServerInfo`/`ServerGroup` and pass `AppStats` to `apps_list`.
> 4. Rewrite `internal/http/templates/apps_list.html` with sortable headers, updated columns (`Name`, `Status`, `CPU`, `Memory`, `Disk`, `Domain`, `Port`), server host in header, external domain links, and vanilla JS per-table sort logic.
> 5. Run `go build ./...`, `go vet ./...`, `go test ./...` to verify code compiles and tests pass.
