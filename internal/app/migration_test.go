package app

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
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

// recConn records commands per host and can fail parts of a move.
type recConn struct {
	*ssh.MockClient
	f    *moveFixture
	host string
}

func (c *recConn) record(cmd string) {
	c.f.mu.Lock()
	c.f.cmds = append(c.f.cmds, c.host+" "+cmd)
	c.f.mu.Unlock()
}

func (c *recConn) Exec(cmd string) (string, error) {
	c.record(cmd)
	if c.f.direct {
		switch {
		case strings.Contains(cmd, "ssh-keyscan"):
			return "10.0.0.2 ssh-ed25519 AAAAhost", nil
		case strings.Contains(cmd, "ssh-keygen"):
			return "/tmp/tmp.mig\nssh-ed25519 AAAAmigkey dockify-migrate-app-1-1", nil
		case strings.Contains(cmd, "ssh_host_"):
			return "ssh-ed25519 AAAAhost root@prod\n", nil
		}
	}
	return c.MockClient.Exec(cmd)
}

func (c *recConn) ExecLong(cmd string) (string, error) {
	c.record(cmd)
	if c.f.failDown && c.host == c.f.src.Host && strings.Contains(cmd, " down") {
		return "permission denied", errors.New("exit status 1")
	}
	return c.MockClient.ExecLong(cmd)
}

func (c *recConn) ExecPipe(cmd string, stdin io.Reader, stdout io.Writer) error {
	c.record("PIPE " + cmd)
	if c.f.failPipe && (stdin != nil || strings.Contains(cmd, "ssh -i")) {
		return errors.New("tar: write error: No space left on device")
	}
	return c.MockClient.ExecPipe(cmd, stdin, stdout)
}

type moveFixture struct {
	repo     *Repository
	svc      *Service
	app      *App
	src, dst *server.Server
	mu       sync.Mutex
	cmds     []string
	direct   bool // source can reach the target over SSH
	failPipe bool // the copy stream fails
	failDown bool // `compose down` fails on the source
}

func newMoveFixture(t *testing.T) *moveFixture {
	repo, srvRepo := setupRepo(t)
	f := &moveFixture{repo: repo, svc: NewService(repo, srvRepo, nil, nil)}
	f.svc.SetConnFactory(func(host string, port int, user, keyPath string) (ssh.Connector, error) {
		return &recConn{MockClient: ssh.NewMockClient(), f: f, host: host}, nil
	})
	f.src = &server.Server{Name: "staging", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
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

// assertStayed checks a failed move left the app running on the source, untouched.
func (f *moveFixture) assertStayed(t *testing.T) {
	t.Helper()
	got, _ := f.repo.Get(f.app.ID)
	if got.ServerID != f.src.ID {
		t.Errorf("server_id changed to %d, want %d", got.ServerID, f.src.ID)
	}
	if got.Status != StatusRunning {
		t.Errorf("status = %q, want running", got.Status)
	}
	routes, _ := f.repo.GetRoutes(f.app.ID)
	if len(routes) != 1 || routes[0].ServerID != f.src.ID {
		t.Errorf("routes changed: %+v", routes)
	}
	if !f.ranOn(f.src, "up -d") {
		t.Errorf("app not restarted on old server; commands: %v", f.cmds)
	}
	if f.ranOn(f.dst, "up -d") || f.ranOn(f.src, "rm -rf /opt") {
		t.Errorf("unexpected deploy or purge; commands: %v", f.cmds)
	}
	deps, _ := f.repo.ListDeployments(f.app.ID)
	if len(deps) != 1 || deps[0].Status != StatusFailed || !strings.Contains(deps[0].Log, "aborted") {
		t.Errorf("want one failed 'aborted' deployment, got %+v", deps)
	}
}

func TestMoveAppCopyFailureKeepsAppOnOldServer(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "relay", true: "direct"}[direct], func(t *testing.T) {
			f := newMoveFixture(t)
			f.direct, f.failPipe = direct, true
			f.svc.MoveApp(f.app.ID, f.src.ID, f.dst.ID, true)
			f.assertStayed(t)
			if direct && !f.ranOn(f.dst, "sed -i --follow-symlinks '/dockify-migrate-app-") {
				t.Errorf("migration key not revoked on target; commands: %v", f.cmds)
			}
		})
	}
}

func TestMoveAppStopFailureAborts(t *testing.T) {
	f := newMoveFixture(t)
	f.failDown = true
	f.svc.MoveApp(f.app.ID, f.src.ID, f.dst.ID, true)
	f.assertStayed(t)
	if f.ranOn(f.src, "PIPE") || f.ranOn(f.dst, "PIPE") {
		t.Errorf("files were streamed although containers did not stop; commands: %v", f.cmds)
	}
}

func TestMoveAppDirectSuccess(t *testing.T) {
	f := newMoveFixture(t)
	f.direct = true
	f.svc.MoveApp(f.app.ID, f.src.ID, f.dst.ID, false)

	got, _ := f.repo.Get(f.app.ID)
	if got.ServerID != f.dst.ID || got.Status != StatusRunning {
		t.Fatalf("want running on %d, got %q on %d", f.dst.ID, got.Status, got.ServerID)
	}
	routes, _ := f.repo.GetRoutes(f.app.ID)
	if len(routes) != 1 || routes[0].ServerID != f.dst.ID {
		t.Errorf("routes not moved: %+v", routes)
	}
	if !f.ranOn(f.src, "PIPE") || !f.ranOn(f.src, "| ssh -i /tmp/tmp.mig/k") || f.ranOn(f.dst, "PIPE") {
		t.Errorf("expected a direct source->target stream; commands: %v", f.cmds)
	}
	for _, want := range []string{"restrict,command=", "expiry-time=", "Z\" ssh-ed25519 AAAAmigkey"} {
		if !f.ranOn(f.dst, want) {
			t.Errorf("authorized_keys entry missing %q; commands: %v", want, f.cmds)
		}
	}
	if !f.ranOn(f.dst, "sed -i --follow-symlinks '/dockify-migrate-app-") || !f.ranOn(f.src, "rm -rf '/tmp/tmp.mig'") {
		t.Errorf("migration key not cleaned up; commands: %v", f.cmds)
	}
	if !f.ranOn(f.src, " down") || f.ranOn(f.src, "rm -rf /opt") {
		t.Errorf("old server should be stopped, not purged; commands: %v", f.cmds)
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
