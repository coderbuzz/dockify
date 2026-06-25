package app

import (
	"database/sql"
	"fmt"
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
		       git_repo, git_branch, auth_user, auth_pass, status, created_at, updated_at
		FROM apps ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var gitRepo, gitBranch sql.NullString
		if err := rows.Scan(
			&a.ID, &a.Name, &a.ServerID, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) Get(id int64) (*App, error) {
	a := &App{}
	var gitRepo, gitBranch sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, created_at, updated_at
		FROM apps WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &a.ServerID, &a.Domain, &a.Port, &a.Compose,
		&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.GitRepo = gitRepo.String
	a.GitBranch = gitBranch.String
	return a, nil
}

func (r *Repository) FindAllByGitRepo(repo, branch string) ([]App, error) {
	rows, err := r.db.Query(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, created_at, updated_at
		FROM apps WHERE git_repo = ? AND git_branch = ? ORDER BY id
	`, repo, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var gitRepo, gitBranch sql.NullString
		if err := rows.Scan(
			&a.ID, &a.Name, &a.ServerID, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) FindByGitRepo(repo, branch string) (*App, error) {
	a := &App{}
	var gitRepo, gitBranch sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, created_at, updated_at
		FROM apps WHERE git_repo = ? AND git_branch = ? LIMIT 1
	`, repo, branch).Scan(
		&a.ID, &a.Name, &a.ServerID, &a.Domain, &a.Port, &a.Compose,
		&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.GitRepo = gitRepo.String
	a.GitBranch = gitBranch.String
	return a, nil
}

func (r *Repository) ListByServer(serverID int64) ([]App, error) {
	rows, err := r.db.Query(`
		SELECT id, name, server_id, domain, port, compose,
		       git_repo, git_branch, auth_user, auth_pass, status, created_at, updated_at
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
		if err := rows.Scan(
			&a.ID, &a.Name, &a.ServerID, &a.Domain, &a.Port, &a.Compose,
			&gitRepo, &gitBranch, &a.AuthUser, &a.AuthPass, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.GitRepo = gitRepo.String
		a.GitBranch = gitBranch.String
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (r *Repository) Create(a *App) error {
	result, err := r.db.Exec(`
		INSERT INTO apps (name, server_id, domain, port, compose, git_repo, git_branch, auth_user, auth_pass, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.Name, a.ServerID, a.Domain, a.Port, a.Compose, nullString(a.GitRepo), nullString(a.GitBranch), a.AuthUser, a.AuthPass, "created")
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
			git_repo=?, git_branch=?, auth_user=?, auth_pass=?, status=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, a.Name, a.ServerID, a.Domain, a.Port, a.Compose,
		nullString(a.GitRepo), nullString(a.GitBranch), a.AuthUser, a.AuthPass, a.Status, a.ID)
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
	ID    int64
	AppID int64
	Key   string
	Value string
}

func (r *Repository) ListSecrets(appID int64) ([]AppSecret, error) {
	rows, err := r.db.Query(`SELECT id, app_id, key, value FROM app_secrets WHERE app_id = ? ORDER BY key`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var secrets []AppSecret
	for rows.Next() {
		var s AppSecret
		if err := rows.Scan(&s.ID, &s.AppID, &s.Key, &s.Value); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (r *Repository) SetSecret(appID int64, key, value string) error {
	_, err := r.db.Exec(`INSERT INTO app_secrets (app_id, key, value) VALUES (?, ?, ?) ON CONFLICT(app_id, key) DO UPDATE SET value = ?`, appID, key, value, value)
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

func (r *Repository) SetFile(appID int64, path, content string) error {
	_, err := r.db.Exec(`INSERT INTO app_files (app_id, path, content) VALUES (?, ?, ?) ON CONFLICT(app_id, path) DO UPDATE SET content = ?`, appID, path, content, content)
	return err
}

func (r *Repository) DeleteFile(appID int64, path string) error {
	_, err := r.db.Exec(`DELETE FROM app_files WHERE app_id = ? AND path = ?`, appID, path)
	return err
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
