package server

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

func (r *Repository) List() ([]Server, error) {
	rows, err := r.db.Query(`
		SELECT id, name, host, port, user, ssh_key, status,
		       cpu_cores, ram_mb, disk_gb, cpu_usage, ram_usage, disk_usage,
		       resources_updated_at, created_at, updated_at
		FROM servers ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		var cpuCores, ramMB, diskGB sql.NullInt64
		var cpuUsage, ramUsage, diskUsage sql.NullFloat64
		var resourcesUpdated sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.SSHKey,
			&s.Status, &cpuCores, &ramMB, &diskGB,
			&cpuUsage, &ramUsage, &diskUsage, &resourcesUpdated,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.CPUCores = int(cpuCores.Int64)
		s.RAMMB = int(ramMB.Int64)
		s.DiskGB = int(diskGB.Int64)
		s.CPUUsage = cpuUsage.Float64
		s.RAMUsage = ramUsage.Float64
		s.DiskUsage = diskUsage.Float64
		if resourcesUpdated.Valid {
			s.ResourcesUpdatedAt = resourcesUpdated.Time
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func (r *Repository) Get(id int64) (*Server, error) {
	s := &Server{}
	var cpuCores, ramMB, diskGB sql.NullInt64
	var cpuUsage, ramUsage, diskUsage sql.NullFloat64
	var resourcesUpdated sql.NullTime
	err := r.db.QueryRow(`
		SELECT id, name, host, port, user, ssh_key, status,
		       cpu_cores, ram_mb, disk_gb, cpu_usage, ram_usage, disk_usage,
		       resources_updated_at, created_at, updated_at
		FROM servers WHERE id = ?
	`, id).Scan(
		&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.SSHKey,
		&s.Status, &cpuCores, &ramMB, &diskGB,
		&cpuUsage, &ramUsage, &diskUsage, &resourcesUpdated,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.CPUCores = int(cpuCores.Int64)
	s.RAMMB = int(ramMB.Int64)
	s.DiskGB = int(diskGB.Int64)
	s.CPUUsage = cpuUsage.Float64
	s.RAMUsage = ramUsage.Float64
	s.DiskUsage = diskUsage.Float64
	if resourcesUpdated.Valid {
		s.ResourcesUpdatedAt = resourcesUpdated.Time
	}
	return s, nil
}

func (r *Repository) Create(s *Server) error {
	result, err := r.db.Exec(`
		INSERT INTO servers (name, host, port, user, ssh_key, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.Name, s.Host, s.Port, s.User, s.SSHKey, s.Status)
	if err != nil {
		return fmt.Errorf("insert server: %w", err)
	}
	id, _ := result.LastInsertId()
	s.ID = id
	return nil
}

func (r *Repository) Update(s *Server) error {
	_, err := r.db.Exec(`
		UPDATE servers SET
			name=?, host=?, port=?, user=?, ssh_key=?, status=?,
			cpu_cores=?, ram_mb=?, disk_gb=?, cpu_usage=?, ram_usage=?, disk_usage=?,
			updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, s.Name, s.Host, s.Port, s.User, s.SSHKey, s.Status,
		s.CPUCores, s.RAMMB, s.DiskGB, s.CPUUsage, s.RAMUsage, s.DiskUsage,
		s.ID)
	return err
}

func (r *Repository) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM servers WHERE id = ?", id)
	return err
}

func (r *Repository) UpdateStatus(id int64, status string) error {
	_, err := r.db.Exec(`
		UPDATE servers SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, status, id)
	return err
}

func (r *Repository) UpdateResources(id int64, cpuCores, ramMB, diskGB int, cpuUsage, ramUsage, diskUsage float64) error {
	_, err := r.db.Exec(`
		UPDATE servers SET
			cpu_cores=?, ram_mb=?, disk_gb=?, cpu_usage=?, ram_usage=?, disk_usage=?,
			resources_updated_at=CURRENT_TIMESTAMP,
			updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, cpuCores, ramMB, diskGB, cpuUsage, ramUsage, diskUsage, id)
	return err
}

func (r *Repository) ListOnline() ([]Server, error) {
	rows, err := r.db.Query(`
		SELECT id, name, host, port, user, ssh_key, status,
		       cpu_cores, ram_mb, disk_gb, cpu_usage, ram_usage, disk_usage,
		       resources_updated_at, created_at, updated_at
		FROM servers WHERE status = 'online' ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		var cpuCores, ramMB, diskGB sql.NullInt64
		var cpuUsage, ramUsage, diskUsage sql.NullFloat64
		var resourcesUpdated sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.SSHKey,
			&s.Status, &cpuCores, &ramMB, &diskGB,
			&cpuUsage, &ramUsage, &diskUsage, &resourcesUpdated,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.CPUCores = int(cpuCores.Int64)
		s.RAMMB = int(ramMB.Int64)
		s.DiskGB = int(diskGB.Int64)
		s.CPUUsage = cpuUsage.Float64
		s.RAMUsage = ramUsage.Float64
		s.DiskUsage = diskUsage.Float64
		if resourcesUpdated.Valid {
			s.ResourcesUpdatedAt = resourcesUpdated.Time
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func nullFloat64(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}

func nullString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullInt64(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

var _ = time.Now
