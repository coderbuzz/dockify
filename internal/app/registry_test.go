package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
)

const testRegistryToken = "ghp_testSecretToken123"

// recordingConn records every remote command (Exec and ExecPipe) in order.
type recordingConn struct {
	*ssh.MockClient
	cmds    []string
	pipes   int
	stdin   string
	pipeOut string
	pipeErr error
}

func (c *recordingConn) Exec(cmd string) (string, error) {
	c.cmds = append(c.cmds, cmd)
	return c.MockClient.Exec(cmd)
}

func (c *recordingConn) ExecPipe(cmd string, stdin io.Reader, stdout io.Writer) error {
	c.cmds = append(c.cmds, cmd)
	c.pipes++
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		c.stdin = string(b)
	}
	io.WriteString(stdout, c.pipeOut)
	return c.pipeErr
}

func setupRegistryDeploy(t *testing.T, withCredential bool, conn *recordingConn) (*Service, *Repository, int64) {
	t.Helper()
	repo, srvRepo := setupRepo(t)

	svr := &server.Server{Name: "w1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key", Status: "online"}
	if withCredential {
		path := filepath.Join(t.TempDir(), "1.registry-token")
		if err := os.WriteFile(path, []byte(testRegistryToken+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		svr.RegistryHost = "ghcr.io"
		svr.RegistryUser = "bot"
		svr.RegistryToken = path
	}
	if err := srvRepo.Create(svr); err != nil {
		t.Fatalf("create server: %v", err)
	}

	a := &App{Name: "private-app", ServerID: svr.ID, Status: StatusRunning, Compose: "services:\n  app:\n    image: ghcr.io/acme/private:latest"}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create app: %v", err)
	}

	s := NewService(repo, srvRepo, nil, nil)
	conn.MockClient = ssh.NewMockClient()
	s.SetConnFactory(func(host string, port int, user, keyPath string) (ssh.Connector, error) {
		return conn, nil
	})
	return s, repo, a.ID
}

func cmdIndex(cmds []string, substr string) int {
	for i, c := range cmds {
		if strings.Contains(c, substr) {
			return i
		}
	}
	return -1
}

func lastDeployment(t *testing.T, repo *Repository, appID int64) Deployment {
	t.Helper()
	deps, err := repo.ListDeployments(appID)
	if err != nil || len(deps) == 0 {
		t.Fatalf("expected a deployment record, got %v (err %v)", deps, err)
	}
	return deps[0]
}

func TestDeployRegistryLoginBeforePull(t *testing.T) {
	conn := &recordingConn{}
	s, repo, appID := setupRegistryDeploy(t, true, conn)

	s.deployWithCommit(appID, "")

	login := cmdIndex(conn.cmds, "docker login")
	pull := cmdIndex(conn.cmds, " pull ")
	if login < 0 || pull < 0 {
		t.Fatalf("expected login and pull commands, got %q", conn.cmds)
	}
	if login > pull {
		t.Fatalf("login (#%d) must run before pull (#%d): %q", login, pull, conn.cmds)
	}
	if got := conn.cmds[login]; got != "docker login 'ghcr.io' -u 'bot' --password-stdin 2>&1" {
		t.Fatalf("unexpected login command: %q", got)
	}
	if conn.stdin != testRegistryToken {
		t.Fatalf("token must be sent via stdin, got %q", conn.stdin)
	}
	for _, c := range conn.cmds {
		if strings.Contains(c, testRegistryToken) {
			t.Fatalf("token leaked into remote command: %q", c)
		}
	}

	d := lastDeployment(t, repo, appID)
	if d.Status != "success" {
		t.Fatalf("expected success, got %s: %s", d.Status, d.Log)
	}
	if strings.Contains(d.Log, testRegistryToken) {
		t.Fatalf("token leaked into deploy log: %q", d.Log)
	}
}

func TestDeployRegistryLoginFailureRedactsAndStops(t *testing.T) {
	conn := &recordingConn{
		pipeOut: "Error response from daemon: denied for " + testRegistryToken,
		pipeErr: errors.New("exec pipe: Process exited with status 1: " + testRegistryToken),
	}
	s, repo, appID := setupRegistryDeploy(t, true, conn)

	s.deployWithCommit(appID, "")

	if i := cmdIndex(conn.cmds, " pull "); i >= 0 {
		t.Fatalf("pull must not run after failed login: %q", conn.cmds)
	}
	d := lastDeployment(t, repo, appID)
	if d.Status != StatusFailed {
		t.Fatalf("expected failed deployment, got %s", d.Status)
	}
	if !strings.Contains(d.Log, "registry login failed") {
		t.Fatalf("expected 'registry login failed' in log, got %q", d.Log)
	}
	if strings.Contains(d.Log, testRegistryToken) {
		t.Fatalf("token leaked into deploy log: %q", d.Log)
	}
	if !strings.Contains(d.Log, "[REDACTED]") {
		t.Fatalf("expected redaction marker in log, got %q", d.Log)
	}
	if a, _ := repo.Get(appID); a.Status != StatusFailed {
		t.Fatalf("expected app status failed, got %s", a.Status)
	}
}

func TestDeployWithoutRegistryCredentialSkipsLogin(t *testing.T) {
	conn := &recordingConn{}
	s, repo, appID := setupRegistryDeploy(t, false, conn)

	s.deployWithCommit(appID, "")

	if i := cmdIndex(conn.cmds, "docker login"); i >= 0 || conn.pipes != 0 {
		t.Fatalf("login must not run without credential: %q", conn.cmds)
	}
	if cmdIndex(conn.cmds, " pull ") < 0 {
		t.Fatalf("expected pull command, got %q", conn.cmds)
	}
	if d := lastDeployment(t, repo, appID); d.Status != "success" {
		t.Fatalf("expected success, got %s: %s", d.Status, d.Log)
	}
}
