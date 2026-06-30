package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Host      string
	Port      string
	DataDir   string
	SSHKeyDir string

	AdminUser string
	AdminPass string

	CloudflareAPIToken string
	CloudflareZoneID   string

	DevMock bool
}

func Load() *Config {
	cfg := &Config{
		Host:               getEnv("DOCKIFY_HOST", "0.0.0.0"),
		Port:               getEnv("DOCKIFY_PORT", "8080"),
		DataDir:            getEnv("DOCKIFY_DATA_DIR", "/var/lib/dockify"),
		SSHKeyDir:          getEnv("DOCKIFY_SSH_KEY_DIR", "/var/lib/dockify/keys"),
		AdminUser:          getEnv("DOCKIFY_ADMIN_USER", "admin"),
		AdminPass:          os.Getenv("DOCKIFY_ADMIN_PASSWORD"),
		CloudflareAPIToken: os.Getenv("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:   os.Getenv("CLOUDFLARE_ZONE_ID"),

		DevMock: os.Getenv("DOCKIFY_DEV_MOCK") == "true",
	}

	os.MkdirAll(cfg.DataDir, 0700)
	os.MkdirAll(cfg.SSHKeyDir, 0700)

	return cfg
}

func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "dockify.db")
}

func (c *Config) Addr() string {
	return c.Host + ":" + c.Port
}

func (c *Config) AuthEnabled() bool {
	return c.AdminPass != ""
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
