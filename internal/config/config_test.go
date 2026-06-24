package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	os.Unsetenv("DOCKIFY_HOST")
	os.Unsetenv("DOCKIFY_PORT")
	os.Unsetenv("DOCKIFY_DATA_DIR")

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host: expected 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port: expected 8080, got %s", cfg.Port)
	}
	if cfg.DataDir != "/var/lib/dockify" {
		t.Errorf("DataDir: expected /var/lib/dockify, got %s", cfg.DataDir)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("AdminUser: expected admin, got %s", cfg.AdminUser)
	}
}

func TestCustomEnv(t *testing.T) {
	os.Setenv("DOCKIFY_HOST", "127.0.0.1")
	os.Setenv("DOCKIFY_PORT", "3000")
	os.Setenv("DOCKIFY_DATA_DIR", "/tmp/test-dockify")
	os.Setenv("CLOUDFLARE_API_TOKEN", "fake-token")
	os.Setenv("CLOUDFLARE_ZONE_ID", "fake-zone")
	defer func() {
		os.Unsetenv("DOCKIFY_HOST")
		os.Unsetenv("DOCKIFY_PORT")
		os.Unsetenv("DOCKIFY_DATA_DIR")
		os.Unsetenv("CLOUDFLARE_API_TOKEN")
		os.Unsetenv("CLOUDFLARE_ZONE_ID")
	}()

	cfg := Load()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host: expected 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != "3000" {
		t.Errorf("Port: expected 3000, got %s", cfg.Port)
	}
	if cfg.CloudflareAPIToken != "fake-token" {
		t.Errorf("CloudflareAPIToken: expected fake-token, got %s", cfg.CloudflareAPIToken)
	}
	if cfg.CloudflareZoneID != "fake-zone" {
		t.Errorf("CloudflareZoneID: expected fake-zone, got %s", cfg.CloudflareZoneID)
	}
}

func TestAddr(t *testing.T) {
	cfg := &Config{Host: "0.0.0.0", Port: "9090"}
	if cfg.Addr() != "0.0.0.0:9090" {
		t.Errorf("Addr: expected 0.0.0.0:9090, got %s", cfg.Addr())
	}
}

func TestDBPath(t *testing.T) {
	cfg := &Config{DataDir: "/var/lib/dockify"}
	if cfg.DBPath() != "/var/lib/dockify/dockify.db" {
		t.Errorf("DBPath: expected /var/lib/dockify/dockify.db, got %s", cfg.DBPath())
	}
}
