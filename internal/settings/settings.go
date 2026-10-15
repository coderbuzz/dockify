package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
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
	info, _ := s.CheckUpdate(false)
	argv, err := upgradeArgv(info.Latest)
	if err != nil {
		return err
	}
	output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemd-run: %w: %s", err, output)
	}
	log.Printf("Update to %s triggered via systemd-run (journalctl -u dockify-upgrade): %s", info.Latest, output)
	return nil
}

var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// upgradeArgv runs update.sh from the release tag in a transient unit, so it
// survives the dockify restart. Nothing is written to disk; output goes to the journal.
func upgradeArgv(tag string) ([]string, error) {
	if !releaseTag.MatchString(tag) {
		return nil, fmt.Errorf("no latest release tag known (got %q); check for updates first", tag)
	}
	url := "https://raw.githubusercontent.com/coderbuzz/dockify/" + tag + "/scripts/update.sh"
	return []string{"systemd-run", "--no-block", "--unit=dockify-upgrade", "--collect",
		"--setenv=DOCKIFY_FORCE=y", "--setenv=DOCKIFY_VERSION=" + tag,
		"/bin/bash", "-c", "set -o pipefail; sleep 3; curl -fsSL " + url + " | bash"}, nil
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
