package app

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/ssh"
)

type dockerStat struct {
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
	PIDs     string `json:"PIDs"`
	Name     string `json:"Name"`
}

func (s *Service) StartStatsCollector() {
	go s.statsLoop()
}

func (s *Service) statsLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	pruneTicker := time.NewTicker(1 * time.Hour)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.collectAllStats()
		case <-pruneTicker.C:
			s.pruneOldStats()
		}
	}
}

func (s *Service) collectAllStats() {
	apps, err := s.repo.List()
	if err != nil {
		log.Printf("Stats collector: failed to list apps: %v", err)
		return
	}

	serverApps := make(map[int64][]App)
	for _, a := range apps {
		if a.Status != StatusRunning || a.ServerID == 0 {
			continue
		}
		serverApps[a.ServerID] = append(serverApps[a.ServerID], a)
	}

	for serverID, appList := range serverApps {
		go func(sid int64, list []App) {
			s.collectServerStats(sid, list)
		}(serverID, appList)
	}
}

func (s *Service) collectServerStats(serverID int64, apps []App) {
	server, err := s.serverRepo.Get(serverID)
	if err != nil || server == nil {
		return
	}

	client, err := s.connFactory(server.Host, server.Port, server.User, server.SSHKey)
	if err != nil {
		log.Printf("Stats collector: SSH connect to %q failed: %v", server.Name, err)
		return
	}
	defer client.Close()

	s.collectContainerStats(client, serverID, apps)
	s.collectCaddyTraffic(client, serverID, apps)
}

func (s *Service) collectContainerStats(client ssh.Connector, serverID int64, apps []App) {
	for _, app := range apps {
		serviceName := app.ContainerServiceName()
		composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", app.ID)

		dc := detectDockerCompose(client)
		containerList, err := client.Exec(fmt.Sprintf("%s -f %s ps --format '{{.Names}}' 2>/dev/null", dc, composePath))
		if err != nil || strings.TrimSpace(containerList) == "" {
			containerList, err = client.Exec(fmt.Sprintf("%s -f %s ps -q 2>/dev/null", dc, composePath))
		}
		if err != nil || strings.TrimSpace(containerList) == "" {
			continue
		}

		containers := strings.Split(strings.TrimSpace(containerList), "\n")
		for _, cname := range containers {
			cname = strings.TrimSpace(cname)
			if cname == "" {
				continue
			}

			out, err := client.Exec(fmt.Sprintf("docker stats --no-stream --format '{{json .}}' %s 2>/dev/null", cname))
			if err != nil || strings.TrimSpace(out) == "" {
				continue
			}

			var stat dockerStat
			if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &stat); err != nil {
				log.Printf("Stats collector: failed to parse docker stats for %q: %v", cname, err)
				continue
			}

			_ = serviceName
			cs := parseDockerStat(stat, app.ID, serverID, cname)
			if cs != nil {
				if err := s.repo.InsertContainerStats(cs); err != nil {
					log.Printf("Stats collector: failed to insert stats for %q: %v", cname, err)
				}
			}
		}
	}
}

func parseDockerStat(stat dockerStat, appID, serverID int64, containerName string) *ContainerStats {
	cs := &ContainerStats{
		AppID:         appID,
		ServerID:      serverID,
		ContainerName: containerName,
	}

	cs.CPUPercent = parsePercent(stat.CPUPerc)
	cs.MemPercent = parsePercent(stat.MemPerc)
	cs.MemUsageBytes, cs.MemLimitBytes = parseMemUsage(stat.MemUsage)
	cs.NetIORxBytes, cs.NetIOTxBytes = parseNetIO(stat.NetIO)
	cs.BlockIORead, cs.BlockIOWrite = parseBlockIO(stat.BlockIO)
	if pids, err := strconv.Atoi(strings.TrimSpace(stat.PIDs)); err == nil {
		cs.PIDs = pids
	}

	return cs
}

func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseMemUsage(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0
	}

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "TiB"):
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "TiB")
	case strings.HasSuffix(s, "GiB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GiB")
	case strings.HasSuffix(s, "MiB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MiB")
	case strings.HasSuffix(s, "KiB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KiB")
	case strings.HasSuffix(s, "TB"):
		multiplier = 1000 * 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "TB")
	case strings.HasSuffix(s, "GB"):
		multiplier = 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1000 * 1000
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "kB"):
		multiplier = 1000
		s = strings.TrimSuffix(s, "kB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}

	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return int64(v * float64(multiplier))
}

func parseNetIO(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func parseBlockIO(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func (s *Service) collectCaddyTraffic(client ssh.Connector, serverID int64, apps []App) {
	metricsOut, err := client.Exec("docker exec caddy curl -s http://localhost:2019/metrics 2>/dev/null")
	if err != nil || strings.TrimSpace(metricsOut) == "" {
		return
	}

	domains := make(map[string]int64)
	for _, app := range apps {
		if app.Domain != "" {
			domains[app.Domain] = app.ID
		}
		routes, _ := s.repo.GetRoutes(app.ID)
		for _, r := range routes {
			domains[r.Domain] = app.ID
		}
	}

	if len(domains) == 0 {
		return
	}

	lines := strings.Split(metricsOut, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if !strings.Contains(line, "caddy_http_requests_total") {
			continue
		}

		host := extractLabel(line, "host")
		if host == "" {
			continue
		}

		appID, ok := domains[host]
		if !ok {
			continue
		}

		val := parseMetricValue(line)
		s2xx := parseMetricLine(lines, "caddy_http_requests_total", "status=\"2", host)
		s3xx := parseMetricLine(lines, "caddy_http_requests_total", "status=\"3", host)
		s4xx := parseMetricLine(lines, "caddy_http_requests_total", "status=\"4", host)
		s5xx := parseMetricLine(lines, "caddy_http_requests_total", "status=\"5", host)

		prev, err := s.repo.LatestRouteStats(appID)
		rps := float64(0)
		if err == nil && prev != nil {
			delta := val - prev.TotalRequests
			if delta > 0 {
				elapsed := time.Since(prev.CreatedAt).Seconds()
				if elapsed > 0 {
					rps = float64(delta) / elapsed
				}
			}
		}

		rs := &RouteStats{
			AppID:         appID,
			Domain:        host,
			TotalRequests: val,
			RequestsRPS:   rps,
			Status2xx:     s2xx,
			Status3xx:     s3xx,
			Status4xx:     s4xx,
			Status5xx:     s5xx,
		}
		if err := s.repo.InsertRouteStats(rs); err != nil {
			log.Printf("Stats collector: failed to insert route stats: %v", err)
		}
	}
}

func extractLabel(line, label string) string {
	idx := strings.Index(line, label+"=\"")
	if idx < 0 {
		return ""
	}
	start := idx + len(label) + 2
	end := strings.Index(line[start:], "\"")
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

func parseMetricValue(line string) int64 {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0
	}
	v, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	return v
}

func parseMetricLine(lines []string, metric, statusPrefix, host string) int64 {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, metric) && strings.Contains(line, statusPrefix) && strings.Contains(line, "host=\""+host+"\"") {
			return parseMetricValue(line)
		}
	}
	return 0
}

func (s *Service) pruneOldStats() {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if err := s.repo.PruneContainerStats(cutoff); err != nil {
		log.Printf("Stats collector: failed to prune container stats: %v", err)
	}
	if err := s.repo.PruneRouteStats(cutoff); err != nil {
		log.Printf("Stats collector: failed to prune route stats: %v", err)
	}
}

func detectDockerCompose(client ssh.Connector) string {
	out, _ := client.Exec("docker compose version 2>/dev/null")
	if strings.TrimSpace(out) != "" {
		return "docker compose"
	}
	return "docker-compose"
}

func (s *Service) GetStatsHistory(appID int64, duration string) map[string]interface{} {
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

	cpu, _ := s.repo.ContainerStatsCPUHistory(appID, since, bucketMins)
	mem, _ := s.repo.ContainerStatsMemHistory(appID, since, bucketMins)
	net, _ := s.repo.ContainerStatsNetHistory(appID, since, bucketMins)

	labels := make([]string, len(cpu))
	for i, p := range cpu {
		labels[i] = p.Time
	}

	return map[string]interface{}{
		"cpu":    cpu,
		"memory": mem,
		"network": net,
		"labels": labels,
	}
}

func (s *Service) GetTrafficHistory(appID int64, duration string) map[string]interface{} {
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

	rps, _ := s.repo.RouteStatsHistory(appID, since, bucketMins)

	labels := make([]string, len(rps))
	for i, p := range rps {
		labels[i] = p.Time
	}

	return map[string]interface{}{
		"rps":    rps,
		"labels": labels,
	}
}
