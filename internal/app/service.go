package app

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/caddy"
	"github.com/coderbuzz/dockify/internal/cloudflare"
	"github.com/coderbuzz/dockify/internal/scheduler"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
)

const (
	StatusCreated   = "created"
	StatusDeploying = "deploying"
	StatusRunning   = "running"
	StatusStopped   = "stopped"
	StatusFailed    = "failed"
)

type Service struct {
	repo             *Repository
	serverRepo       *server.Repository
	cf               *cloudflare.Client
	scheduler        *scheduler.Scheduler
}

func NewService(repo *Repository, serverRepo *server.Repository, cf *cloudflare.Client, sch *scheduler.Scheduler) *Service {
	return &Service{repo: repo, serverRepo: serverRepo, cf: cf, scheduler: sch}
}

func (s *Service) List() ([]App, error) {
	return s.repo.List()
}

func (s *Service) Get(id int64) (*App, error) {
	return s.repo.Get(id)
}

func (s *Service) Create(app *App) error {
	return s.repo.Create(app)
}

func (s *Service) ListSecrets(appID int64) ([]AppSecret, error) {
	return s.repo.ListSecrets(appID)
}

func (s *Service) SetSecret(appID int64, key, value string) error {
	return s.repo.SetSecret(appID, key, value)
}

func (s *Service) DeleteSecret(appID int64, key string) error {
	return s.repo.DeleteSecret(appID, key)
}

func (s *Service) PickServerID() (int64, error) {
	if s.scheduler == nil {
		return 0, fmt.Errorf("scheduler not available")
	}
	svr, err := s.scheduler.PickServer()
	if err != nil {
		return 0, err
	}
	return svr.ID, nil
}

func (s *Service) Delete(id int64) error {
	return s.repo.Delete(id)
}

func (s *Service) Deploy(id int64) {
	s.deployWithCommit(id, "")
}

func (s *Service) DeployByGit(repo, branch, commitSHA string) {
	apps, err := s.repo.FindAllByGitRepo(repo, branch)
	if err != nil || len(apps) == 0 {
		log.Printf("Webhook: no app found for %s@%s", repo, branch)
		return
	}
	for _, app := range apps {
		log.Printf("Webhook triggered deploy for %q (commit %s)", app.Name, commitSHA)
		go s.deployWithCommit(app.ID, commitSHA)
	}
}

func (s *Service) deployWithCommit(id int64, commitSHA string) {
	s.repo.UpdateStatus(id, StatusDeploying)

	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		s.recordDeployment(id, 0, StatusFailed, "app not found", commitSHA, "")
		return
	}

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		s.recordDeployment(id, app.ServerID, StatusFailed, "server not found", commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	client, err := ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		s.recordDeployment(id, svr.ID, StatusFailed, fmt.Sprintf("SSH connect: %v", err), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}
	defer client.Close()

	composeCmd := dockerComposeCmd(client)

	remoteDir := fmt.Sprintf("/opt/dockify/apps/%s", app.Name)
	composePath := fmt.Sprintf("%s/docker-compose.yml", remoteDir)

	log.Printf("Deploying %q to %s...", app.Name, svr.Name)

	// Tulis .env file dari app secrets
	secrets, _ := s.repo.ListSecrets(id)
	if len(secrets) > 0 {
		envPath := fmt.Sprintf("%s/.env", remoteDir)
		var envLines []string
		for _, sec := range secrets {
			envLines = append(envLines, sec.Key+"="+sec.Value)
		}
		if err := client.WriteFile(envPath, strings.Join(envLines, "\n")+"\n", 0644); err != nil {
			log.Printf("Warning: write .env: %v", err)
		}
	}

	if err := client.WriteFile(composePath, app.Compose, 0644); err != nil {
		s.recordDeployment(id, svr.ID, StatusFailed, fmt.Sprintf("write compose: %v", err), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	logs := []string{}

	client.Exec("docker network inspect dockify >/dev/null 2>&1 || docker network create dockify")

	if out, err := client.Exec(fmt.Sprintf("%s -f %s up -d --force-recreate 2>&1", composeCmd, composePath)); err != nil {
		logs = append(logs, fmt.Sprintf("compose up: %v", err))
		logs = append(logs, out)
		s.recordDeployment(id, svr.ID, StatusFailed, strings.Join(logs, "\n"), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	if app.Domain != "" {
		target := fmt.Sprintf("%s:%d", getServiceName(app.Compose), app.Port)
		caddyClient := caddy.NewClient(client)
		var caddyErr error
		if app.AuthUser != "" && app.AuthPass != "" {
			caddyErr = caddyClient.AddRouteWithAuth(app.Domain, target, app.AuthUser, app.AuthPass)
			log.Printf("Caddy route (with auth) added: %s -> %s", app.Domain, target)
		} else {
			caddyErr = caddyClient.AddRoute(app.Domain, target)
			log.Printf("Caddy route added: %s -> %s", app.Domain, target)
		}
		if caddyErr != nil {
			log.Printf("Warning: caddy route injection failed for %q: %v", app.Name, caddyErr)
			logs = append(logs, fmt.Sprintf("caddy: %v", caddyErr))
		}
		s.repo.SaveRoute(&Route{
			AppID:    id,
			ServerID: svr.ID,
			Domain:   app.Domain,
			Target:   target,
		})

		if s.cf != nil && s.cf.Enabled() {
			record, err := s.cf.CreateRecord(app.Domain, svr.Host, false)
			if err != nil {
				log.Printf("Warning: Cloudflare DNS failed for %q: %v", app.Name, err)
				logs = append(logs, fmt.Sprintf("dns: %v", err))
			} else {
				log.Printf("DNS record created: %s -> %s", record.Name, record.Content)
				s.repo.SaveDNSRecord(id, svr.ID, record.ZoneID, record.ID, record.Name, "A", record.Content, record.Proxied)
			}
		}
	} else {
		log.Printf("App %q deployed as internal service (no domain, no Caddy route)", app.Name)
	}

	s.repo.UpdateStatus(id, StatusRunning)
	s.recordDeployment(id, svr.ID, "success", strings.Join(logs, "\n"), commitSHA, app.Compose)
	log.Printf("App %q deployed successfully", app.Name)
}

func (s *Service) Rollback(id int64) error {
	last, err := s.repo.GetLastSuccessfulDeployment(id)
	if err != nil || last == nil {
		return fmt.Errorf("no successful deployment to rollback to")
	}

	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return fmt.Errorf("app not found")
	}

	if last.ComposeSnapshot == "" {
		return fmt.Errorf("no compose snapshot available for rollback")
	}

	app.Compose = last.ComposeSnapshot
	if err := s.repo.Update(app); err != nil {
		return fmt.Errorf("update app compose: %w", err)
	}

	log.Printf("Rolling back %q to deployment #%d", app.Name, last.ID)
	go s.deployWithCommit(id, "")
	return nil
}

func (s *Service) FetchLogs(id int64, tail int) (string, error) {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return "", fmt.Errorf("app not found")
	}

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		return "", fmt.Errorf("server not found")
	}

	client, err := ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		return "", fmt.Errorf("SSH connect: %w", err)
	}
	defer client.Close()

	composePath := fmt.Sprintf("/opt/dockify/apps/%s/docker-compose.yml", app.Name)
	dc := dockerComposeCmd(client)
	out, err := client.Exec(fmt.Sprintf("%s -f %s logs --tail=%d 2>&1", dc, composePath, tail))
	if err != nil {
		return "", err
	}
	return out, nil
}

func (s *Service) Undeploy(id int64) error {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return fmt.Errorf("app not found")
	}

	if s.cf != nil && s.cf.Enabled() {
		dnsRecords, err := s.repo.GetDNSRecords(id)
		if err == nil {
			for _, rec := range dnsRecords {
				if delErr := s.cf.DeleteRecord(rec.RecordID); delErr != nil {
					log.Printf("Warning: failed to delete DNS record %s: %v", rec.RecordID, delErr)
				}
				s.repo.DeleteDNSRecord(rec.ID)
			}
		}
	}

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		log.Printf("Undeploy %q: server not found, cleaning up DB", app.Name)
		s.repo.DeleteDNSRecords(app.ID)
		s.repo.DeleteRoutes(app.ID)
		s.repo.DeleteDeployments(app.ID)
		s.repo.Delete(id)
		return nil
	}

	client, err := ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		log.Printf("Undeploy %q: SSH connect failed, cleaning up DB: %v", app.Name, err)
		s.repo.DeleteDNSRecords(app.ID)
		s.repo.DeleteRoutes(app.ID)
		s.repo.DeleteDeployments(app.ID)
		s.repo.Delete(id)
		return nil
	}
	defer client.Close()

	dc := dockerComposeCmd(client)

	remoteDir := fmt.Sprintf("/opt/dockify/apps/%s", app.Name)
	composePath := fmt.Sprintf("%s/docker-compose.yml", remoteDir)

	log.Printf("Undeploying %q from %s...", app.Name, svr.Name)

	client.Exec(fmt.Sprintf("%s -f %s down 2>&1 || true", dc, composePath))

	if app.Domain != "" {
		caddyClient := caddy.NewClient(client)
		caddyClient.RemoveRoute(app.Domain)
	}

	s.repo.DeleteRoutes(app.ID)
	s.repo.DeleteDNSRecords(app.ID)
	s.repo.DeleteDeployments(app.ID)

	client.Exec(fmt.Sprintf("rm -rf %s", remoteDir))

	s.repo.Delete(id)
	log.Printf("App %q undeployed", app.Name)
	return nil
}

func (s *Service) Redeploy(id int64) {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return
	}
	log.Printf("Redeploying %q...", app.Name)
	s.deployWithCommit(id, "")
}

func (s *Service) DashboardStats() *DashboardStats {
	apps, _ := s.repo.List()
	servers, _ := s.serverRepo.List()

	stats := &DashboardStats{
		TotalApps:    len(apps),
		TotalServers: len(servers),
	}

	running := 0
	online := 0
	for _, a := range apps {
		if a.Status == StatusRunning {
			running++
		}
	}
	for _, svr := range servers {
		if svr.Status == "online" {
			online++
		}
	}
	stats.RunningApps = running
	stats.OnlineServers = online

	return stats
}

type DashboardStats struct {
	TotalApps     int
	RunningApps   int
	TotalServers  int
	OnlineServers int
}

func (s *Service) recordDeployment(appID, serverID int64, status, logMsg, commitSHA, composeSnapshot string) {
	d := &Deployment{
		AppID:           appID,
		ServerID:        serverID,
		Status:          status,
		Log:             logMsg,
		CommitSHA:       commitSHA,
		ComposeSnapshot: composeSnapshot,
	}
	if err := s.repo.AddDeployment(d); err != nil {
		log.Printf("Failed to record deployment: %v", err)
	}
}

func (s *Service) ListDeployments(appID int64) ([]Deployment, error) {
	return s.repo.ListDeployments(appID)
}

func (s *Service) GetDeployment(id int64) (*Deployment, error) {
	return s.repo.GetDeployment(id)
}

	var _ = time.Now

func dockerComposeCmd(c *ssh.Client) string {
	out, err := c.Exec("command -v docker-compose 2>/dev/null || echo docker compose")
	if err != nil {
		return "docker compose"
	}
	return strings.TrimSpace(out)
}
