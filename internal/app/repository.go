package app

import (
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List() ([]App, error) {
	rows, err := r.db.Query(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, compose_mode,
		       memory_limit, cpu_limit, log_max_size, log_max_file,
		       command, ports, ulimits_nofile, created_at, updated_at
		FROM apps ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var gitRepo, gitBranch sql.NullString
		var serverID sql.NullInt64
		if err := rows.Scan(
			&a.ID, &a.Name, &serverID, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.ComposeMode,
			&a.MemoryLimit, &a.CPULimit, &a.LogMaxSize, &a.LogMaxFile,
			&a.Command, &a.Ports, &a.UlimitsNofile, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.ServerID = serverID.Int64
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) Get(id int64) (*App, error) {
	a := &App{}
	var gitRepo, gitBranch sql.NullString
	var serverID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, compose_mode,
		       memory_limit, cpu_limit, log_max_size, log_max_file,
		       command, ports, ulimits_nofile, created_at, updated_at
		FROM apps WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &serverID, &a.Domain, &a.Port, &a.Compose,
		&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.ComposeMode,
		&a.MemoryLimit, &a.CPULimit, &a.LogMaxSize, &a.LogMaxFile,
		&a.Command, &a.Ports, &a.UlimitsNofile, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.ServerID = serverID.Int64
	a.GitRepo = gitRepo.String
	a.GitBranch = gitBranch.String
	return a, nil
}

func (r *Repository) FindAllByGitRepo(repo, branch string) ([]App, error) {
	rows, err := r.db.Query(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, compose_mode,
		       memory_limit, cpu_limit, log_max_size, log_max_file,
		       command, ports, ulimits_nofile, created_at, updated_at
		FROM apps WHERE git_repo = ? AND git_branch = ? AND status != 'draft' ORDER BY id
	`, repo, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var gitRepo, gitBranch sql.NullString
		var serverID sql.NullInt64
		if err := rows.Scan(
			&a.ID, &a.Name, &serverID, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.ComposeMode,
			&a.MemoryLimit, &a.CPULimit, &a.LogMaxSize, &a.LogMaxFile,
			&a.Command, &a.Ports, &a.UlimitsNofile, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.ServerID = serverID.Int64
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) FindByGitRepo(repo, branch string) (*App, error) {
	a := &App{}
	var gitRepo, gitBranch sql.NullString
	var serverID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, compose_mode,
		       memory_limit, cpu_limit, log_max_size, log_max_file,
		       command, created_at, updated_at
		FROM apps WHERE git_repo = ? AND git_branch = ? LIMIT 1
	`, repo, branch).Scan(
		&a.ID, &a.Name, &serverID, &a.Domain, &a.Port, &a.Compose,
		&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.ComposeMode,
		&a.MemoryLimit, &a.CPULimit, &a.LogMaxSize, &a.LogMaxFile,
		&a.Command, &a.Ports, &a.UlimitsNofile, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.ServerID = serverID.Int64
	a.GitRepo = gitRepo.String
	a.GitBranch = gitBranch.String
	return a, nil
}

func (r *Repository) ListByServer(serverID int64) ([]App, error) {
	rows, err := r.db.Query(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, compose_mode,
		       memory_limit, cpu_limit, log_max_size, log_max_file,
		       command, ports, ulimits_nofile, created_at, updated_at
		FROM apps WHERE server_id = ? ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var gitRepo, gitBranch sql.NullString
		var ns sql.NullInt64
		if err := rows.Scan(
			&a.ID, &a.Name, &ns, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.ComposeMode,
			&a.MemoryLimit, &a.CPULimit, &a.LogMaxSize, &a.LogMaxFile,
			&a.Command, &a.Ports, &a.UlimitsNofile, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.ServerID = ns.Int64
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) Create(a *App) error {
	result, err := r.db.Exec(`
		INSERT INTO apps (name, server_id, domain, port, compose, git_repo, git_branch, auth_user, auth_pass, status, compose_mode, memory_limit, cpu_limit, log_max_size, log_max_file, command, ports, ulimits_nofile)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.Name, nullInt64(a.ServerID), a.Domain, a.Port, a.Compose, nullString(a.GitRepo), nullString(a.GitBranch), a.AuthUser, a.AuthPass, defaultStatus(a.Status), a.ComposeMode, a.MemoryLimit, a.CPULimit, a.LogMaxSize, a.LogMaxFile, a.Command, a.Ports, a.UlimitsNofile)
	if err != nil {
		return fmt.Errorf("insert app: %w", err)
	}
	id, _ := result.LastInsertId()
	a.ID = id
	return nil
}

func (r *Repository) Update(a *App) error {
	_, err := r.db.Exec(`
		UPDATE apps SET
			name=?, server_id=?, domain=?, port=?, compose=?,
			git_repo=?, git_branch=?, auth_user=?, auth_pass=?, status=?, compose_mode=?,
			memory_limit=?, cpu_limit=?, log_max_size=?, log_max_file=?,
			command=?, ports=?, ulimits_nofile=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, a.Name, nullInt64(a.ServerID), a.Domain, a.Port, a.Compose,
		nullString(a.GitRepo), nullString(a.GitBranch), a.AuthUser, a.AuthPass, a.Status, a.ComposeMode,
		a.MemoryLimit, a.CPULimit, a.LogMaxSize, a.LogMaxFile,
		a.Command, a.Ports, a.UlimitsNofile, a.ID)
	return err
}

func (r *Repository) UpdateStatus(id int64, status string) error {
	_, err := r.db.Exec(`
		UPDATE apps SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, status, id)
	return err
}

func (r *Repository) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM apps WHERE id = ?", id)
	return err
}

func (r *Repository) AddDeployment(d *Deployment) error {
	result, err := r.db.Exec(`
		INSERT INTO deployments (app_id, server_id, status, log, commit_sha, compose_snapshot)
		VALUES (?, ?, ?, ?, ?, ?)
	`, d.AppID, d.ServerID, d.Status, d.Log, nullString(d.CommitSHA), nullString(d.ComposeSnapshot))
	if err != nil {
		return fmt.Errorf("insert deployment: %w", err)
	}
	id, _ := result.LastInsertId()
	d.ID = id
	return nil
}

func (r *Repository) ListDeployments(appID int64) ([]Deployment, error) {
	rows, err := r.db.Query(`
		SELECT id, app_id, server_id, status, log, commit_sha, compose_snapshot, created_at
		FROM deployments WHERE app_id = ? ORDER BY created_at DESC LIMIT 20
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []Deployment
	for rows.Next() {
		var d Deployment
		var logStr, commitSHA, compSnap sql.NullString
		if err := rows.Scan(
			&d.ID, &d.AppID, &d.ServerID, &d.Status,
			&logStr, &commitSHA, &compSnap, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		d.Log = logStr.String
		d.CommitSHA = commitSHA.String
		d.ComposeSnapshot = compSnap.String
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

func (r *Repository) GetDeployment(id int64) (*Deployment, error) {
	d := &Deployment{}
	var logStr, commitSHA, compSnap sql.NullString
	err := r.db.QueryRow(`
		SELECT id, app_id, server_id, status, log, commit_sha, compose_snapshot, created_at
		FROM deployments WHERE id = ?
	`, id).Scan(
		&d.ID, &d.AppID, &d.ServerID, &d.Status,
		&logStr, &commitSHA, &compSnap, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Log = logStr.String
	d.CommitSHA = commitSHA.String
	d.ComposeSnapshot = compSnap.String
	return d, nil
}

func (r *Repository) GetLastSuccessfulDeployment(appID int64) (*Deployment, error) {
	d := &Deployment{}
	var logStr, commitSHA, compSnap sql.NullString
	err := r.db.QueryRow(`
		SELECT id, app_id, server_id, status, log, commit_sha, compose_snapshot, created_at
		FROM deployments WHERE app_id = ? AND status = 'success'
		ORDER BY created_at DESC LIMIT 1
	`, appID).Scan(
		&d.ID, &d.AppID, &d.ServerID, &d.Status,
		&logStr, &commitSHA, &compSnap, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Log = logStr.String
	d.CommitSHA = commitSHA.String
	d.ComposeSnapshot = compSnap.String
	return d, nil
}

func (r *Repository) DeleteDeployments(appID int64) error {
	_, err := r.db.Exec("DELETE FROM deployments WHERE app_id = ?", appID)
	return err
}

func (r *Repository) DeleteRoutes(appID int64) error {
	_, err := r.db.Exec("DELETE FROM routes WHERE app_id = ?", appID)
	return err
}

func (r *Repository) DeleteDNSRecords(appID int64) error {
	_, err := r.db.Exec("DELETE FROM dns_records WHERE app_id = ?", appID)
	return err
}

func (r *Repository) SaveRoute(route *Route) error {
	result, err := r.db.Exec(`
		INSERT INTO routes (app_id, server_id, domain, target, status)
		VALUES (?, ?, ?, ?, 'active')
	`, route.AppID, route.ServerID, route.Domain, route.Target)
	if err != nil {
		return fmt.Errorf("insert route: %w", err)
	}
	id, _ := result.LastInsertId()
	route.ID = id
	return nil
}

func (r *Repository) UpdateRouteTarget(routeID int64, target string) error {
	_, err := r.db.Exec("UPDATE routes SET target = ? WHERE id = ?", target, routeID)
	return err
}

func (r *Repository) DeleteRouteByDomain(appID int64, domain string) error {
	_, err := r.db.Exec("DELETE FROM routes WHERE app_id = ? AND domain = ?", appID, domain)
	return err
}

func (r *Repository) RemoveRoute(routeID int64) error {
	_, err := r.db.Exec("UPDATE routes SET status='removed' WHERE id=?", routeID)
	return err
}

func (r *Repository) GetRoutes(appID int64) ([]Route, error) {
	rows, err := r.db.Query(`
		SELECT id, app_id, server_id, domain, target, status, created_at
		FROM routes WHERE app_id = ? AND status = 'active'
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []Route
	for rows.Next() {
		var rt Route
		if err := rows.Scan(&rt.ID, &rt.AppID, &rt.ServerID, &rt.Domain, &rt.Target, &rt.Status, &rt.CreatedAt); err != nil {
			return nil, err
		}
		routes = append(routes, rt)
	}
	return routes, rows.Err()
}

type Route struct {
	ID        int64
	AppID     int64
	ServerID  int64
	Domain    string
	Target    string
	Status    string
	CreatedAt string
}

func (r *Repository) SaveDNSRecord(appID, serverID int64, zoneID, recordID, name, recordType, content string, proxied bool) error {
	proxiedInt := 0
	if proxied {
		proxiedInt = 1
	}
	_, err := r.db.Exec(`
		INSERT INTO dns_records (app_id, server_id, zone_id, record_id, name, type, content, proxied)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, appID, serverID, zoneID, recordID, name, recordType, content, proxiedInt)
	return err
}

func (r *Repository) UpdateDNSRecordProxied(recordID string, proxied bool) error {
	proxiedInt := 0
	if proxied {
		proxiedInt = 1
	}
	_, err := r.db.Exec(`UPDATE dns_records SET proxied = ? WHERE record_id = ?`, proxiedInt, recordID)
	return err
}

func (r *Repository) GetDNSRecords(appID int64) ([]DNSRecordInfo, error) {
	rows, err := r.db.Query(`
		SELECT id, app_id, server_id, zone_id, record_id, name, type, content, proxied, created_at
		FROM dns_records WHERE app_id = ?
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DNSRecordInfo
	for rows.Next() {
		var r2 DNSRecordInfo
		if err := rows.Scan(&r2.ID, &r2.AppID, &r2.ServerID, &r2.ZoneID, &r2.RecordID, &r2.Name, &r2.Type, &r2.Content, &r2.Proxied, &r2.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r2)
	}
	return records, rows.Err()
}

func (r *Repository) DeleteDNSRecord(id int64) error {
	_, err := r.db.Exec("DELETE FROM dns_records WHERE id = ?", id)
	return err
}

type DNSRecordInfo struct {
	ID        int64
	AppID     int64
	ServerID  int64
	ZoneID    string
	RecordID  string
	Name      string
	Type      string
	Content   string
	Proxied   int
	CreatedAt string
}

type AppSecret struct {
	ID       int64  `json:"id"`
	AppID    int64  `json:"app_id"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

func (r *Repository) ListSecrets(appID int64) ([]AppSecret, error) {
	rows, err := r.db.Query(`SELECT id, app_id, key, value, is_secret FROM app_secrets WHERE app_id = ? ORDER BY key`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var secrets []AppSecret
	for rows.Next() {
		var s AppSecret
		var isSecret int
		if err := rows.Scan(&s.ID, &s.AppID, &s.Key, &s.Value, &isSecret); err != nil {
			return nil, err
		}
		s.IsSecret = isSecret == 1
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (r *Repository) ListAllSecrets() ([]AppSecret, error) {
	rows, err := r.db.Query(`SELECT id, app_id, key, value, is_secret FROM app_secrets ORDER BY app_id, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var secrets []AppSecret
	for rows.Next() {
		var s AppSecret
		var isSecret int
		if err := rows.Scan(&s.ID, &s.AppID, &s.Key, &s.Value, &isSecret); err != nil {
			return nil, err
		}
		s.IsSecret = isSecret == 1
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (r *Repository) SetSecret(appID int64, key, value string) error {
	_, err := r.db.Exec(`INSERT INTO app_secrets (app_id, key, value) VALUES (?, ?, ?) ON CONFLICT(app_id, key) DO UPDATE SET value = ?`, appID, key, value, value)
	return err
}

func (r *Repository) SetSecretWithType(appID int64, key, value string, isSecret bool) error {
	isSecretInt := 0
	if isSecret {
		isSecretInt = 1
	}
	_, err := r.db.Exec(`INSERT INTO app_secrets (app_id, key, value, is_secret) VALUES (?, ?, ?, ?) ON CONFLICT(app_id, key) DO UPDATE SET value = ?, is_secret = ?`, appID, key, value, isSecretInt, value, isSecretInt)
	return err
}

func (r *Repository) DeleteSecret(appID int64, key string) error {
	_, err := r.db.Exec(`DELETE FROM app_secrets WHERE app_id = ? AND key = ?`, appID, key)
	return err
}

func (r *Repository) DeleteSecrets(appID int64) error {
	_, err := r.db.Exec(`DELETE FROM app_secrets WHERE app_id = ?`, appID)
	return err
}

type AppFile struct {
	ID      int64
	AppID   int64
	Path    string
	Content string
}

func (r *Repository) ListFiles(appID int64) ([]AppFile, error) {
	rows, err := r.db.Query(`SELECT id, app_id, path, content FROM app_files WHERE app_id = ? ORDER BY path`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []AppFile
	for rows.Next() {
		var f AppFile
		if err := rows.Scan(&f.ID, &f.AppID, &f.Path, &f.Content); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *Repository) ListAllFiles() ([]AppFile, error) {
	rows, err := r.db.Query(`SELECT id, app_id, path, content FROM app_files ORDER BY app_id, path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []AppFile
	for rows.Next() {
		var f AppFile
		if err := rows.Scan(&f.ID, &f.AppID, &f.Path, &f.Content); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *Repository) SetFile(appID int64, path, content string) error {
	_, err := r.db.Exec(`INSERT INTO app_files (app_id, path, content) VALUES (?, ?, ?) ON CONFLICT(app_id, path) DO UPDATE SET content = ?`, appID, path, content, content)
	return err
}

func (r *Repository) DeleteFile(appID int64, path string) error {
	_, err := r.db.Exec(`DELETE FROM app_files WHERE app_id = ? AND path = ?`, appID, path)
	return err
}

func defaultStatus(s string) string {
	if s == "" {
		return "created"
	}
	return s
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(i int64) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

func (r *Repository) InsertContainerStats(s *ContainerStats) error {
	_, err := r.db.Exec(`
		INSERT INTO container_stats (app_id, server_id, container_name, cpu_percent, mem_usage_bytes, mem_limit_bytes, mem_percent, net_io_rx_bytes, net_io_tx_bytes, block_io_read, block_io_write)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.AppID, s.ServerID, s.ContainerName, s.CPUPercent, s.MemUsageBytes, s.MemLimitBytes, s.MemPercent, s.NetIORxBytes, s.NetIOTxBytes, s.BlockIORead, s.BlockIOWrite)
	return err
}

func (r *Repository) LatestContainerStats(appID int64) (*ContainerStats, error) {
	s := &ContainerStats{}
	err := r.db.QueryRow(`
		SELECT id, app_id, server_id, container_name, cpu_percent, mem_usage_bytes, mem_limit_bytes, mem_percent, net_io_rx_bytes, net_io_tx_bytes, block_io_read, block_io_write, created_at
		FROM container_stats WHERE app_id = ? ORDER BY created_at DESC LIMIT 1
	`, appID).Scan(&s.ID, &s.AppID, &s.ServerID, &s.ContainerName, &s.CPUPercent, &s.MemUsageBytes, &s.MemLimitBytes, &s.MemPercent, &s.NetIORxBytes, &s.NetIOTxBytes, &s.BlockIORead, &s.BlockIOWrite, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// LatestAggregatedStats returns the total resource usage of an app at its most
// recent collection tick (summed across all of the app's containers). Because
// created_at is second-precision, all containers collected in one tick share
// the same timestamp, so summing that tick yields the app total.
func (r *Repository) LatestAggregatedStats(appID int64) (*ContainerStats, error) {
	row := r.db.QueryRow(`
		SELECT
			SUM(cpu_percent),
			SUM(mem_usage_bytes),
			SUM(mem_limit_bytes),
			SUM(mem_percent),
			SUM(net_io_rx_bytes),
			SUM(net_io_tx_bytes),
			SUM(block_io_read),
			SUM(block_io_write)
		FROM container_stats
		WHERE app_id = ? AND created_at = (
			SELECT MAX(created_at) FROM container_stats WHERE app_id = ?
		)
	`, appID, appID)

	s := &ContainerStats{AppID: appID}
	if err := row.Scan(
		&s.CPUPercent, &s.MemUsageBytes, &s.MemLimitBytes, &s.MemPercent,
		&s.NetIORxBytes, &s.NetIOTxBytes, &s.BlockIORead, &s.BlockIOWrite,
	); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return s, nil
}

// LatestStatsByApp returns refined per-app resource metrics for the app list view:
// - CPU: 15-minute moving average (AVG of total container CPU per tick)
// - Memory: 1-hour peak usage (MAX of total container RAM per tick)
// - Network / Block IO: Latest snapshot
// Falls back to latest available tick if no data exists within the time window.
func (r *Repository) LatestStatsByApp() (map[int64]*ContainerStats, error) {
	rows, err := r.db.Query(`
		WITH cs_15m AS (
			SELECT app_id, AVG(tick_cpu) AS avg_cpu
			FROM (
				SELECT app_id, created_at, SUM(cpu_percent) AS tick_cpu
				FROM container_stats
				WHERE created_at >= DATETIME('now', '-15 minutes')
				GROUP BY app_id, created_at
			)
			GROUP BY app_id
		),
		cs_60m AS (
			SELECT app_id,
			       MAX(tick_mem_bytes) AS max_mem_bytes,
			       MAX(tick_mem_limit) AS max_mem_limit,
			       MAX(tick_mem_pct) AS max_mem_pct
			FROM (
				SELECT app_id, created_at,
				       SUM(mem_usage_bytes) AS tick_mem_bytes,
				       SUM(mem_limit_bytes) AS tick_mem_limit,
				       SUM(mem_percent) AS tick_mem_pct
				FROM container_stats
				WHERE created_at >= DATETIME('now', '-1 hour')
				GROUP BY app_id, created_at
			)
			GROUP BY app_id
		),
		cs_latest AS (
			SELECT cs.app_id,
			       SUM(cs.cpu_percent) AS fallback_cpu,
			       SUM(cs.mem_usage_bytes) AS fallback_mem_bytes,
			       SUM(cs.mem_limit_bytes) AS fallback_mem_limit,
			       SUM(cs.mem_percent) AS fallback_mem_pct,
			       SUM(cs.net_io_rx_bytes) AS latest_rx,
			       SUM(cs.net_io_tx_bytes) AS latest_tx,
			       SUM(cs.block_io_read) AS latest_blk_r,
			       SUM(cs.block_io_write) AS latest_blk_w
			FROM container_stats cs
			JOIN (
				SELECT app_id, MAX(created_at) AS max_ts FROM container_stats GROUP BY app_id
			) m ON m.app_id = cs.app_id AND m.max_ts = cs.created_at
			GROUP BY cs.app_id
		)
		SELECT 
			l.app_id,
			COALESCE(c15.avg_cpu, l.fallback_cpu) AS cpu_percent,
			COALESCE(c60.max_mem_bytes, l.fallback_mem_bytes) AS mem_usage_bytes,
			COALESCE(c60.max_mem_limit, l.fallback_mem_limit) AS mem_limit_bytes,
			COALESCE(c60.max_mem_pct, l.fallback_mem_pct) AS mem_percent,
			l.latest_rx, l.latest_tx, l.latest_blk_r, l.latest_blk_w
		FROM cs_latest l
		LEFT JOIN cs_15m c15 ON c15.app_id = l.app_id
		LEFT JOIN cs_60m c60 ON c60.app_id = l.app_id
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

func (r *Repository) ContainerStatsCPUHistory(appID int64, since time.Time, bucketMinutes int) ([]ChartPoint, error) {
	bucketSecs := bucketMinutes * 60
	if bucketSecs <= 0 {
		bucketSecs = 60
	}
	groupBy := fmt.Sprintf("(strftime('%%s', created_at) / %d) * %d", bucketSecs, bucketSecs)
	query := fmt.Sprintf(`
		WITH ticks AS (
			SELECT app_id, created_at, SUM(cpu_percent) AS tick_cpu
			FROM container_stats
			WHERE app_id = ? AND created_at >= ?
			GROUP BY app_id, created_at
		)
		SELECT datetime(%s, 'unixepoch') AS bucket, AVG(tick_cpu)
		FROM ticks
		GROUP BY bucket ORDER BY bucket ASC
	`, groupBy)
	return r.queryChartPoints(query, appID, since)
}

func (r *Repository) ContainerStatsMemHistory(appID int64, since time.Time, bucketMinutes int) ([]ChartPoint, error) {
	bucketSecs := bucketMinutes * 60
	if bucketSecs <= 0 {
		bucketSecs = 60
	}
	groupBy := fmt.Sprintf("(strftime('%%s', created_at) / %d) * %d", bucketSecs, bucketSecs)
	query := fmt.Sprintf(`
		WITH ticks AS (
			SELECT app_id, created_at, SUM(mem_percent) AS tick_mem
			FROM container_stats
			WHERE app_id = ? AND created_at >= ?
			GROUP BY app_id, created_at
		)
		SELECT datetime(%s, 'unixepoch') AS bucket, AVG(tick_mem)
		FROM ticks
		GROUP BY bucket ORDER BY bucket ASC
	`, groupBy)
	return r.queryChartPoints(query, appID, since)
}

func (r *Repository) ContainerStatsNetHistory(appID int64, since time.Time, bucketMinutes int) ([]ChartPoint, error) {
	bucketSecs := bucketMinutes * 60
	sinceLookback := since.Add(-time.Duration(bucketSecs) * time.Second)
	groupBy := fmt.Sprintf("(strftime('%%s', created_at) / %d) * %d", bucketSecs, bucketSecs)
	query := fmt.Sprintf(`
		WITH b AS (
			SELECT datetime(%s, 'unixepoch') as bucket,
			       SUM(net_io_rx_bytes + net_io_tx_bytes) as total_bytes
			FROM container_stats
			WHERE app_id = ? AND created_at >= ?
			GROUP BY bucket
		),
		rates AS (
			SELECT bucket,
			       total_bytes - LAG(total_bytes) OVER (ORDER BY bucket ASC) as delta_bytes
			FROM b
		)
		SELECT bucket,
		       CASE
		         WHEN delta_bytes IS NULL OR delta_bytes < 0 THEN 0.0
		         ELSE delta_bytes / %d.0
		       END as net_rate
		FROM rates
		WHERE bucket >= ?
		ORDER BY bucket ASC
	`, groupBy, bucketSecs)
	return r.queryChartPoints(query, appID, sinceLookback, since)
}

// AppDiskUsageHistory returns disk usage over time from the low-frequency
// app_disk_stats table (collected every 5 minutes), bucketed like the other
// history queries. MAX per bucket picks the representative value.
func (r *Repository) AppDiskUsageHistory(appID int64, since time.Time, bucketMinutes int) ([]ChartPoint, error) {
	groupBy := fmt.Sprintf("(strftime('%%s', created_at) / %d) * %d", bucketMinutes*60, bucketMinutes*60)
	query := fmt.Sprintf(`
		SELECT datetime(%s, 'unixepoch') as bucket, MAX(disk_usage_bytes)
		FROM app_disk_stats
		WHERE app_id = ? AND created_at >= ?
		GROUP BY bucket ORDER BY bucket ASC
	`, groupBy)
	return r.queryChartPoints(query, appID, since)
}

func (r *Repository) queryChartPoints(query string, args ...interface{}) ([]ChartPoint, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ChartPoint
	for rows.Next() {
		var bucket string
		var val float64
		if err := rows.Scan(&bucket, &val); err != nil {
			return nil, err
		}
		points = append(points, ChartPoint{Time: bucket, Value: val})
	}
	return points, rows.Err()
}

func (r *Repository) PruneContainerStats(before time.Time) error {
	_, err := r.db.Exec(`DELETE FROM container_stats WHERE created_at < ?`, before)
	return err
}

// InsertAppDiskUsage appends a disk usage sample for an app. Called by the
// 5-minute disk collector (separate from the 10s container_stats collector).
func (r *Repository) InsertAppDiskUsage(appID, serverID, bytes int64) error {
	_, err := r.db.Exec(
		`INSERT INTO app_disk_stats (app_id, server_id, disk_usage_bytes) VALUES (?, ?, ?)`,
		appID, serverID, bytes)
	return err
}

// LatestAppDiskUsage returns the most recent disk usage sample for an app in
// bytes, or 0 if none has been collected yet.
func (r *Repository) LatestAppDiskUsage(appID int64) (int64, error) {
	var bytes int64
	err := r.db.QueryRow(
		`SELECT disk_usage_bytes FROM app_disk_stats WHERE app_id = ? ORDER BY created_at DESC LIMIT 1`,
		appID).Scan(&bytes)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return bytes, err
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

func (r *Repository) PruneAppDiskStats(before time.Time) error {
	_, err := r.db.Exec(`DELETE FROM app_disk_stats WHERE created_at < ?`, before)
	return err
}

func (r *Repository) InsertRouteStats(s *RouteStats) error {
	_, err := r.db.Exec(`
		INSERT INTO route_stats (app_id, domain, total_requests, requests_rps, status_2xx, status_3xx, status_4xx, status_5xx)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.AppID, s.Domain, s.TotalRequests, s.RequestsRPS, s.Status2xx, s.Status3xx, s.Status4xx, s.Status5xx)
	return err
}

func (r *Repository) LatestRouteStats(appID int64) (*RouteStats, error) {
	s := &RouteStats{}
	err := r.db.QueryRow(`
		SELECT id, app_id, domain, total_requests, requests_rps, status_2xx, status_3xx, status_4xx, status_5xx, created_at
		FROM route_stats WHERE app_id = ? ORDER BY created_at DESC LIMIT 1
	`, appID).Scan(&s.ID, &s.AppID, &s.Domain, &s.TotalRequests, &s.RequestsRPS, &s.Status2xx, &s.Status3xx, &s.Status4xx, &s.Status5xx, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *Repository) RouteStatsHistory(appID int64, since time.Time, bucketMinutes int) ([]ChartPoint, error) {
	groupBy := fmt.Sprintf("(strftime('%%s', created_at) / %d) * %d", bucketMinutes*60, bucketMinutes*60)
	query := fmt.Sprintf(`
		SELECT datetime(%s, 'unixepoch') as bucket, MAX(requests_rps)
		FROM route_stats
		WHERE app_id = ? AND created_at >= ?
		GROUP BY bucket ORDER BY bucket ASC
	`, groupBy)
	return r.queryChartPoints(query, appID, since)
}

func (r *Repository) PruneRouteStats(before time.Time) error {
	_, err := r.db.Exec(`DELETE FROM route_stats WHERE created_at < ?`, before)
	return err
}
