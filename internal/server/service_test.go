package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/go-chi/chi/v5"
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

// DELETE /api/servers/{id}: 409 while an app uses the server, otherwise the
// row and its key files go.
func TestDeleteServer(t *testing.T) {
	svc, repo := setupTestServerService(t)
	keyDir := t.TempDir()
	r := chi.NewRouter()
	r.Delete("/api/servers/{id}", NewHandler(svc, keyDir).Delete)
	del := func(id int64) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/servers/%d", id), nil))
		return rec.Code
	}

	s := &Server{Name: "worker-1", Host: "10.0.0.1", SSHKey: "/tmp/key"}
	if err := repo.Create(s); err != nil {
		t.Fatal(err)
	}
	keys := []string{filepath.Join(keyDir, fmt.Sprintf("%d.pem", s.ID)), filepath.Join(keyDir, fmt.Sprintf("%d.registry-token", s.ID))}
	for _, k := range keys {
		os.WriteFile(k, []byte("x"), 0600)
	}
	if _, err := repo.db.Exec("INSERT INTO apps (name, server_id, compose) VALUES ('web', ?, 'services: {}')", s.ID); err != nil {
		t.Fatal(err)
	}

	if code := del(s.ID); code != http.StatusConflict {
		t.Fatalf("delete with app: %d, want 409", code)
	}
	repo.db.Exec("DELETE FROM apps")
	if code := del(s.ID); code != http.StatusOK {
		t.Fatalf("delete without apps: %d, want 200", code)
	}
	if got, _ := repo.Get(s.ID); got != nil {
		t.Error("server row still there")
	}
	for _, k := range keys {
		if _, err := os.Stat(k); !os.IsNotExist(err) {
			t.Errorf("%s not removed", k)
		}
	}
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

// The host key store round-trips on servers.host_key, and PATCH forgets the
// key on host_key_clear or when host or port change.
func TestHostKeyStoreAndClear(t *testing.T) {
	svc, repo := setupTestServerService(t)
	r := chi.NewRouter()
	r.Patch("/api/servers/{id}", NewHandler(svc, t.TempDir()).Update)

	s := &Server{Name: "worker-1", Host: "10.0.0.1", Port: 22, SSHKey: "/tmp/key"}
	if err := repo.Create(s); err != nil {
		t.Fatal(err)
	}
	if k, err := repo.HostKey("10.0.0.1", 22); err != nil || k != "" {
		t.Fatalf("new server: key %q, err %v", k, err)
	}
	for _, tc := range []struct {
		body string
		want string
	}{
		{`{"name":"renamed"}`, "AAAA"},
		{`{"host_key_clear":true}`, ""},
		{`{"port":2222}`, ""},
	} {
		if err := repo.SaveHostKey(s.Host, 22, "AAAA"); err != nil {
			t.Fatal(err)
		}
		if k, _ := repo.HostKey(s.Host, 22); k != "AAAA" {
			t.Fatalf("stored key %q", k)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/servers/%d", s.ID), strings.NewReader(tc.body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("patch %s: %d %s", tc.body, rec.Code, rec.Body)
		}
		if got, _ := repo.Get(s.ID); got.HostKey != tc.want {
			t.Fatalf("after %s: host key %q, want %q", tc.body, got.HostKey, tc.want)
		}
	}
}
