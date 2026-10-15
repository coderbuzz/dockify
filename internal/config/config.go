package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Host      string
	Port      string
	DataDir   string
	SSHKeyDir string

	AdminUser string
	AdminPass string
	APIToken  string

	CloudflareAPIToken string
	CloudflareZoneID   string

	AllowNoAuth bool
	DevMock     bool
}

func Load() *Config {
	dataDir := getEnv("DOCKIFY_DATA_DIR", "/var/lib/dockify")
	cfg := &Config{
		Host:               getEnv("DOCKIFY_HOST", "0.0.0.0"),
		Port:               getEnv("DOCKIFY_PORT", "8080"),
		DataDir:            dataDir,
		SSHKeyDir:          getEnv("DOCKIFY_SSH_KEY_DIR", filepath.Join(dataDir, "keys")),
		AdminUser:          getEnv("DOCKIFY_ADMIN_USER", "admin"),
		AdminPass:          os.Getenv("DOCKIFY_ADMIN_PASSWORD"),
		APIToken:           os.Getenv("DOCKIFY_API_TOKEN"),
		CloudflareAPIToken: os.Getenv("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:   os.Getenv("CLOUDFLARE_ZONE_ID"),

		AllowNoAuth: os.Getenv("DOCKIFY_ALLOW_NO_AUTH") == "true",
		DevMock:     os.Getenv("DOCKIFY_DEV_MOCK") == "true",
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

// CheckAuth refuses a passwordless UI on a non-loopback bind unless the
// operator opted in with DOCKIFY_ALLOW_NO_AUTH=true.
func (c *Config) CheckAuth() error {
	h := strings.Trim(c.Host, "[]")
	if c.AdminPass != "" || c.AllowNoAuth || h == "localhost" || net.ParseIP(h).IsLoopback() {
		return nil
	}
	return errors.New("refusing to start: no DOCKIFY_ADMIN_PASSWORD set and DOCKIFY_HOST=" + c.Host +
		" is reachable from the network. Set DOCKIFY_ADMIN_PASSWORD, bind to DOCKIFY_HOST=127.0.0.1," +
		" or set DOCKIFY_ALLOW_NO_AUTH=true to run without authentication")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
