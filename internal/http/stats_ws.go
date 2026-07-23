package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type StatsHandler struct {
	serverSvc *server.Service
	sshKeyDir string
}

func NewStatsHandler(svc *server.Service, sshKeyDir string) *StatsHandler {
	return &StatsHandler{serverSvc: svc, sshKeyDir: sshKeyDir}
}

func (h *StatsHandler) ServeLiveStats(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid server id", http.StatusBadRequest)
		return
	}

	svr, err := h.serverSvc.Get(id)
	if err != nil || svr == nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("stats ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wsKeepAlive(conn, ctx)

	var client ssh.Connector

	if GetDevMock(r) {
		client = ssh.NewMockClient()
	} else {
		client, err = ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
		if err != nil {
			log.Printf("stats ssh connect error: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"SSH connection failed"}`))
			return
		}
		defer client.Close()
	}

	cpuCores, _ := parseCPUCountLive(client)
	ramMB, _ := parseRAMLive(client)
	diskGB, _ := parseDiskLive(client)

	prevStat, err := client.Exec("head -1 /proc/stat")
	if err != nil {
		log.Printf("stats initial cpu sample error: %v", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currStat, err := client.Exec("head -1 /proc/stat")
			if err != nil {
				continue
			}

			cpuUsage := calculateCPUUsage(prevStat, currStat)
			prevStat = currStat

			ramUsage, _ := parseRAMUsageLive(client)
			diskUsage, _ := parseDiskUsageLive(client)

			data := map[string]interface{}{
				"cpu_usage":  cpuUsage,
				"ram_usage":  ramUsage,
				"disk_usage": diskUsage,
				"cpu_cores":  cpuCores,
				"ram_mb":     ramMB,
				"disk_gb":    diskGB,
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
			}
			jsonStr, _ := json.Marshal(data)
			if err := conn.WriteMessage(websocket.TextMessage, jsonStr); err != nil {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func calculateCPUUsage(prev, curr string) float64 {
	prevParts := strings.Fields(prev)
	currParts := strings.Fields(curr)

	if len(prevParts) < 5 || len(currParts) < 5 {
		return 0
	}

	prevTotal := 0.0
	for _, f := range prevParts[1:] {
		v, _ := strconv.ParseFloat(f, 64)
		prevTotal += v
	}

	currTotal := 0.0
	for _, f := range currParts[1:] {
		v, _ := strconv.ParseFloat(f, 64)
		currTotal += v
	}

	prevIdle, _ := strconv.ParseFloat(prevParts[4], 64)
	currIdle, _ := strconv.ParseFloat(currParts[4], 64)

	dt := currTotal - prevTotal
	di := currIdle - prevIdle

	if dt <= 0 {
		return 0
	}

	return 100 * (dt - di) / dt
}

func parseCPUCountLive(c ssh.Connector) (int, error) {
	out, err := c.Exec("nproc")
	if err != nil {
		return 0, err
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count, nil
}

func parseRAMLive(c ssh.Connector) (int, error) {
	out, err := c.Exec("awk '/MemTotal/{printf \"%d\", $2/1024}' /proc/meminfo")
	if err != nil {
		return 0, err
	}
	var mb int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &mb)
	return mb, nil
}

func parseDiskLive(c ssh.Connector) (int, error) {
	out, err := c.Exec("df -BG / | awk 'NR==2 {gsub(/G/,\"\"); print $2}'")
	if err != nil {
		return 0, err
	}
	var gb int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &gb)
	return gb, nil
}

func parseRAMUsageLive(c ssh.Connector) (float64, error) {
	out, err := c.Exec("awk '/MemTotal/{t=$2} /MemAvailable/{a=$2} END{printf \"%.1f\", 100*(t-a)/t}' /proc/meminfo")
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}

func parseDiskUsageLive(c ssh.Connector) (float64, error) {
	out, err := c.Exec("df -BG / | awk 'NR==2 {gsub(/%/,\"\"); print $5}'")
	if err != nil {
		return 0, err
	}
	var usage float64
	fmt.Sscanf(strings.TrimSpace(out), "%f", &usage)
	return usage, nil
}
