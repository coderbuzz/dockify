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

	// Migrasi: hapus kolom webhook_secret dari apps (global settings sekarang)
	db.Exec("ALTER TABLE apps DROP COLUMN webhook_secret")

	// Migrasi: tambah kolom disk_usage (v0.4.0)
	db.Exec("ALTER TABLE servers ADD COLUMN disk_usage REAL")
	db.Exec("ALTER TABLE servers ADD COLUMN resources_updated_at DATETIME")

	// Migrasi: tambah kolom unique_service_name (v0.3.0)
	db.Exec("ALTER TABLE apps ADD COLUMN unique_service_name INTEGER DEFAULT 0")

	return db, nil
}
