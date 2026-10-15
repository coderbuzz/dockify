package db

import "testing"

// productionSeed mirrors production before Wave 2: a live server with an app
// and its history, and a dead server without apps that deployments, DNS and
// stats rows still reference.
var productionSeed = []string{
	"INSERT INTO servers (id, name, host, ssh_key) VALUES (1, 'live', '10.0.0.1', '/k/1.pem'), (2, 'dead', '10.0.0.2', '/k/2.pem')",
	"INSERT INTO apps (id, name, server_id, compose, domain) VALUES (10, 'web', 1, 'services: {}', 'web.example.com')",
	"INSERT INTO deployments (app_id, server_id, status) VALUES (10, 1, 'success'), (10, 2, 'success')",
	"INSERT INTO routes (app_id, server_id, domain, target) VALUES (10, 1, 'web.example.com', '')",
	"INSERT INTO dns_records (app_id, server_id, zone_id, record_id, name, content) VALUES (10, 2, 'z', 'r1', 'web.example.com', '10.0.0.2')",
	"INSERT INTO container_stats (app_id, server_id, container_name) VALUES (10, 1, 'web-1'), (10, 2, 'web-1')",
	"INSERT INTO app_disk_stats (app_id, server_id) VALUES (10, 1), (10, 2)",
	"INSERT INTO route_stats (app_id, domain) VALUES (10, 'web.example.com')",
	"INSERT INTO server_stats (server_id) VALUES (1), (2)",
}

func TestForeignKeysAfterMigration(t *testing.T) {
	d := open(t, legacyDB(t, productionSeed...))

	rows, err := d.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Error("foreign_key_check reports violations after migration")
	}
	rows.Close()
	if scalar(t, d, "SELECT COUNT(*) FROM container_stats") != 2 {
		t.Fatal("container_stats rows lost in the rebuild")
	}

	if _, err := d.Exec("DELETE FROM servers WHERE id = 1"); err == nil {
		t.Error("deleting a server that still has apps succeeded")
	}

	if _, err := d.Exec("DELETE FROM servers WHERE id = 2"); err != nil {
		t.Fatalf("delete server without apps: %v", err)
	}
	for _, table := range []string{"deployments", "dns_records", "container_stats", "app_disk_stats"} {
		if n := scalar(t, d, "SELECT COUNT(*) FROM "+table+" WHERE server_id IS NULL"); n != 1 {
			t.Errorf("%s: %d rows with server_id NULL, want 1", table, n)
		}
	}
	if n := scalar(t, d, "SELECT COUNT(*) FROM server_stats WHERE server_id = 2"); n != 0 {
		t.Errorf("server_stats of the deleted server: %d rows", n)
	}

	if _, err := d.Exec("DELETE FROM apps WHERE id = 10"); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	for _, table := range []string{"deployments", "routes", "dns_records", "container_stats", "app_disk_stats", "route_stats"} {
		if n := scalar(t, d, "SELECT COUNT(*) FROM "+table); n != 0 {
			t.Errorf("%s: %d rows left after app delete", table, n)
		}
	}
}
