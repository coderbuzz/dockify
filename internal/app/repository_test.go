package app

import (
	"testing"

	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
)

func setupRepo(t *testing.T) (*Repository, *server.Repository) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	srvRepo := server.NewRepository(database)
	return NewRepository(database), srvRepo
}

func createTestServer(t *testing.T, srvRepo *server.Repository) int64 {
	t.Helper()
	s := &server.Server{
		Name:   "test-worker",
		Host:   "10.0.0.1",
		Port:   22,
		User:   "root",
		SSHKey: "/tmp/key",
		Status: "online",
	}
	if err := srvRepo.Create(s); err != nil {
		t.Fatalf("create server: %v", err)
	}
	return s.ID
}

func TestRepo_CreateAndGet(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	a := &App{
		Name:     "test-app",
		ServerID: srvID,
		Domain:   "test.example.com",
		Port:     3000,
		Compose:  "services:\n  web:\n    image: nginx",
		GitRepo:  "https://github.com/test/repo.git",
	}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	got, err := repo.Get(a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected app, got nil")
	}
	if got.Name != "test-app" {
		t.Errorf("Name: expected test-app, got %s", got.Name)
	}
	if got.GitRepo != "https://github.com/test/repo.git" {
		t.Errorf("GitRepo mismatch")
	}
	if got.Status != "created" {
		t.Errorf("default status: expected created, got %s", got.Status)
	}
}

func TestRepo_List(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	for _, name := range []string{"a", "b", "c"} {
		repo.Create(&App{Name: name, Domain: name + ".com", Port: 80, ServerID: srvID, Compose: "x"})
	}

	apps, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(apps) != 3 {
		t.Errorf("expected 3 apps, got %d", len(apps))
	}
}

func TestRepo_NotFound(t *testing.T) {
	repo, _ := setupRepo(t)

	got, err := repo.Get(999)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing app")
	}
}

func TestRepo_UpdateStatus(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	repo.Create(&App{Name: "up", Domain: "up.com", Port: 80, ServerID: srvID, Compose: "x"})

	if err := repo.UpdateStatus(1, "running"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := repo.Get(1)
	if got.Status != "running" {
		t.Errorf("expected running, got %s", got.Status)
	}
}

func TestRepo_Delete(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	repo.Create(&App{Name: "del", Domain: "del.com", Port: 80, ServerID: srvID, Compose: "x"})

	if err := repo.Delete(1); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, _ := repo.Get(1)
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestRepo_Deployment(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	a := &App{Name: "dep-test", Domain: "dep.com", Port: 80, ServerID: srvID, Compose: "x"}
	repo.Create(a)

	d := &Deployment{
		AppID:           a.ID,
		ServerID:        srvID,
		Status:          "success",
		Log:             "deployed",
		CommitSHA:       "abc123",
		ComposeSnapshot: "services:\n  web:\n    image: nginx",
	}
	if err := repo.AddDeployment(d); err != nil {
		t.Fatalf("add deployment: %v", err)
	}
	if d.ID == 0 {
		t.Fatal("expected non-zero deployment ID")
	}

	deps, err := repo.ListDeployments(a.ID)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].CommitSHA != "abc123" {
		t.Errorf("CommitSHA mismatch")
	}
	if deps[0].ComposeSnapshot == "" {
		t.Error("expected compose snapshot to be stored")
	}
}
