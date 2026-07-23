package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
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
	ticker := time.NewTicker(10 * time.Second)
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

	s.collectAllContainerStats(client, serverID, apps)
	s.collectCaddyTraffic(client, serverID, apps)
}

// collectAllContainerStats collects stats for every container on a server in
// just two SSH round-trips:
//  1. `docker ps` lists running containers with their compose project label
//     (com.docker.compose.project = app-<id>), mapping container -> appID.
//  2. a single `docker stats --no-stream` for all those containers, returning
//     one JSON line per container.
// This replaces the previous per-app/per-container `docker stats` calls, which
// spawned one SSH session per container every tick.
func (s *Service) collectAllContainerStats(client ssh.Connector, serverID int64, apps []App) {
	appByID := make(map[int64]App, len(apps))
	for _, a := range apps {
		appByID[a.ID] = a
	}

	// containerName -> appID
	containerApp := make(map[string]int64)

	psOut, err := client.Exec(`docker ps --format '{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null`)
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(psOut), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 2)
			if len(parts) != 2 {
				continue
			}
			cname := strings.TrimSpace(parts[0])
			project := strings.TrimSpace(parts[1])
			if appID := parseComposeProjectAppID(project); appID > 0 {
				if _, ok := appByID[appID]; ok {
					containerApp[cname] = appID
				}
			}
		}
	}

	// Fallback: per-app discovery for containers whose project label is missing.
	if len(containerApp) == 0 {
		for _, app := range apps {
			containers, err := listAppContainers(client, app)
			if err != nil {
				continue
			}
			for _, c := range containers {
				containerApp[c] = app.ID
			}
		}
	}

	if len(containerApp) == 0 {
		return
	}

	names := make([]string, 0, len(containerApp))
	for c := range containerApp {
		names = append(names, c)
	}
	sort.Strings(names)

	out, err := client.Exec(fmt.Sprintf("docker stats --no-stream --format '{{json .}}' %s 2>/dev/null", strings.Join(names, " ")))
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var stat dockerStat
		if err := json.Unmarshal([]byte(line), &stat); err != nil {
			log.Printf("Stats collector: failed to parse docker stats for %q: %v", stat.Name, err)
			continue
		}
		appID, ok := containerApp[stat.Name]
		if !ok {
			continue
		}
		cs := parseDockerStat(stat, appID, serverID, stat.Name)
		if cs != nil {
			if err := s.repo.InsertContainerStats(cs); err != nil {
				log.Printf("Stats collector: failed to insert stats for %q: %v", stat.Name, err)
			}
		}
	}
}

// parseComposeProjectAppID extracts the numeric app id from a compose project
// name of the form "app-<id>" (the directory name Dockify uses for each app's
// compose file). Returns 0 if it cannot parse.
func parseComposeProjectAppID(project string) int64 {
	project = strings.TrimSpace(project)
	if !strings.HasPrefix(project, "app-") {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(project, "app-"), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// listAppContainers returns the running container names that belong to an app's
// compose project (e.g. app-<id>-<service>-<n>). It reuses the same discovery
// logic as the historical collector.
func listAppContainers(client ssh.Connector, app App) ([]string, error) {
	composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", app.ID)
	dc := detectDockerCompose(client)

	out, err := client.Exec(fmt.Sprintf("%s -f %s ps --format '{{.Names}}' 2>/dev/null", dc, composePath))
	if err != nil || strings.TrimSpace(out) == "" {
		out, err = client.Exec(fmt.Sprintf("%s -f %s ps -q 2>/dev/null", dc, composePath))
		if err != nil || strings.TrimSpace(out) == "" {
			return nil, err
		}
	}

	var containers []string
	for _, c := range strings.Split(strings.TrimSpace(out), "\n") {
		c = strings.TrimSpace(c)
		if c != "" {
			containers = append(containers, c)
		}
	}
	return containers, nil
}

// sumStats aggregates (sums) a set of per-container stats into a single
// ContainerStats representing the total resource usage of an app.
func sumStats(stats []*ContainerStats) *ContainerStats {
	if len(stats) == 0 {
		return nil
	}
	var sum ContainerStats
	for _, cs := range stats {
		sum.AppID = cs.AppID
		sum.ServerID = cs.ServerID
		sum.CPUPercent += cs.CPUPercent
		sum.MemUsageBytes += cs.MemUsageBytes
		sum.MemLimitBytes += cs.MemLimitBytes
		sum.MemPercent += cs.MemPercent
		sum.NetIORxBytes += cs.NetIORxBytes
		sum.NetIOTxBytes += cs.NetIOTxBytes
		sum.BlockIORead += cs.BlockIORead
		sum.BlockIOWrite += cs.BlockIOWrite
		sum.PIDs += cs.PIDs
	}
	return &sum
}

// LiveSnapshot returns a single aggregated snapshot of an app's containers
// using a one-shot `docker stats --no-stream` call. Used by the dev-mock live
// WebSocket path (the mock SSH client only supports one-shot Exec).
func (s *Service) LiveSnapshot(client ssh.Connector, app *App) (*ContainerStats, error) {
	if app == nil {
		return nil, fmt.Errorf("app is nil")
	}
	if app.ServerID == 0 {
		return nil, fmt.Errorf("app %d is not assigned to a server", app.ID)
	}

	containers, err := listAppContainers(client, *app)
	if err != nil || len(containers) == 0 {
		return nil, err
	}

	var parsed []*ContainerStats
	for _, cname := range containers {
		out, err := client.Exec(fmt.Sprintf("docker stats --no-stream --format '{{json .}}' %s 2>/dev/null", cname))
		if err != nil || strings.TrimSpace(out) == "" {
			continue
		}
		var stat dockerStat
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &stat); err != nil {
			continue
		}
		if cs := parseDockerStat(stat, app.ID, app.ServerID, cname); cs != nil {
			parsed = append(parsed, cs)
		}
	}
	return sumStats(parsed), nil
}

// StreamStats continuously reads a streaming `docker stats` session for an app's
// containers and emits an aggregated snapshot (summed across containers) on
// each parsed line. It blocks until ctx is cancelled.
func (s *Service) StreamStats(ctx context.Context, client ssh.Connector, app *App, out chan<- *ContainerStats) error {
	if app == nil {
		return fmt.Errorf("app is nil")
	}
	if app.ServerID == 0 {
		return fmt.Errorf("app %d is not assigned to a server", app.ID)
	}

	containers, err := listAppContainers(client, *app)
	if err != nil || len(containers) == 0 {
		return err
	}

	cmd := fmt.Sprintf("docker stats --format '{{json .}}' %s 2>/dev/null", strings.Join(containers, " "))
	lines, err := client.ExecStream(ctx, cmd)
	if err != nil {
		return err
	}

	latest := make(map[string]*ContainerStats)
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var stat dockerStat
			if err := json.Unmarshal([]byte(line), &stat); err != nil {
				continue
			}
			cs := parseDockerStat(stat, app.ID, app.ServerID, stat.Name)
			if cs == nil {
				continue
			}
			latest[cs.ContainerName] = cs

			var all []*ContainerStats
			for _, c := range latest {
				all = append(all, c)
			}
			if agg := sumStats(all); agg != nil {
				select {
				case out <- agg:
				case <-ctx.Done():
					return nil
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
