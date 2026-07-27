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

func TestRepo_LatestBatchedStats(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	a1 := &App{Name: "app-1", Domain: "a1.com", Port: 80, ServerID: srvID, Compose: "x"}
	a2 := &App{Name: "app-2", Domain: "a2.com", Port: 81, ServerID: srvID, Compose: "x"}
	repo.Create(a1)
	repo.Create(a2)

	// Insert container stats for app-1 and app-2
	err1 := repo.InsertContainerStats(&ContainerStats{
		AppID: a1.ID, ServerID: srvID, ContainerName: "web1",
		CPUPercent: 25.5, MemUsageBytes: 1024 * 1024 * 50, MemLimitBytes: 1024 * 1024 * 100, MemPercent: 50.0,
	})
	err2 := repo.InsertContainerStats(&ContainerStats{
		AppID: a2.ID, ServerID: srvID, ContainerName: "web2",
		CPUPercent: 12.0, MemUsageBytes: 1024 * 1024 * 30, MemLimitBytes: 1024 * 1024 * 100, MemPercent: 30.0,
	})
	if err1 != nil || err2 != nil {
		t.Fatalf("insert container stats failed: %v, %v", err1, err2)
	}

	// Insert disk stats
	if err := repo.InsertAppDiskUsage(a1.ID, srvID, 500000); err != nil {
		t.Fatalf("insert disk stats failed: %v", err)
	}

	// Test LatestStatsByApp
	statsMap, err := repo.LatestStatsByApp()
	if err != nil {
		t.Fatalf("LatestStatsByApp failed: %v", err)
	}
	if len(statsMap) != 2 {
		t.Fatalf("expected 2 app stats entries, got %d", len(statsMap))
	}
	if statsMap[a1.ID].CPUPercent != 25.5 {
		t.Errorf("expected CPU 25.5 for a1, got %f", statsMap[a1.ID].CPUPercent)
	}
	if statsMap[a2.ID].CPUPercent != 12.0 {
		t.Errorf("expected CPU 12.0 for a2, got %f", statsMap[a2.ID].CPUPercent)
	}

	// Test LatestDiskByApp
	diskMap, err := repo.LatestDiskByApp()
	if err != nil {
		t.Fatalf("LatestDiskByApp failed: %v", err)
	}
	if diskMap[a1.ID] != 500000 {
		t.Errorf("expected 500000 disk usage for a1, got %d", diskMap[a1.ID])
	}
	if _, ok := diskMap[a2.ID]; ok {
		t.Errorf("expected no disk entry for a2")
	}
}
