package app

import (
	"testing"
	"time"
)

func TestRepo_ContainerStatsNetHistory_NoSpikeOnFirstBucket(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	a := &App{Name: "net-test-app", Domain: "net.example.com", Port: 80, ServerID: srvID, Compose: "x"}
	if err := repo.Create(a); err != nil {
		t.Fatalf("create app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Tick 1 (10 minutes ago): 1,000,000 bytes cumulative
	repo.InsertContainerStats(&ContainerStats{
		AppID: a.ID, ServerID: srvID, ContainerName: "web",
		CPUPercent: 1.0, MemUsageBytes: 100, MemLimitBytes: 1000, MemPercent: 10.0,
		NetIORxBytes: 500000, NetIOTxBytes: 500000,
	})
	repo.db.Exec(`UPDATE container_stats SET created_at = ? WHERE app_id = ?`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), a.ID)

	// Tick 2 (5 minutes ago): 1,060,000 bytes cumulative (+60,000 bytes in 5 mins = 60,000 / 300s = 200 B/s)
	repo.InsertContainerStats(&ContainerStats{
		AppID: a.ID, ServerID: srvID, ContainerName: "web",
		CPUPercent: 1.0, MemUsageBytes: 100, MemLimitBytes: 1000, MemPercent: 10.0,
		NetIORxBytes: 530000, NetIOTxBytes: 530000,
	})
	repo.db.Exec(`UPDATE container_stats SET created_at = ? WHERE app_id = ? AND net_io_rx_bytes = 530000`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"), a.ID)

	// Fetch 6h history (bucketMinutes = 5)
	since := now.Add(-6 * time.Hour)
	points, err := repo.ContainerStatsNetHistory(a.ID, since, 5)
	if err != nil {
		t.Fatalf("ContainerStatsNetHistory failed: %v", err)
	}

	if len(points) < 2 {
		t.Fatalf("expected at least 2 points, got %d", len(points))
	}

	// Verify that the first point does NOT contain a raw cumulative byte spike (e.g., 1,000,000 / 300 = 3333.3 B/s).
	// Because there was no prior sample before 10m ago, the first sample (10m ago) should evaluate to 0.0 B/s.
	if points[0].Value != 0.0 {
		t.Errorf("expected first bucket throughput to be 0.0 (no spike), got %f", points[0].Value)
	}

	// The second sample (5m ago) should be (1,060,000 - 1,000,000) / 300s = 200.0 B/s.
	if points[1].Value != 200.0 {
		t.Errorf("expected second bucket throughput to be 200.0 B/s, got %f", points[1].Value)
	}
}

func TestRepo_ContainerStatsNetHistory_ContainerReset(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)

	a := &App{Name: "reset-app", Domain: "reset.example.com", Port: 80, ServerID: srvID, Compose: "x"}
	repo.Create(a)

	now := time.Now().UTC().Truncate(time.Second)

	// Tick 1 (10 mins ago): 500,000 bytes
	repo.InsertContainerStats(&ContainerStats{
		AppID: a.ID, ServerID: srvID, ContainerName: "web",
		NetIORxBytes: 250000, NetIOTxBytes: 250000,
	})
	repo.db.Exec(`UPDATE container_stats SET created_at = ? WHERE app_id = ?`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), a.ID)

	// Tick 2 (5 mins ago, container restarted): 10,000 bytes (reset counter)
	repo.InsertContainerStats(&ContainerStats{
		AppID: a.ID, ServerID: srvID, ContainerName: "web",
		NetIORxBytes: 5000, NetIOTxBytes: 5000,
	})
	repo.db.Exec(`UPDATE container_stats SET created_at = ? WHERE app_id = ? AND net_io_rx_bytes = 5000`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"), a.ID)

	since := now.Add(-6 * time.Hour)
	points, err := repo.ContainerStatsNetHistory(a.ID, since, 5)
	if err != nil {
		t.Fatalf("ContainerStatsNetHistory failed: %v", err)
	}

	for _, p := range points {
		if p.Value < 0 {
			t.Errorf("expected non-negative net_rate, got %f for bucket %s", p.Value, p.Time)
		}
	}
}
