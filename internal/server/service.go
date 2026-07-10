package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/ssh"
)

const (
	StatusPending  = "pending"
	StatusOnline   = "online"
	StatusOffline  = "offline"
	StatusError    = "error"
)

type Service struct {
	repo        *Repository
	monitor     *Monitor
	connFactory ssh.Factory
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo:        repo,
		connFactory: ssh.RealFactory(),
	}
}

func (s *Service) SetConnFactory(f ssh.Factory) {
	s.connFactory = f
}

func (s *Service) List() ([]Server, error) {
	return s.repo.List()
}

func (s *Service) Get(id int64) (*Server, error) {
	return s.repo.Get(id)
}

func (s *Service) Create(server *Server) error {
	if server.Port == 0 {
		server.Port = 22
	}
	if server.User == "" {
		server.User = "root"
	}
	return s.repo.Create(server)
}

func (s *Service) Update(server *Server) error {
	return s.repo.Update(server)
}

func (s *Service) Delete(id int64) error {
	return s.repo.Delete(id)
}

func (s *Service) TestConnection(id int64) error {
	server, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if server == nil {
		return fmt.Errorf("server not found")
	}

	client, err := s.connFactory(server.Host, server.Port, server.User, server.SSHKey)
	if err != nil {
		s.repo.UpdateStatus(id, StatusOffline)
		return fmt.Errorf("SSH connect failed: %w", err)
	}
	defer client.Close()

	out, err := client.Exec("uptime")
	if err != nil {
		s.repo.UpdateStatus(id, StatusOffline)
		return fmt.Errorf("SSH exec failed: %w", err)
	}

	s.repo.UpdateStatus(id, StatusOnline)
	log.Printf("Server %q is online: %s", server.Name, strings.TrimSpace(out))
	return nil
}

func (s *Service) InitWorker(id int64) error {
	server, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if server == nil {
		return fmt.Errorf("server not found")
	}

	s.repo.UpdateStatus(id, "initializing")

	client, err := s.connFactory(server.Host, server.Port, server.User, server.SSHKey)
	if err != nil {
		s.repo.UpdateStatus(id, StatusError)
		return fmt.Errorf("SSH connect: %w", err)
	}
	defer client.Close()

	log.Printf("Installing Docker on %s...", server.Name)
	_, err = client.Exec("command -v docker || curl -fsSL https://get.docker.com | sh")
	if err != nil {
		s.repo.UpdateStatus(id, StatusError)
		return fmt.Errorf("install docker: %w", err)
	}

	log.Printf("Creating dockify network on %s...", server.Name)
	_, err = client.Exec("docker network inspect dockify >/dev/null 2>&1 || docker network create dockify")
	if err != nil {
		log.Printf("Warning: failed to create dockify network: %v", err)
	}

	caddyRunning, _ := client.Exec("docker ps -q --filter name=^/caddy$")
	if strings.TrimSpace(caddyRunning) == "" {
		log.Printf("Deploying Caddy on %s...", server.Name)
		caddyRun := `mkdir -p /opt/dockify/caddy
if [ ! -f /opt/dockify/caddy/config.json ]; then
  echo '{"apps":{"http":{"servers":{"srv0":{"listen":[":80",":443"]}}}}}' > /opt/dockify/caddy/config.json
fi
docker rm -f caddy 2>/dev/null
docker run -d \
  --name caddy \
  --network dockify \
  -p 80:80 \
  -p 443:443 \
  -p 127.0.0.1:2019:2019 \
  -v caddy_data:/data \
  -v /opt/dockify/caddy/config.json:/data/config.json \
  --restart unless-stopped \
  caddy:latest caddy run --config /data/config.json`
		_, err = client.Exec(caddyRun)
		if err != nil {
			s.repo.UpdateStatus(id, StatusError)
			return fmt.Errorf("deploy caddy: %w", err)
		}
	} else {
		log.Printf("Caddy already running on %s, checking config...", server.Name)
		migrateCmd := `if [ ! -f /opt/dockify/caddy/config.json ]; then
  mkdir -p /opt/dockify/caddy
  docker exec caddy curl -s http://localhost:2019/config/ > /opt/dockify/caddy/config.json
  docker rm -f caddy 2>/dev/null
  docker run -d \
    --name caddy \
    --network dockify \
    -p 80:80 \
    -p 443:443 \
    -p 127.0.0.1:2019:2019 \
    -v caddy_data:/data \
    -v /opt/dockify/caddy/config.json:/data/config.json \
    --restart unless-stopped \
    caddy:latest caddy run --config /data/config.json
fi`
		_, err = client.Exec(migrateCmd)
		if err != nil {
			log.Printf("Warning: caddy config migration failed for %s: %v", server.Name, err)
		}
	}

	s.repo.UpdateStatus(id, StatusOnline)
	log.Printf("Worker %q initialized successfully", server.Name)

	go s.RefreshResources(id)
	return nil
}

func (s *Service) RefreshResources(id int64) error {
	server, err := s.repo.Get(id)
	if err != nil || server == nil {
		return err
	}

	client, err := s.connFactory(server.Host, server.Port, server.User, server.SSHKey)
	if err != nil {
		s.repo.UpdateStatus(id, StatusOffline)
		return err
	}
	defer client.Close()

	cpuCores, _ := parseCPUCount(client)
	ramMB, _ := parseRAM(client)
	diskGB, _ := parseDisk(client)
	cpuUsage, _ := parseCPUUsage(client)
	ramUsage, _ := parseRAMUsage(client)
	diskUsage, _ := parseDiskUsage(client)

	s.repo.UpdateResources(id, cpuCores, ramMB, diskGB, cpuUsage, ramUsage, diskUsage)
	s.repo.UpdateStatus(id, StatusOnline)

	return nil
}

func (s *Service) StartMonitor() {
	s.monitor = NewMonitor(s)
	go s.monitor.Run()
}

func parseCPUCount(c ssh.Connector) (int, error) {
	out, err := c.Exec("nproc")
	if err != nil {
		return 0, err
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count, nil
}

func parseRAM(c ssh.Connector) (int, error) {
	out, err := c.Exec("free -m | awk '/Mem:/ {print $2}'")
	if err != nil {
		return 0, err
	}
	var mb int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &mb)
	return mb, nil
}

func parseDisk(c ssh.Connector) (int, error) {
	out, err := c.Exec("df -BG / | awk 'NR==2 {gsub(/G/,\"\"); print $2}'")
	if err != nil {
		return 0, err
	}
	var gb int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &gb)
	return gb, nil
}

func parseCPUUsage(c ssh.Connector) (float64, error) {
	out, err := c.Exec("awk '/^cpu / {printf \"%.1f\", ($2+$4)*100/($2+$4+$5)}' /proc/stat")
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}

func parseRAMUsage(c ssh.Connector) (float64, error) {
	out, err := c.Exec("free -m | awk '/Mem:/ {printf \"%.1f\", $3/$2 * 100.0}'")
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}

func parseDiskUsage(c ssh.Connector) (float64, error) {
	out, err := c.Exec("df -BG / | awk 'NR==2 {gsub(/%/,\"\"); print $5}'")
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}

type Monitor struct {
	service *Service
}

func NewMonitor(svc *Service) *Monitor {
	return &Monitor{service: svc}
}

func (m *Monitor) Run() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		servers, err := m.service.List()
		if err != nil {
			log.Printf("Monitor: failed to list servers: %v", err)
			continue
		}

		for _, server := range servers {
			if server.Status != StatusOnline && server.Status != StatusOffline {
				continue
			}

			go func(id int64, name string) {
				if err := m.service.RefreshResources(id); err != nil {
					log.Printf("Monitor: refresh %q failed: %v", name, err)
				}
			}(server.ID, server.Name)
		}
	}
}
