package db

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyDB writes a database the way an old Open left it: schema.sql plus
// the columns it added and dropped on every start, at user_version 0.
func legacyDB(t *testing.T, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	stmts := append([]string{
		schemaSQL,
		"ALTER TABLE apps ADD COLUMN webhook_secret TEXT",
		"ALTER TABLE apps ADD COLUMN unique_service_name INTEGER DEFAULT 0",
		"ALTER TABLE apps DROP COLUMN command", // a release that never got it
		"ALTER TABLE container_stats ADD COLUMN pids INTEGER",
		"INSERT INTO apps (name, compose, unique_service_name, compose_mode) VALUES ('legacy', 'services: {}', 1, 'advanced')",
	}, extra...)
	for _, s := range stmts {
		if _, err := old.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

func open(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func scalar(t *testing.T, d *sql.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := d.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestMigrationsRunOnce(t *testing.T) {
	path := legacyDB(t)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	d := open(t, path)
	if v := scalar(t, d, "PRAGMA user_version"); v != len(migrations) {
		t.Fatalf("user_version = %d, want %d", v, len(migrations))
	}
	if scalar(t, d, "SELECT COUNT(*) FROM apps WHERE compose_mode = 'simple'") != 1 {
		t.Error("unique_service_name = 1 not converted to compose_mode simple")
	}
	for _, c := range []struct {
		table, column string
		want          int
	}{{"apps", "command", 1}, {"apps", "webhook_secret", 0}, {"container_stats", "pids", 0}} {
		if got := scalar(t, d, "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", c.table, c.column); got != c.want {
			t.Errorf("%s.%s present = %d, want %d", c.table, c.column, got, c.want)
		}
	}
	d.Close()

	open(t, path)
	out := logs.String()
	if strings.Count(out, "migrating database") != 1 {
		t.Errorf("want exactly one migration run, log:\n%s", out)
	}
	for i := range migrations {
		if n := strings.Count(out, fmt.Sprintf("migration %d:", i+1)); n != 1 {
			t.Errorf("migration %d ran %d times", i+1, n)
		}
	}
}
