package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type StatsHandler struct {
	serverSvc *server.Service
	appSvc    *app.Service
	sshKeyDir string
}

func NewStatsHandler(svc *server.Service, appSvc *app.Service, sshKeyDir string) *StatsHandler {
	return &StatsHandler{serverSvc: svc, appSvc: appSvc, sshKeyDir: sshKeyDir}
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

	client, err := h.serverSvc.Connect(svr)
	if err != nil {
		log.Printf("stats ssh connect error: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"SSH connection failed"}`))
		return
	}
	defer client.Close()

	cpuCores, _ := server.ParseCPUCount(client)
	ramMB, _ := server.ParseRAM(client)
	diskGB, _ := server.ParseDisk(client)

	prevStat, err := client.Exec("head -1 /proc/stat")
	if err != nil {
		log.Printf("stats initial cpu sample error: %v", err)
	}

	currentRange := "realtime"
	sendChartData(conn, h.serverSvc, id, currentRange)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	chartTicker := time.NewTicker(30 * time.Second)
	defer chartTicker.Stop()

	rangeCh := make(chan string, 1)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				close(rangeCh)
				return
			}
			var clientMsg map[string]interface{}
			if err := json.Unmarshal(msg, &clientMsg); err == nil {
				if r, ok := clientMsg["range"].(string); ok {
					rangeCh <- r
				}
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			currStat, err := client.Exec("head -1 /proc/stat")
			if err != nil {
				continue
			}

			cpuUsage := clamp100(calculateCPUUsage(prevStat, currStat))
			prevStat = currStat

			ramUsage, _ := server.ParseRAMUsage(client)
			ramUsage = clamp100(ramUsage)

			diskUsage, _ := server.ParseDiskUsage(client)
			diskUsage = clamp100(diskUsage)

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

		case <-chartTicker.C:
			if err := sendChartData(conn, h.serverSvc, id, currentRange); err != nil {
				return
			}

		case r, ok := <-rangeCh:
			if !ok {
				return
			}
			currentRange = r
			if err := sendChartData(conn, h.serverSvc, id, currentRange); err != nil {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func sendChartData(conn *websocket.Conn, svc *server.Service, serverID int64, rangeStr string) error {
	chartData := svc.GetStatsHistory(serverID, rangeStr)
	if chartData == nil {
		return nil
	}
	data := map[string]interface{}{
		"chart_data": chartData,
	}
	jsonStr, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, jsonStr)
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
