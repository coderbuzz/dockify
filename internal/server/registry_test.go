package server

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/db"
)

func TestMigrationAddsRegistryColumnsToExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// servers table as it existed before registry credentials.
	if _, err := old.Exec(`CREATE TABLE servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, host TEXT NOT NULL,
		port INTEGER DEFAULT 22, user TEXT DEFAULT 'root', ssh_key TEXT NOT NULL,
		status TEXT DEFAULT 'pending', cpu_cores INTEGER, ram_mb INTEGER, disk_gb INTEGER,
		cpu_usage REAL, ram_usage REAL, disk_usage REAL, resources_updated_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO servers (name, host, ssh_key) VALUES ('legacy', '10.0.0.9', '/keys/1.pem')`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	defer database.Close()

	s, err := NewRepository(database).Get(1)
	if err != nil || s == nil {
		t.Fatalf("get legacy server: %v", err)
	}
	if s.RegistryHost != "" || s.RegistryUser != "" || s.RegistryToken != "" {
		t.Fatalf("legacy server must have no registry credential, got %+v", s)
	}
}

func TestApplyRegistry(t *testing.T) {
	dir := t.TempDir()
	s := &Server{ID: 7}

	if _, err := applyRegistry(dir, s, "", "bot", "", false); err != errRegistryPair {
		t.Fatalf("user without token: expected errRegistryPair, got %v", err)
	}
	if _, err := applyRegistry(dir, s, "", "", "tok", false); err != errRegistryPair {
		t.Fatalf("token without user: expected errRegistryPair, got %v", err)
	}

	if _, err := applyRegistry(dir, s, "", "bot", "tok-1", false); err != nil {
		t.Fatal(err)
	}
	if s.RegistryHost != DefaultRegistryHost || s.RegistryUser != "bot" {
		t.Fatalf("unexpected credential: %+v", s)
	}
	info, err := os.Stat(s.RegistryToken)
	if err != nil {
		t.Fatalf("token file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token file must be 0600, got %v", info.Mode().Perm())
	}

	// Empty token keeps the current one.
	if _, err := applyRegistry(dir, s, "registry.example.com", "bot2", "", false); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(s.RegistryToken); string(raw) != "tok-1" {
		t.Fatalf("empty token must keep current token, got %q", raw)
	}
	if s.RegistryHost != "registry.example.com" || s.RegistryUser != "bot2" {
		t.Fatalf("unexpected credential: %+v", s)
	}

	// Clearing the user while a token exists is rejected.
	if _, err := applyRegistry(dir, s, "", "", "", false); err != errRegistryPair {
		t.Fatalf("expected errRegistryPair, got %v", err)
	}

	oldPath, err := applyRegistry(dir, s, "", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if s.RegistryHost != "" || s.RegistryUser != "" || s.RegistryToken != "" {
		t.Fatalf("clear must remove the credential, got %+v", s)
	}
	removeTokenFile(oldPath)
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("token file must be removed, stat err %v", err)
	}
}

func TestServerJSONOmitsRegistryToken(t *testing.T) {
	s := Server{RegistryHost: "ghcr.io", RegistryUser: "bot", RegistryToken: "/keys/1.registry-token"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "registry_token") || strings.Contains(string(b), s.RegistryToken) {
		t.Fatalf("registry token must not be serialized: %s", b)
	}
}
