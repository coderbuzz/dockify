package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type cachedResult struct {
	info      *UpdateInfo
	err       error
	timestamp time.Time
}

const webhookSecretKey = "webhook_secret"

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
}

type Service struct {
	db      *sql.DB
	version string

	cacheMu sync.Mutex
	cache   *cachedResult
}

func NewService(db *sql.DB, version string) *Service {
	s := &Service{db: db, version: version}
	s.ensureWebhookSecret()
	return s
}

func (s *Service) CheckUpdate(force bool) (*UpdateInfo, error) {
	current := s.version
	if current == "" {
		current = "0.0.0"
	}

	if !force {
		s.cacheMu.Lock()
		if s.cache != nil && time.Since(s.cache.timestamp) < 5*time.Minute {
			info, err := s.cache.info, s.cache.err
			s.cacheMu.Unlock()
			return info, err
		}
		s.cacheMu.Unlock()
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/coderbuzz/dockify/releases/latest")
	if err != nil {
		info := &UpdateInfo{Current: current, Latest: "", HasUpdate: false}
		err = fmt.Errorf("fetch latest: %w", err)
		s.cacheMu.Lock()
		s.cache = &cachedResult{info: info, err: err, timestamp: time.Now()}
		s.cacheMu.Unlock()
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		info := &UpdateInfo{Current: current, Latest: "", HasUpdate: false}
		err = fmt.Errorf("GitHub API rate limit exceeded (HTTP %d)", resp.StatusCode)
		s.cacheMu.Lock()
		s.cache = &cachedResult{info: info, err: err, timestamp: time.Now()}
		s.cacheMu.Unlock()
		return info, err
	}

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		info := &UpdateInfo{Current: current, Latest: "", HasUpdate: false}
		err = fmt.Errorf("decode: %w", err)
		s.cacheMu.Lock()
		s.cache = &cachedResult{info: info, err: err, timestamp: time.Now()}
		s.cacheMu.Unlock()
		return info, err
	}

	latest := result.TagName
	hasUpdate := latest != "" && latest != "v"+current && latest != current
	info := &UpdateInfo{
		Current:   current,
		Latest:    latest,
		HasUpdate: hasUpdate,
	}

	s.cacheMu.Lock()
	s.cache = &cachedResult{info: info, err: nil, timestamp: time.Now()}
	s.cacheMu.Unlock()

	return info, nil
}

func (s *Service) RunUpdate() error {
	script := `#!/bin/bash
exec > /tmp/dockify-update.log 2>&1
echo "Update started $(date)"
sleep 3
export DOCKIFY_FORCE=y
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash
echo "Update finished $(date)"
`
	path := "/tmp/dockify-upgrade.sh"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return fmt.Errorf("write upgrade script: %w", err)
	}
	cmd := exec.Command("systemd-run", "--no-block", "--unit=dockify-upgrade", "--collect", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemd-run: %w: %s", err, output)
	}
	log.Printf("Update triggered via systemd-run: %s", output)
	return nil
}

func (s *Service) ensureWebhookSecret() {
	_, err := s.GetWebhookSecret()
	if err != nil {
		log.Printf("Settings: failed to initialize webhook secret: %v", err)
	}
}

func (s *Service) GetWebhookSecret() (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", webhookSecretKey).Scan(&value)
	if err == sql.ErrNoRows {
		secret := generateSecret()
		_, err = s.db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", webhookSecretKey, secret)
		if err != nil {
			return "", err
		}
		return secret, nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Service) RegenerateWebhookSecret() (string, error) {
	secret := generateSecret()
	_, err := s.db.Exec("UPDATE settings SET value = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?", secret, webhookSecretKey)
	if err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) DisableWebhookSecret() error {
	_, err := s.db.Exec("DELETE FROM settings WHERE key = ?", webhookSecretKey)
	return err
}

func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
