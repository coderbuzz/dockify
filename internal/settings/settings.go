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
	"time"
)

const webhookSecretKey = "webhook_secret"

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
}

type Service struct {
	db      *sql.DB
	version string
}

func NewService(db *sql.DB, version string) *Service {
	s := &Service{db: db, version: version}
	s.ensureWebhookSecret()
	return s
}

func (s *Service) CheckUpdate() (*UpdateInfo, error) {
	current := s.version
	if current == "" {
		current = "0.0.0"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/coderbuzz/dockify/releases/latest")
	if err != nil {
		return &UpdateInfo{Current: current, Latest: "", HasUpdate: false}, fmt.Errorf("fetch latest: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &UpdateInfo{Current: current, Latest: "", HasUpdate: false}, fmt.Errorf("decode: %w", err)
	}

	latest := result.TagName
	hasUpdate := latest != "" && latest != "v"+current && latest != current

	return &UpdateInfo{
		Current:   current,
		Latest:    latest,
		HasUpdate: hasUpdate,
	}, nil
}

func (s *Service) RunUpdate() error {
	script := `#!/bin/bash
sleep 1
export DOCKIFY_FORCE=y
curl -fsSL https://raw.githubusercontent.com/coderbuzz/dockify/main/scripts/update.sh | bash
`
	path := "/tmp/dockify-upgrade.sh"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		return fmt.Errorf("write upgrade script: %w", err)
	}
	cmd := exec.Command("setsid", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start upgrade: %w", err)
	}
	log.Printf("Update triggered: PID %d", cmd.Process.Pid)
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

func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
