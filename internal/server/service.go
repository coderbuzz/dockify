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

	log.Printf("Installing Docker Compose plugin on %s...", server.Name)
	_, err = client.Exec(`docker compose version 2>/dev/null || (mkdir -p /usr/local/lib/docker/cli-plugins && curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose && chmod +x /usr/local/lib/docker/cli-plugins/docker-compose)`)
	if err != nil {
		log.Printf("Warning: failed to install docker compose plugin on %s: %v", server.Name, err)
	}

	log.Printf("Creating dockify network on %s...", server.Name)
	_, err = client.Exec("docker network inspect dockify >/dev/null 2>&1 || docker network create dockify")
	if err != nil {
		log.Printf("Warning: failed to create dockify network: %v", err)
	}

	caddyRunning, _ := client.Exec("docker ps -q --filter name=^/caddy$")
	baseConfig := `{"apps":{"http":{"servers":{"srv0":{"listen":[":80",":443"]}}}}}`
	if strings.TrimSpace(caddyRunning) == "" {
		log.Printf("Deploying Caddy on %s...", server.Name)
		caddyRun := fmt.Sprintf(`docker pull caddy:latest 2>/dev/null
mkdir -p /opt/dockify/caddy
echo '%s' > /opt/dockify/caddy/config.json
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
sleep 2
if docker exec caddy curl -sf -o /dev/null -X PATCH http://localhost:2019/config/metrics -H 'Content-Type: application/json' -d '{}' 2>/dev/null; then
  docker exec caddy curl -s http://localhost:2019/config/ > /opt/dockify/caddy/config.json
  echo "METRICS_ENABLED"
else
  echo "METRICS_UNAVAILABLE"
fi`, baseConfig)
		out, err := client.Exec(caddyRun)
		if err != nil {
			s.repo.UpdateStatus(id, StatusError)
			return fmt.Errorf("deploy caddy: %w", err)
		}
		if strings.Contains(out, "METRICS_ENABLED") {
			log.Printf("Worker %q: Caddy metrics enabled", server.Name)
		} else {
			log.Printf("Worker %q: Caddy metrics not available (older Caddy build), traffic stats disabled", server.Name)
		}
	} else {
		log.Printf("Caddy already running on %s, checking config...", server.Name)
		migrateCmd := `docker pull caddy:latest 2>/dev/null
mkdir -p /opt/dockify/caddy
if [ ! -f /opt/dockify/caddy/config.json ]; then
  docker exec caddy curl -s http://localhost:2019/config/ > /opt/dockify/caddy/config.json
fi
if docker exec caddy curl -sf -o /dev/null -X PATCH http://localhost:2019/config/metrics -H 'Content-Type: application/json' -d '{}' 2>/dev/null; then
  docker exec caddy curl -s http://localhost:2019/config/ > /opt/dockify/caddy/config.json
  docker rm -f caddy
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
  echo "METRICS_ENABLED"
else
  echo "METRICS_UNAVAILABLE"
fi`
		out, err := client.Exec(migrateCmd)
		if err != nil {
			log.Printf("Warning: caddy config migration failed for %s: %v", server.Name, err)
		}
		if strings.Contains(out, "METRICS_ENABLED") {
			log.Printf("Worker %q: Caddy metrics enabled (migrated)", server.Name)
		} else {
			log.Printf("Worker %q: Caddy metrics not available, traffic stats disabled", server.Name)
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

	s.repo.InsertStats(&ServerStats{
		ServerID:   id,
		CPUPercent: cpuUsage,
		RAMPercent: ramUsage,
		DiskPercent: diskUsage,
		CPUCores:   cpuCores,
		RAMMB:      ramMB,
		DiskGB:     diskGB,
	})

	return nil
}

func (s *Service) StartMonitor() {
	s.monitor = NewMonitor(s)
	go s.monitor.Run()
}

func (s *Service) GetStatsHistory(serverID int64, duration string) map[string]interface{} {
	now := time.Now().UTC()
	var since time.Time
	var bucketMins int

	switch duration {
	case "1h":
		since = now.Add(-1 * time.Hour)
		bucketMins = 1
	case "6h":
		since = now.Add(-6 * time.Hour)
		bucketMins = 5
	case "24h":
		since = now.Add(-24 * time.Hour)
		bucketMins = 10
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
		bucketMins = 60
	default:
		since = now.Add(-1 * time.Hour)
		bucketMins = 1
	}

	cpu, _ := s.repo.StatsHistory(serverID, since, bucketMins, "cpu")
	ram, _ := s.repo.StatsHistory(serverID, since, bucketMins, "ram")
	disk, _ := s.repo.StatsHistory(serverID, since, bucketMins, "disk")

	return map[string]interface{}{
		"cpu":  cpu,
		"ram":  ram,
		"disk": disk,
	}
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
	out, err := c.Exec("awk '/MemTotal/{printf \"%d\", $2/1024}' /proc/meminfo")
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
	out, err := c.Exec(`s=$(head -1 /proc/stat); sleep 1; e=$(head -1 /proc/stat); awk -v s="$s" -v e="$e" 'BEGIN{n=split(s,a); split(e,b); t1=a[2]+a[3]+a[4]+a[5]; t2=b[2]+b[3]+b[4]+b[5]; dt=t2-t1; di=b[5]-a[5]; if(dt>0) printf "%.1f", 100*(dt-di)/dt}'`)
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}

func parseRAMUsage(c ssh.Connector) (float64, error) {
	out, err := c.Exec("awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} END{printf \"%.1f\", 100*(t-a)/t}' /proc/meminfo")
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
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	pruneTicker := time.NewTicker(1 * time.Hour)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ticker.C:
			m.refreshAll()
		case <-pruneTicker.C:
			m.service.repo.PruneStats(time.Now().Add(-7 * 24 * time.Hour))
		}
	}
}

func (m *Monitor) refreshAll() {
	servers, err := m.service.List()
	if err != nil {
		log.Printf("Monitor: failed to list servers: %v", err)
		return
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
