package server

import (
	"testing"

	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/ssh"
)

func setupTestServerService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := NewRepository(database)
	svc := NewService(repo)
	svc.SetConnFactory(ssh.MockFactory())
	return svc, repo
}

func TestPruneWorkerStandard(t *testing.T) {
	svc, repo := setupTestServerService(t)

	s := &Server{Name: "worker-1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	if err := repo.Create(s); err != nil {
		t.Fatalf("create server: %v", err)
	}

	out, err := svc.PruneWorker(s.ID, false, false)
	if err != nil {
		t.Fatalf("PruneWorker standard failed: %v", err)
	}
	_ = out
}

func TestPruneWorkerDeep(t *testing.T) {
	svc, repo := setupTestServerService(t)

	s := &Server{Name: "worker-2", Host: "10.0.0.2", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	if err := repo.Create(s); err != nil {
		t.Fatalf("create server: %v", err)
	}

	out, err := svc.PruneWorker(s.ID, true, true)
	if err != nil {
		t.Fatalf("PruneWorker deep failed: %v", err)
	}
	_ = out
}
