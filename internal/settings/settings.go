package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
)

const webhookSecretKey = "webhook_secret"

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	s := &Service{db: db}
	s.ensureWebhookSecret()
	return s
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
