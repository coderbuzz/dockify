package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// migration upgrades the schema by one version. Each runs once, in its own
// transaction, after schema.sql (the baseline, CREATE ... IF NOT EXISTS).
type migration struct {
	desc string
	fn   func(*sql.Tx) error
}

// migrations is append-only: PRAGMA user_version holds the number applied.
var migrations = []migration{
	{"legacy column changes (v0.3-v0.13)", migrateLegacy},
	{"cascade app deletes, keep history on server delete", migrateForeignKeys},
}

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

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies the migrations above user_version. Foreign keys are off
// while they run (the pragma is a no-op inside a transaction, so it is set on
// the connection) to allow table rebuilds; foreign_key_check guards each
// commit instead. On error Open closes the pool, so the off state cannot leak.
func migrate(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("migrate: read user_version: %w", err)
	}
	if version >= len(migrations) {
		return nil
	}
	log.Printf("migrating database from version %d to %d", version, len(migrations))

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for v := version + 1; v <= len(migrations); v++ {
		m := migrations[v-1]
		log.Printf("migration %d: %s", v, m.desc)
		if err := runMigration(ctx, conn, v, m.fn); err != nil {
			return fmt.Errorf("migration %d (%s) failed, database left at version %d: %w", v, m.desc, v-1, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Printf("database at version %d", len(migrations))
	return nil
}

func runMigration(ctx context.Context, conn *sql.Conn, version int, fn func(*sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	var table, parent string
	var rowid sql.NullInt64
	var fkid int
	err = tx.QueryRow("PRAGMA foreign_key_check").Scan(&table, &rowid, &parent, &fkid)
	if err == nil {
		return fmt.Errorf("foreign key violation: %s rowid %d references a missing %s row", table, rowid.Int64, parent)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

func hasColumn(tx *sql.Tx, table, column string) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&n)
	return n > 0, err
}

func addColumn(tx *sql.Tx, table, column, def string) error {
	ok, err := hasColumn(tx, table, column)
	if err != nil || ok {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def))
	return err
}

func dropColumn(tx *sql.Tx, table, column string) error {
	ok, err := hasColumn(tx, table, column)
	if err != nil || !ok {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column))
	return err
}

// migrateLegacy is the column work Open used to repeat on every start while
// ignoring errors; a database at version 0 may have any subset of it. The
// tables those releases created are in schema.sql.
func migrateLegacy(tx *sql.Tx) error {
	for _, c := range []struct{ table, column, def string }{
		{"servers", "disk_usage", "REAL"},
		{"servers", "resources_updated_at", "DATETIME"},
		{"servers", "registry_host", "TEXT DEFAULT ''"},
		{"servers", "registry_user", "TEXT DEFAULT ''"},
		{"servers", "registry_token", "TEXT DEFAULT ''"},
		{"apps", "compose_mode", "TEXT DEFAULT 'advanced'"},
		{"apps", "memory_limit", "TEXT DEFAULT ''"},
		{"apps", "cpu_limit", "TEXT DEFAULT ''"},
		{"apps", "log_max_size", "TEXT DEFAULT ''"},
		{"apps", "log_max_file", "TEXT DEFAULT ''"},
		{"apps", "command", "TEXT DEFAULT ''"},
		{"apps", "ports", "TEXT DEFAULT ''"},
		{"apps", "ulimits_nofile", "TEXT DEFAULT ''"},
		{"app_secrets", "is_secret", "INTEGER DEFAULT 0"},
	} {
		if err := addColumn(tx, c.table, c.column, c.def); err != nil {
			return fmt.Errorf("add %s.%s: %w", c.table, c.column, err)
		}
	}

	// compose_mode replaced unique_service_name (v0.5.0).
	if ok, err := hasColumn(tx, "apps", "unique_service_name"); err != nil {
		return err
	} else if ok {
		if _, err := tx.Exec("UPDATE apps SET compose_mode = 'simple' WHERE unique_service_name = 1"); err != nil {
			return err
		}
	}

	for _, c := range []struct{ table, column string }{
		{"apps", "webhook_secret"},              // a global setting now
		{"container_stats", "disk_usage_bytes"}, // moved to app_disk_stats
		{"container_stats", "pids"},
	} {
		if err := dropColumn(tx, c.table, c.column); err != nil {
			return fmt.Errorf("drop %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// migrateForeignKeys rebuilds the tables that point at apps and servers:
// deleting an app deletes its history, deleting a server keeps it with
// server_id NULL. route_stats already cascades and has no server_id.
// SQLite cannot alter a constraint, so each table is copied into a new one.
func migrateForeignKeys(tx *sql.Tx) error {
	const app = "INTEGER REFERENCES apps(id) ON DELETE CASCADE"
	const srv = "INTEGER REFERENCES servers(id) ON DELETE SET NULL"
	tables := []struct{ name, columns, index string }{
		{"deployments", `
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id           ` + app + `,
			server_id        ` + srv + `,
			status           TEXT,
			log              TEXT,
			commit_sha       TEXT,
			compose_snapshot TEXT,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP`, ""},
		{"routes", `
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id      ` + app + `,
			server_id   ` + srv + `,
			domain      TEXT NOT NULL,
			target      TEXT NOT NULL,
			status      TEXT DEFAULT 'active',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP`, ""},
		{"dns_records", `
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id      ` + app + `,
			server_id   ` + srv + `,
			zone_id     TEXT NOT NULL,
			record_id   TEXT NOT NULL,
			name        TEXT NOT NULL,
			type        TEXT DEFAULT 'A',
			content     TEXT NOT NULL,
			proxied     INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP`, ""},
		{"container_stats", `
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id           ` + app + `,
			server_id        ` + srv + `,
			container_name   TEXT NOT NULL,
			cpu_percent      REAL,
			mem_usage_bytes  INTEGER,
			mem_limit_bytes  INTEGER,
			mem_percent      REAL,
			net_io_rx_bytes  INTEGER,
			net_io_tx_bytes  INTEGER,
			block_io_read    INTEGER,
			block_io_write   INTEGER,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP`,
			"CREATE INDEX idx_container_stats_app_time ON container_stats(app_id, created_at)"},
		{"app_disk_stats", `
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id           ` + app + `,
			server_id        ` + srv + `,
			disk_usage_bytes INTEGER DEFAULT 0,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP`,
			"CREATE INDEX idx_app_disk_stats_app_time ON app_disk_stats(app_id, created_at)"},
	}
	for _, t := range tables {
		if err := rebuildTable(tx, t.name, t.columns, t.index); err != nil {
			return fmt.Errorf("rebuild %s: %w", t.name, err)
		}
	}
	return nil
}

// rebuildTable is SQLite's documented table rebuild: create the new table,
// copy the rows in one INSERT ... SELECT, drop the old one, rename, re-index.
// Only columns both tables share are copied. The AUTOINCREMENT counter is
// carried over so ids of deleted rows are not handed out again.
func rebuildTable(tx *sql.Tx, name, columns, index string) error {
	tmp := "new_" + name
	if _, err := tx.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", tmp, columns)); err != nil {
		return err
	}
	rows, err := tx.Query("SELECT n.name FROM pragma_table_info(?) n JOIN pragma_table_info(?) o ON o.name = n.name", tmp, name)
	if err != nil {
		return err
	}
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	list := strings.Join(cols, ", ")
	for _, stmt := range []string{
		fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", tmp, list, list, name),
		fmt.Sprintf("DELETE FROM sqlite_sequence WHERE name = '%s'", tmp),
		fmt.Sprintf("INSERT INTO sqlite_sequence (name, seq) SELECT '%s', seq FROM sqlite_sequence WHERE name = '%s'", tmp, name),
		fmt.Sprintf("DROP TABLE %s", name),
		fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tmp, name),
		index,
	} {
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}
