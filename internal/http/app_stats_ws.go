package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// ServeLiveAppStats streams live per-second resource usage for an app's
// containers over a WebSocket, mirroring ServeLiveStats for servers.
//
// Backend strategy (efficient):
//   - real mode: a single streaming `docker stats` SSH session feeds parsed
//     container stats continuously (~1 line/container/sec); we aggregate (sum)
//     across the app's containers and emit one snapshot per line. No process
//     is spawned per second.
//   - dev mock mode: the mock SSH client only supports one-shot Exec, so we poll
//     `docker stats --no-stream` on a 1s ticker (the mock returns a canned JSON
//     line).
//
// The live stream does NOT write to the database; the 10s background collector
// (app.Service.statsLoop) is solely responsible for persisted history/audit.
func (h *StatsHandler) ServeLiveAppStats(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	a, err := h.appSvc.Get(id)
	if err != nil || a == nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if a.ServerID == 0 {
		http.Error(w, "app is not assigned to a server", http.StatusBadRequest)
		return
	}

	svr, err := h.serverSvc.Get(a.ServerID)
	if err != nil || svr == nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("app stats ws upgrade error: %v", err)
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
			log.Printf("app stats ssh connect error: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"SSH connection failed"}`))
			return
		}
		defer client.Close()
	}

	currentRange := "1h"
	sendAppChartData(conn, h.appSvc, id, currentRange)

	// Snapshot channel fed by the streaming collector (real mode) or the mock
	// ticker (mock mode).
	snapCh := make(chan *app.ContainerStats, 16)

	if GetDevMock(r) {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cs, err := h.appSvc.LiveSnapshot(client, a)
					if err != nil || cs == nil {
						continue
					}
					select {
					case snapCh <- cs:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	} else {
		go func() {
			if err := h.appSvc.StreamStats(ctx, client, a, snapCh); err != nil {
				log.Printf("app stats stream error for app %d: %v", id, err)
			}
		}()

		// Fallback: if StreamStats doesn't deliver a snapshot within 3 seconds
		// (e.g. docker stats streaming failed or returned no containers),
		// fall back to polling LiveSnapshot every 1s so the live feed stays alive.
		go func() {
			streamActive := int32(0)
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					if atomic.LoadInt32(&streamActive) == 0 {
						log.Printf("app stats fallback: no stream data for app %d, starting poll fallback", id)
						ticker := time.NewTicker(1 * time.Second)
						defer ticker.Stop()
						for {
							select {
							case <-ctx.Done():
								return
							case <-ticker.C:
								cs, err := h.appSvc.LiveSnapshot(client, a)
								if err != nil || cs == nil {
									continue
								}
								select {
								case snapCh <- cs:
								case <-ctx.Done():
									return
								}
							}
						}
					}
					return
				case cs := <-snapCh:
					atomic.StoreInt32(&streamActive, 1)
					// Re-inject the snapshot for the main loop to consume.
					select {
					case snapCh <- cs:
					case <-ctx.Done():
						return
					}
					return
				}
			}
		}()
	}

	// Read client messages (range changes) without blocking the snapshot loop.
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

	chartTicker := time.NewTicker(30 * time.Second)
	defer chartTicker.Stop()

	// Disk usage is collected by the background collector every 10s and stored
	// in the DB. We read it from the DB at the same cadence — no extra SSH call.
	diskTicker := time.NewTicker(10 * time.Second)
	defer diskTicker.Stop()

	// Throttle live emission to ~1/sec (matching the server live stats cadence
	// and the "per detik" requirement). For multi-container apps, docker stats
	// streams several lines per interval; we keep only the latest aggregate and
	// emit once per second.
	liveTicker := time.NewTicker(1 * time.Second)
	defer liveTicker.Stop()

	var latestSnap *app.ContainerStats
	var prevNetBytes int64
	var prevNetTime time.Time
	var diskUsageBytes int64

	for {
		select {
		case cs, ok := <-snapCh:
			if !ok {
				return
			}
			latestSnap = cs

		case <-liveTicker.C:
			if latestSnap == nil {
				continue
			}
			cs := latestSnap
			latestSnap = nil

			// Network throughput (bytes/s) = delta of cumulative bytes / elapsed.
			netRate := float64(0)
			now := time.Now()
			curNet := cs.NetIORxBytes + cs.NetIOTxBytes
			if !prevNetTime.IsZero() {
				elapsed := now.Sub(prevNetTime).Seconds()
				if elapsed > 0 {
					netRate = float64(curNet-prevNetBytes) / elapsed
					if netRate < 0 {
						netRate = 0
					}
				}
			}
			prevNetBytes = curNet
			prevNetTime = now

			data := map[string]interface{}{
				"cpu":              cs.CPUPercent,
				"memory":           cs.MemPercent,
				"network":          netRate,
				"mem_usage_bytes":  cs.MemUsageBytes,
				"mem_limit_bytes":  cs.MemLimitBytes,
				"net_rx":           cs.NetIORxBytes,
				"net_tx":           cs.NetIOTxBytes,
				"block_r":          cs.BlockIORead,
				"block_w":          cs.BlockIOWrite,
				"disk_usage_bytes": diskUsageBytes,
				"timestamp":        now.UTC().Format(time.RFC3339),
			}
			jsonStr, _ := json.Marshal(data)
			if err := conn.WriteMessage(websocket.TextMessage, jsonStr); err != nil {
				return
			}

		case <-chartTicker.C:
			if err := sendAppChartData(conn, h.appSvc, id, currentRange); err != nil {
				return
			}

		case <-diskTicker.C:
			if stats := h.appSvc.GetStatsOverview(id); stats != nil {
				diskUsageBytes = stats.DiskUsageBytes
			}

		case r, ok := <-rangeCh:
			if !ok {
				return
			}
			currentRange = r
			if err := sendAppChartData(conn, h.appSvc, id, currentRange); err != nil {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func sendAppChartData(conn *websocket.Conn, svc *app.Service, appID int64, rangeStr string) error {
	chartData := svc.GetStatsHistory(appID, rangeStr)
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
