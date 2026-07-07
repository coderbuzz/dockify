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
	connFactory      ssh.Factory
}

func (s *Service) SetConnFactory(f ssh.Factory) {
	s.connFactory = f
}

func NewService(repo *Repository, serverRepo *server.Repository, cf *cloudflare.Client, sch *scheduler.Scheduler) *Service {
	return &Service{repo: repo, serverRepo: serverRepo, cf: cf, scheduler: sch, connFactory: ssh.RealFactory()}
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

func (s *Service) Update(app *App) error {
	return s.repo.Update(app)
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

func (s *Service) DeleteSecrets(appID int64) error {
	return s.repo.DeleteSecrets(appID)
}

func (s *Service) ListFiles(appID int64) ([]AppFile, error) {
	return s.repo.ListFiles(appID)
}

func (s *Service) SetFile(appID int64, path, content string) error {
	return s.repo.SetFile(appID, path, content)
}

func (s *Service) DeleteFile(appID int64, path string) error {
	return s.repo.DeleteFile(appID, path)
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

func (s *Service) SaveRoute(route *Route) error {
	return s.repo.SaveRoute(route)
}

func (s *Service) GetRoutes(appID int64) ([]Route, error) {
	return s.repo.GetRoutes(appID)
}

func (s *Service) DeleteRouteByDomain(appID int64, domain string) error {
	return s.repo.DeleteRouteByDomain(appID, domain)
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

func (s *Service) deployWithCommit(id int64, commitSHA string, removedDomains ...string) {
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

	client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		s.recordDeployment(id, svr.ID, StatusFailed, fmt.Sprintf("SSH connect: %v", err), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}
	defer client.Close()

	composeCmd := DockerComposeCmd(client)

	remoteDir := fmt.Sprintf("/opt/dockify/apps/app-%d", app.ID)
	composePath := fmt.Sprintf("%s/docker-compose.yml", remoteDir)

	log.Printf("Deploying %q to %s...", app.Name, svr.Name)

	secrets, _ := s.repo.ListSecrets(id)
	if len(secrets) > 0 {
		envPath := fmt.Sprintf("%s/.env", remoteDir)
		var envLines []string
		for _, sec := range secrets {
			envLines = append(envLines, sec.Key+"="+strings.ReplaceAll(sec.Value, "$", "$$"))
		}
		if err := client.WriteFile(envPath, strings.Join(envLines, "\n")+"\n", 0644); err != nil {
			log.Printf("Warning: write .env: %v", err)
		}
	}

	files, _ := s.repo.ListFiles(id)
	for _, f := range files {
		filePath := fmt.Sprintf("%s/%s", remoteDir, f.Path)
		if err := client.WriteFile(filePath, f.Content, 0644); err != nil {
			log.Printf("Warning: write config file %s: %v", f.Path, err)
		}
	}

	composeContent := ensureDockifyNetwork(app.Compose)

	if app.ComposeMode == "simple" {
		newName := sanitizeAppName(app.Name)
		composeContent = renameFirstService(composeContent, newName)
		log.Printf("Renamed first service to %q (compose_mode=simple)", newName)
	}

	if err := client.WriteFile(composePath, composeContent, 0644); err != nil {
		s.recordDeployment(id, svr.ID, StatusFailed, fmt.Sprintf("write compose: %v", err), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	logs := []string{}

	client.Exec("docker network inspect dockify >/dev/null 2>&1 || docker network create dockify")

	if pullOut, pullErr := client.Exec(fmt.Sprintf("%s -f %s pull 2>&1", composeCmd, composePath)); pullErr != nil {
		logs = append(logs, fmt.Sprintf("compose pull failed: %v\n%s", pullErr, pullOut))
		s.recordDeployment(id, svr.ID, StatusFailed, strings.Join(logs, "\n"), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	if out, err := client.Exec(fmt.Sprintf("%s -f %s up -d --remove-orphans 2>&1", composeCmd, composePath)); err != nil {
		logs = append(logs, fmt.Sprintf("compose up: %v", err))
		logs = append(logs, out)
		s.recordDeployment(id, svr.ID, StatusFailed, strings.Join(logs, "\n"), commitSHA, app.Compose)
		s.repo.UpdateStatus(id, StatusFailed)
		return
	}

	if _, err := client.Exec("docker image prune -f 2>&1"); err != nil {
		log.Printf("Warning: image prune failed: %v (non-fatal)", err)
	}

	for _, domain := range removedDomains {
		caddyClient := caddy.NewClient(client)
		if err := caddyClient.RemoveRoute(domain); err != nil {
			log.Printf("Warning: remove Caddy route %s: %v", domain, err)
			logs = append(logs, fmt.Sprintf("caddy/cleanup:%s: %v", domain, err))
		} else {
			log.Printf("Caddy route removed: %s", domain)
		}
	}

	if app.Domain != "" {
		routes, _ := s.repo.GetRoutes(app.ID)
		for _, r := range routes {
			s.setupRouteAndDNSForDomain(r, app, svr, client, composeContent, &logs)
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

	client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		return "", fmt.Errorf("SSH connect: %w", err)
	}
	defer client.Close()

	composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", app.ID)
	dc := DockerComposeCmd(client)
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

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		log.Printf("Undeploy %q: server not found, cleaning up DB", app.Name)
		s.repo.DeleteRoutes(app.ID)
		s.repo.DeleteDeployments(app.ID)
		s.repo.Delete(id)
		return nil
	}

	client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		log.Printf("Undeploy %q: SSH connect failed, cleaning up DB: %v", app.Name, err)
		s.repo.DeleteRoutes(app.ID)
		s.repo.DeleteDeployments(app.ID)
		s.repo.Delete(id)
		return nil
	}
	defer client.Close()

	dc := DockerComposeCmd(client)

	remoteDir := fmt.Sprintf("/opt/dockify/apps/app-%d", app.ID)
	composePath := fmt.Sprintf("%s/docker-compose.yml", remoteDir)

	log.Printf("Undeploying %q from %s...", app.Name, svr.Name)

	client.Exec(fmt.Sprintf("%s -f %s down 2>&1 || true", dc, composePath))
	client.Exec("docker image prune -af 2>&1 || true")
	client.Exec("docker builder prune -af 2>&1 || true")

	routes, _ := s.repo.GetRoutes(app.ID)
	for _, r := range routes {
		caddyClient := caddy.NewClient(client)
		caddyClient.RemoveRoute(r.Domain)
	}
	s.repo.DeleteRoutes(app.ID)
	s.repo.DeleteDeployments(app.ID)

	client.Exec(fmt.Sprintf("rm -rf %s", remoteDir))

	s.repo.Delete(id)
	log.Printf("App %q undeployed", app.Name)
	return nil
}

func (s *Service) setupRouteAndDNSForDomain(route Route, app *App, svr *server.Server, client ssh.Connector, composeContent string, logs *[]string) {
	target := fmt.Sprintf("%s:%d", getServiceName(composeContent), app.Port)
	caddyClient := caddy.NewClient(client)

	var caddyErr error
	if app.AuthUser != "" && app.AuthPass != "" {
		caddyErr = caddyClient.AddRouteWithAuth(route.Domain, target, app.AuthUser, app.AuthPass)
		log.Printf("Caddy route (with auth) added: %s -> %s", route.Domain, target)
	} else {
		caddyErr = caddyClient.AddRoute(route.Domain, target)
		log.Printf("Caddy route added: %s -> %s", route.Domain, target)
	}
	if caddyErr != nil {
		log.Printf("Warning: caddy route injection failed for %s: %v", route.Domain, caddyErr)
		if logs != nil {
			*logs = append(*logs, fmt.Sprintf("caddy/%s: %v", route.Domain, caddyErr))
		}
	}

	s.repo.UpdateRouteTarget(route.ID, target)

	if s.cf != nil && s.cf.Enabled() {
		records, err := s.cf.ListRecords(route.Domain)
		if err != nil {
			log.Printf("Warning: Cloudflare DNS lookup failed for %s: %v", route.Domain, err)
			return
		}
		var existing *cloudflare.DNSRecord
		for _, r := range records {
			if r.Name == route.Domain && r.Type == "A" {
				existing = &r
				break
			}
		}

		if existing != nil {
			if existing.Content != svr.Host {
				log.Printf("DNS A record IP mismatch for %s: current=%s, expected=%s, updating...", route.Domain, existing.Content, svr.Host)
				record, err := s.cf.UpdateRecord(existing.ID, route.Domain, svr.Host, existing.Proxied)
				if err != nil {
					log.Printf("Warning: Cloudflare DNS update failed for %s: %v", route.Domain, err)
					if logs != nil {
						*logs = append(*logs, fmt.Sprintf("dns/%s: %v", route.Domain, err))
					}
				} else {
					log.Printf("DNS A record updated: %s -> %s", record.Name, record.Content)
					_ = record
				}
			} else {
				log.Printf("DNS A record already exists for %s, IP matches (IP: %s, proxied: %v)", route.Domain, existing.Content, existing.Proxied)
			}
		} else {
			record, err := s.cf.CreateRecord(route.Domain, svr.Host, true)
			if err != nil {
				log.Printf("Warning: Cloudflare DNS failed for %s: %v", route.Domain, err)
				if logs != nil {
					*logs = append(*logs, fmt.Sprintf("dns/%s: %v", route.Domain, err))
				}
			} else {
				log.Printf("DNS A record created: %s -> %s", record.Name, record.Content)
				s.repo.SaveDNSRecord(app.ID, svr.ID, record.ZoneID, record.ID, record.Name, "A", record.Content, record.Proxied)
			}
		}
	}
}

func (s *Service) Redeploy(id int64, removedDomains ...string) {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return
	}
	log.Printf("Redeploying %q...", app.Name)
	s.deployWithCommit(id, "", removedDomains...)
}

func (s *Service) Stop(id int64) error {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return fmt.Errorf("app not found")
	}

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		return fmt.Errorf("server not found")
	}

	client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		return fmt.Errorf("SSH connect: %w", err)
	}
	defer client.Close()

	dc := DockerComposeCmd(client)
	composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", app.ID)
	log.Printf("Stopping %q on %s...", app.Name, svr.Name)

	if out, err := client.Exec(fmt.Sprintf("%s -f %s stop 2>&1", dc, composePath)); err != nil {
		return fmt.Errorf("compose stop: %w\n%s", err, out)
	}

	s.repo.UpdateStatus(id, StatusStopped)
	log.Printf("App %q stopped", app.Name)
	return nil
}

func (s *Service) Start(id int64) error {
	app, err := s.repo.Get(id)
	if err != nil || app == nil {
		return fmt.Errorf("app not found")
	}

	svr, err := s.serverRepo.Get(app.ServerID)
	if err != nil || svr == nil {
		return fmt.Errorf("server not found")
	}

	client, err := s.connFactory(svr.Host, svr.Port, svr.User, svr.SSHKey)
	if err != nil {
		return fmt.Errorf("SSH connect: %w", err)
	}
	defer client.Close()

	dc := DockerComposeCmd(client)
	composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", app.ID)
	log.Printf("Starting %q on %s...", app.Name, svr.Name)

	if out, err := client.Exec(fmt.Sprintf("%s -f %s start 2>&1", dc, composePath)); err != nil {
		return fmt.Errorf("compose start: %w\n%s", err, out)
	}

	s.repo.UpdateStatus(id, StatusRunning)
	log.Printf("App %q started", app.Name)
	return nil
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

func DockerComposeCmd(c ssh.Connector) string {
	out, err := c.Exec("command -v docker-compose 2>/dev/null || echo docker compose")
	if err != nil {
		return "docker compose"
	}
	return strings.TrimSpace(out)
}
