package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Migration: drop webhook_secret column from apps (global settings now)
	db.Exec("ALTER TABLE apps DROP COLUMN webhook_secret")

	// Migration: add disk_usage column (v0.4.0)
	db.Exec("ALTER TABLE servers ADD COLUMN disk_usage REAL")
	db.Exec("ALTER TABLE servers ADD COLUMN resources_updated_at DATETIME")

	// Migration: add unique_service_name column (v0.3.0)
	db.Exec("ALTER TABLE apps ADD COLUMN unique_service_name INTEGER DEFAULT 0")

	// Migration: add compose_mode column, replaces unique_service_name (v0.5.0)
	db.Exec("ALTER TABLE apps ADD COLUMN compose_mode TEXT DEFAULT 'advanced'")
	db.Exec("UPDATE apps SET compose_mode = 'simple' WHERE unique_service_name = 1")

	// Migration: add resource limit columns (v0.7.0)
	db.Exec("ALTER TABLE apps ADD COLUMN memory_limit TEXT DEFAULT ''")
	db.Exec("ALTER TABLE apps ADD COLUMN cpu_limit TEXT DEFAULT ''")
	db.Exec("ALTER TABLE apps ADD COLUMN log_max_size TEXT DEFAULT ''")
	db.Exec("ALTER TABLE apps ADD COLUMN log_max_file TEXT DEFAULT ''")

	// Migration: add is_secret column (v0.8.0)
	db.Exec("ALTER TABLE app_secrets ADD COLUMN is_secret INTEGER DEFAULT 0")

	// Migration: add command column (v0.9.0)
	db.Exec("ALTER TABLE apps ADD COLUMN command TEXT DEFAULT ''")

	// Migration: add ports column (v0.10.0)
	db.Exec("ALTER TABLE apps ADD COLUMN ports TEXT DEFAULT ''")

	// Migration: add ulimits_nofile column (v0.11.0)
	db.Exec("ALTER TABLE apps ADD COLUMN ulimits_nofile TEXT DEFAULT ''")

	// Migration: add container_stats + route_stats tables (v0.12.0)
	db.Exec(`CREATE TABLE IF NOT EXISTS container_stats (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id           INTEGER REFERENCES apps(id) ON DELETE CASCADE,
		server_id        INTEGER REFERENCES servers(id),
		container_name   TEXT NOT NULL,
		cpu_percent      REAL,
		mem_usage_bytes  INTEGER,
		mem_limit_bytes  INTEGER,
		mem_percent      REAL,
		net_io_rx_bytes  INTEGER,
		net_io_tx_bytes  INTEGER,
		block_io_read    INTEGER,
		block_io_write   INTEGER,
		pids             INTEGER,
		created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_container_stats_app_time ON container_stats(app_id, created_at)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS route_stats (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id          INTEGER REFERENCES apps(id) ON DELETE CASCADE,
		domain          TEXT NOT NULL,
		total_requests  INTEGER,
		requests_rps    REAL,
		status_2xx      INTEGER DEFAULT 0,
		status_3xx      INTEGER DEFAULT 0,
		status_4xx      INTEGER DEFAULT 0,
		status_5xx      INTEGER DEFAULT 0,
		created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_route_stats_app_time ON route_stats(app_id, created_at)`)

	return db, nil
}
