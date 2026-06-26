package scheduler

import (
	"testing"

	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
)

func setupTestDB(t *testing.T) *server.Repository {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return server.NewRepository(database)
}

func insertServer(t *testing.T, repo *server.Repository, name string, status string, cpu, ram float64) int64 {
	t.Helper()
	s := &server.Server{
		Name:   name,
		Host:   "10.0.0.1",
		Port:   22,
		User:   "root",
		SSHKey: "/tmp/key",
		Status: status,
	}
	if err := repo.Create(s); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	if err := repo.UpdateResources(s.ID, 4, 8192, 50, cpu, ram, 0); err != nil {
		t.Fatalf("update resources: %v", err)
	}
	return s.ID
}

func TestPickServer_NoServers(t *testing.T) {
	repo := setupTestDB(t)
	sch := New(repo)

	_, err := sch.PickServer()
	if err == nil {
		t.Fatal("expected error for no servers")
	}
}

func TestPickServer_NoOnlineServers(t *testing.T) {
	repo := setupTestDB(t)
	sch := New(repo)

	insertServer(t, repo, "offline-1", "offline", 10, 20)
	insertServer(t, repo, "pending-1", "pending", 5, 15)

	_, err := sch.PickServer()
	if err == nil {
		t.Fatal("expected error when no online servers")
	}
}

func TestPickServer_SingleOnline(t *testing.T) {
	repo := setupTestDB(t)
	sch := New(repo)

	insertServer(t, repo, "worker-1", "online", 30, 50)

	s, err := sch.PickServer()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "worker-1" {
		t.Fatalf("expected worker-1, got %s", s.Name)
	}
}

func TestPickServer_LeastLoaded(t *testing.T) {
	repo := setupTestDB(t)
	sch := New(repo)

	insertServer(t, repo, "heavy", "online", 90, 80)
	insertServer(t, repo, "light", "online", 10, 5)
	insertServer(t, repo, "medium", "online", 50, 40)
	insertServer(t, repo, "busy-by-ram", "online", 20, 95)

	s, err := sch.PickServer()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "light" {
		t.Fatalf("expected light (lowest load), got %s", s.Name)
	}
}

func TestPickServer_MixedStatuses(t *testing.T) {
	repo := setupTestDB(t)
	sch := New(repo)

	insertServer(t, repo, "unused", "online", 0, 0)
	insertServer(t, repo, "dead", "offline", 5, 5)

	s, err := sch.PickServer()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "unused" {
		t.Fatalf("expected unused, got %s", s.Name)
	}
}
