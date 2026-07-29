package app

import (
	"testing"

	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
)

func TestCopyAppFilesAndCleanup(t *testing.T) {
	repo, srvRepo := setupRepo(t)

	s := NewService(repo, srvRepo, nil, nil)
	s.SetConnFactory(ssh.MockFactory())

	s1 := &server.Server{Name: "Server1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	s2 := &server.Server{Name: "Server2", Host: "10.0.0.2", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	if err := srvRepo.Create(s1); err != nil {
		t.Fatalf("create s1: %v", err)
	}
	if err := srvRepo.Create(s2); err != nil {
		t.Fatalf("create s2: %v", err)
	}

	a := &App{Name: "test-app", ServerID: s1.ID, Status: StatusRunning, Compose: "services:\n  app:\n    image: nginx"}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create app: %v", err)
	}

	// Test CopyAppFiles streaming between s1 and s2
	err := s.CopyAppFiles(a.ID, s1.ID, s2.ID)
	if err != nil {
		t.Fatalf("CopyAppFiles failed: %v", err)
	}

	// Test CleanupFromServer with purgeOld = true
	s.CleanupFromServer(a.ID, s1.ID, true)

	// Test CleanupFromServer with purgeOld = false
	s.CleanupFromServer(a.ID, s1.ID, false)
}

func TestUndeployPurge(t *testing.T) {
	repo, srvRepo := setupRepo(t)

	s := NewService(repo, srvRepo, nil, nil)
	s.SetConnFactory(ssh.MockFactory())

	s1 := &server.Server{Name: "Server1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	if err := srvRepo.Create(s1); err != nil {
		t.Fatalf("create s1: %v", err)
	}

	a := &App{Name: "test-app", ServerID: s1.ID, Status: StatusRunning, Compose: "services:\n  app:\n    image: nginx"}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create app: %v", err)
	}

	err := s.Undeploy(a.ID)
	if err != nil {
		t.Fatalf("Undeploy failed: %v", err)
	}

	appInDB, _ := repo.Get(a.ID)
	if appInDB != nil {
		t.Fatalf("expected app to be deleted from DB after undeploy")
	}
}
