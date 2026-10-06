package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

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
	_, err := s.CopyAppFiles(a.ID, s1.ID, s2.ID)
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

// recConn records ExecLong commands per host and can fail parts of a move.
type recConn struct {
	*ssh.MockClient
	host     string
	mu       *sync.Mutex
	cmds     *[]string
	failPipe bool // fail the target-side unpack
	failDown bool // fail `compose down`
	piped    *bool
}

func (c *recConn) ExecLong(cmd string) (string, error) {
	c.mu.Lock()
	*c.cmds = append(*c.cmds, c.host+" "+cmd)
	c.mu.Unlock()
	if c.failDown && strings.Contains(cmd, " down") {
		return "permission denied", errors.New("exit status 1")
	}
	return c.MockClient.ExecLong(cmd)
}

func (c *recConn) ExecPipe(cmd string, stdin io.Reader, stdout io.Writer) error {
	c.mu.Lock()
	*c.piped = true
	c.mu.Unlock()
	if c.failPipe && stdin != nil {
		return errors.New("tar: write error: No space left on device")
	}
	return c.MockClient.ExecPipe(cmd, stdin, stdout)
}

type moveFixture struct {
	repo     *Repository
	svc      *Service
	app      *App
	src, dst *server.Server
	cmds     []string
	piped    bool
	mu       sync.Mutex
	failPipe bool
	failDown bool
}

func newMoveFixture(t *testing.T) *moveFixture {
	repo, srvRepo := setupRepo(t)
	f := &moveFixture{repo: repo, svc: NewService(repo, srvRepo, nil, nil)}
	f.svc.SetConnFactory(func(host string, port int, user, keyPath string) (ssh.Connector, error) {
		return &recConn{MockClient: ssh.NewMockClient(), host: host, mu: &f.mu, cmds: &f.cmds, piped: &f.piped,
			failPipe: f.failPipe && host == f.dst.Host, failDown: f.failDown && host == f.src.Host}, nil
	})
	f.src = &server.Server{Name: "staging", Host: "10.0.0.1", Port: 22, User: "dockify", SSHKey: "/tmp/key", Status: "online"}
	f.dst = &server.Server{Name: "prod-heavy-1", Host: "10.0.0.2", Port: 22, User: "dockify", SSHKey: "/tmp/key", Status: "online"}
	if err := srvRepo.Create(f.src); err != nil {
		t.Fatal(err)
	}
	if err := srvRepo.Create(f.dst); err != nil {
		t.Fatal(err)
	}
	f.app = &App{Name: "mongo", ServerID: f.src.ID, Status: StatusRunning, Domain: "mongo.example.com", Compose: "services:\n  mongo:\n    image: mongo\n    volumes:\n      - ./data:/data/db"}
	if err := repo.Create(f.app); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveRoute(&Route{AppID: f.app.ID, ServerID: f.src.ID, Domain: "mongo.example.com"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *moveFixture) ranOn(srv *server.Server, substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if strings.HasPrefix(c, srv.Host+" ") && strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

type noServers struct{}

func (noServers) List() ([]ServerInfo, error) { return nil, nil }

func TestEditMoveCopyFailureKeepsAppOnOldServer(t *testing.T) {
	f := newMoveFixture(t)
	f.failPipe = true

	form := url.Values{
		"name": {"mongo"}, "mode": {"advanced"}, "compose": {f.app.Compose},
		"server_id": {strconv.FormatInt(f.dst.ID, 10)}, "domain": {"mongo.example.com"},
		"move_files": {"1"}, "purge_old": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/apps/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(f.app.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	var gotStatus int
	var gotErr string
	render := func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{}) {
		gotStatus = status
		gotErr, _ = data.(map[string]interface{})["Error"].(string)
	}
	NewWebHandler(f.svc, noServers{}).AppEditForm(httptest.NewRecorder(), req, render)

	if gotStatus != http.StatusInternalServerError || !strings.Contains(gotErr, "aborted") {
		t.Fatalf("expected aborted error page, got %d %q", gotStatus, gotErr)
	}
	got, _ := f.repo.Get(f.app.ID)
	if got.ServerID != f.src.ID {
		t.Errorf("server_id changed to %d, want %d", got.ServerID, f.src.ID)
	}
	if got.Status != StatusRunning {
		t.Errorf("status = %q, want running (no redeploy)", got.Status)
	}
	routes, _ := f.repo.GetRoutes(f.app.ID)
	if len(routes) != 1 || routes[0].ServerID != f.src.ID {
		t.Errorf("routes changed: %+v", routes)
	}
	if !f.ranOn(f.src, "up -d") {
		t.Errorf("app not restarted on old server; commands: %v", f.cmds)
	}
	if f.ranOn(f.dst, "up -d") || f.ranOn(f.src, "rm -rf") {
		t.Errorf("unexpected deploy or purge; commands: %v", f.cmds)
	}
}

func TestCopyAppFilesAbortsWhenStopFails(t *testing.T) {
	f := newMoveFixture(t)
	f.failDown = true

	if _, err := f.svc.CopyAppFiles(f.app.ID, f.src.ID, f.dst.ID); err == nil {
		t.Fatal("expected error when compose down fails")
	}
	if f.piped {
		t.Error("files were streamed although containers did not stop")
	}
	if !f.ranOn(f.src, "up -d") {
		t.Errorf("app not restarted on old server; commands: %v", f.cmds)
	}
}

func TestDockerVolumes(t *testing.T) {
	compose := `services:
  db:
    image: mongo
    volumes:
      - ./data:/data/db
      - /opt/x:/x
      - mongo-config:/data/configdb
      - /anon
      - type: volume
        source: ch-data
        target: /var/lib/clickhouse
      - type: bind
        source: ./logs
        target: /logs
volumes:
  mongo-config:
  ch-data:
`
	want := []string{"/anon", "ch-data", "mongo-config"}
	if got := dockerVolumes(compose); !reflect.DeepEqual(got, want) {
		t.Errorf("dockerVolumes = %v, want %v", got, want)
	}
}
