package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/server"
	"gopkg.in/yaml.v3"
)

const Version = 1

type ExportData struct {
	Version    int            `yaml:"version"`
	MasterSalt string         `yaml:"master_salt,omitempty"`
	Servers    []ExportServer `yaml:"servers"`
	Apps       []ExportApp    `yaml:"apps"`
}

type ExportServer struct {
	Name   string `yaml:"name"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	User   string `yaml:"user"`
	SSHKey string `yaml:"ssh_key,omitempty"`
}

type ExportSecret struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type ExportFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

type ExportApp struct {
	Name               string         `yaml:"name"`
	ServerName         string         `yaml:"server_name"`
	Domain             string         `yaml:"domain"`
	Port               int            `yaml:"port"`
	Compose            string         `yaml:"compose"`
	GitRepo            string         `yaml:"git_repo,omitempty"`
	GitBranch          string         `yaml:"git_branch,omitempty"`
	AuthUser           string         `yaml:"auth_user,omitempty"`
	AuthPass           string         `yaml:"auth_pass,omitempty"`
	ComposeMode string `yaml:"compose_mode,omitempty"`
	Secrets            []ExportSecret `yaml:"secrets,omitempty"`
	Files              []ExportFile   `yaml:"files,omitempty"`
}

type Service struct {
	serverSvc *server.Service
	appSvc    *app.Service
	keyDir    string
}

func NewService(serverSvc *server.Service, appSvc *app.Service, keyDir string) *Service {
	return &Service{serverSvc: serverSvc, appSvc: appSvc, keyDir: keyDir}
}

func encrypt(plaintext, passphrase string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), nil)
	raw := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	raw = append(raw, salt...)
	raw = append(raw, nonce...)
	raw = append(raw, ciphertext...)
	return "enc:" + base64.StdEncoding.EncodeToString(raw), nil
}

func encryptValue(aead cipher.AEAD, plaintext string) (string, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), nil)
	raw := append(nonce, ciphertext...)
	return "enc:" + base64.StdEncoding.EncodeToString(raw), nil
}

func decrypt(encoded, passphrase string) (string, error) {
	if !strings.HasPrefix(encoded, "enc:") {
		return encoded, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded[4:])
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(raw) < 28 {
		return "", fmt.Errorf("invalid encrypted data")
	}
	salt, raw := raw[:16], raw[16:]
	nonce, ciphertext := raw[:12], raw[12:]
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("message authentication failed: %w", err)
	}
	return string(plaintext), nil
}

func decryptWithAEAD(aead cipher.AEAD, encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "enc:") {
		return encoded, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded[4:])
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	nonceSize := aead.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("invalid encrypted data")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("message authentication failed: %w", err)
	}
	return string(plaintext), nil
}

func validateDecrypt(value, passphrase string) error {
	if !strings.HasPrefix(value, "enc:") {
		return nil
	}
	if passphrase == "" {
		return fmt.Errorf("has an encrypted value but no passphrase was provided")
	}
	_, err := decrypt(value, passphrase)
	if err != nil {
		return fmt.Errorf("wrong passphrase or corrupted data")
	}
	return nil
}

func validateDecryptWithAEAD(aead cipher.AEAD, value string) error {
	if !strings.HasPrefix(value, "enc:") {
		return nil
	}
	_, err := decryptWithAEAD(aead, value)
	if err != nil {
		return fmt.Errorf("wrong passphrase or corrupted data")
	}
	return nil
}

func deriveAEAD(passphrase string) ([]byte, cipher.AEAD, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, err
	}
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	return salt, aead, nil
}

func deriveAEADFromSalt(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead, nil
}

func (s *Service) Export(passphrase string) (string, error) {
	servers, err := s.serverSvc.List()
	if err != nil {
		return "", fmt.Errorf("list servers: %w", err)
	}

	apps, err := s.appSvc.List()
	if err != nil {
		return "", fmt.Errorf("list apps: %w", err)
	}

	data := ExportData{Version: Version}

	var enc func(string) (string, error)
	if passphrase != "" {
		salt, aead, err := deriveAEAD(passphrase)
		if err != nil {
			return "", fmt.Errorf("derive encryption key: %w", err)
		}
		data.MasterSalt = base64.StdEncoding.EncodeToString(salt)
		enc = func(plaintext string) (string, error) {
			return encryptValue(aead, plaintext)
		}
	} else {
		enc = func(plaintext string) (string, error) {
			return plaintext, nil
		}
	}

	serverNameMap := map[int64]string{}
	for _, svr := range servers {
		serverNameMap[svr.ID] = svr.Name
		es := ExportServer{
			Name: svr.Name,
			Host: svr.Host,
			Port: svr.Port,
			User: svr.User,
		}
		if svr.SSHKey != "" && svr.SSHKey != "pending" {
			raw, err := os.ReadFile(svr.SSHKey)
			if err != nil {
				return "", fmt.Errorf("read ssh_key file for %q: %w", svr.Name, err)
			}
			keyContent := string(raw)
			es.SSHKey, err = enc(keyContent)
			if err != nil {
				return "", fmt.Errorf("encrypt ssh_key for %q: %w", svr.Name, err)
			}
		}
		data.Servers = append(data.Servers, es)
	}

	allSecrets, err := s.appSvc.ListAllSecrets()
	if err != nil {
		return "", fmt.Errorf("list all secrets: %w", err)
	}
	secretsByApp := map[int64][]ExportSecret{}
	for _, sec := range allSecrets {
		val, err := enc(sec.Value)
		if err != nil {
			return "", fmt.Errorf("encrypt secret %q: %w", sec.Key, err)
		}
		secretsByApp[sec.AppID] = append(secretsByApp[sec.AppID], ExportSecret{Key: sec.Key, Value: val})
	}

	allFiles, err := s.appSvc.ListAllFiles()
	if err != nil {
		return "", fmt.Errorf("list all files: %w", err)
	}
	filesByApp := map[int64][]ExportFile{}
	for _, f := range allFiles {
		content, err := enc(f.Content)
		if err != nil {
			return "", fmt.Errorf("encrypt file %q: %w", f.Path, err)
		}
		filesByApp[f.AppID] = append(filesByApp[f.AppID], ExportFile{Path: f.Path, Content: content})
	}

	for _, a := range apps {
		ea := ExportApp{
			Name:        a.Name,
			ServerName:  serverNameMap[a.ServerID],
			Domain:      a.Domain,
			Port:        a.Port,
			Compose:     a.Compose,
			GitRepo:     a.GitRepo,
			GitBranch:   a.GitBranch,
			AuthUser:    a.AuthUser,
			ComposeMode: a.ComposeMode,
		}

		if a.AuthPass != "" {
			ea.AuthPass, err = enc(a.AuthPass)
			if err != nil {
				return "", fmt.Errorf("encrypt auth_pass for %q: %w", a.Name, err)
			}
		}

		ea.Secrets = secretsByApp[a.ID]
		ea.Files = filesByApp[a.ID]

		data.Apps = append(data.Apps, ea)
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	sb := strings.Builder{}
	sb.WriteString("# Dockify Configuration Export\n")
	if passphrase != "" {
		sb.WriteString("# All sensitive values (secrets, auth passwords, config files, SSH keys) are encrypted.\n")
	}
	sb.WriteString("# Remove or edit entries before import as needed.\n")
	sb.Write(out)
	return sb.String(), nil
}

func (s *Service) Import(yamlData, passphrase, mode string) (string, error) {
	var data ExportData
	if err := yaml.Unmarshal([]byte(yamlData), &data); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}

	if data.Version != Version {
		return "", fmt.Errorf("unsupported version %d (expected %d)", data.Version, Version)
	}

	var logLines []string

	var impAEAD cipher.AEAD
	if passphrase != "" && data.MasterSalt != "" {
		salt, err := base64.StdEncoding.DecodeString(data.MasterSalt)
		if err != nil {
			return "", fmt.Errorf("decode master salt: %w", err)
		}
		impAEAD, err = deriveAEADFromSalt(passphrase, salt)
		if err != nil {
			return "", fmt.Errorf("derive encryption key: %w", err)
		}
	}

	var dec func(string) (string, error)
	if impAEAD != nil {
		dec = func(value string) (string, error) {
			return decryptWithAEAD(impAEAD, value)
		}
	} else if passphrase != "" {
		dec = func(value string) (string, error) {
			return decrypt(value, passphrase)
		}
	} else {
		dec = func(value string) (string, error) {
			return value, nil
		}
	}

	for _, es := range data.Servers {
		if impAEAD != nil {
			if err := validateDecryptWithAEAD(impAEAD, es.SSHKey); err != nil {
				return "", fmt.Errorf("server %q ssh_key: %w", es.Name, err)
			}
		} else {
			if err := validateDecrypt(es.SSHKey, passphrase); err != nil {
				return "", fmt.Errorf("server %q ssh_key: %w", es.Name, err)
			}
		}
	}
	for _, ea := range data.Apps {
		if impAEAD != nil {
			if err := validateDecryptWithAEAD(impAEAD, ea.AuthPass); err != nil {
				return "", fmt.Errorf("%q: %w", ea.Name, err)
			}
		} else {
			if err := validateDecrypt(ea.AuthPass, passphrase); err != nil {
				return "", fmt.Errorf("%q: %w", ea.Name, err)
			}
		}
		for _, sec := range ea.Secrets {
			if impAEAD != nil {
				if err := validateDecryptWithAEAD(impAEAD, sec.Value); err != nil {
					return "", fmt.Errorf("%q secret %q: %w", ea.Name, sec.Key, err)
				}
			} else {
				if err := validateDecrypt(sec.Value, passphrase); err != nil {
					return "", fmt.Errorf("%q secret %q: %w", ea.Name, sec.Key, err)
				}
			}
		}
		for _, f := range ea.Files {
			if impAEAD != nil {
				if err := validateDecryptWithAEAD(impAEAD, f.Content); err != nil {
					return "", fmt.Errorf("%q file %q: %w", ea.Name, f.Path, err)
				}
			} else {
				if err := validateDecrypt(f.Content, passphrase); err != nil {
					return "", fmt.Errorf("%q file %q: %w", ea.Name, f.Path, err)
				}
			}
		}
	}

	if mode == "replace" {
		apps, _ := s.appSvc.List()
		for _, a := range apps {
			logLines = append(logLines, fmt.Sprintf("Removing app %q", a.Name))
			s.appSvc.Undeploy(a.ID)
		}
		servers, _ := s.serverSvc.List()
		for _, svr := range servers {
			logLines = append(logLines, fmt.Sprintf("Removing server %q", svr.Name))
			s.serverSvc.Delete(svr.ID)
		}
	}

	serverIDs := map[string]int64{}

	for _, es := range data.Servers {
		existing, _ := s.findServerByName(es.Name)
		if existing != nil {
			logLines = append(logLines, fmt.Sprintf("Server %q already exists, skipping", es.Name))
			serverIDs[es.Name] = existing.ID
			continue
		}
		sshKeyContent, err := dec(es.SSHKey)
		if err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("server %q: wrong passphrase or corrupted data", es.Name)
		}

		svr := &server.Server{
			Name:   es.Name,
			Host:   es.Host,
			Port:   es.Port,
			User:   es.User,
			SSHKey: "pending",
			Status: "pending",
		}
		if svr.Port == 0 {
			svr.Port = 22
		}
		if svr.User == "" {
			svr.User = "root"
		}
		if err := s.serverSvc.Create(svr); err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("create server %q: %w", es.Name, err)
		}

		if sshKeyContent != "" && sshKeyContent != "pending" {
			path := filepath.Join(s.keyDir, fmt.Sprintf("%d.pem", svr.ID))
			if err := os.WriteFile(path, []byte(sshKeyContent), 0600); err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("save ssh_key file for %q: %w", es.Name, err)
			}
			svr.SSHKey = path
			if err := s.serverSvc.Update(svr); err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("update ssh_key path for %q: %w", es.Name, err)
			}
		}

		serverIDs[es.Name] = svr.ID
		logLines = append(logLines, fmt.Sprintf("Created server %q (%s)", es.Name, es.Host))
	}

	for _, ea := range data.Apps {
		existing, _ := s.findAppByName(ea.Name)
		if existing != nil {
			logLines = append(logLines, fmt.Sprintf("App %q already exists, skipping", ea.Name))
			continue
		}

		sid, ok := serverIDs[ea.ServerName]
		if !ok {
			logLines = append(logLines, fmt.Sprintf("App %q: server %q not found, skipping", ea.Name, ea.ServerName))
			continue
		}

		authPass, err := dec(ea.AuthPass)
		if err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
		}

		ap := &app.App{
			Name:        ea.Name,
			ServerID:    sid,
			Domain:      ea.Domain,
			Port:        ea.Port,
			Compose:     ea.Compose,
			GitRepo:     ea.GitRepo,
			GitBranch:   ea.GitBranch,
			AuthUser:    ea.AuthUser,
			AuthPass:    authPass,
			ComposeMode: ea.ComposeMode,
		}
		if ap.GitBranch == "" {
			ap.GitBranch = "main"
		}
		if err := s.appSvc.Create(ap); err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("create app %q: %w", ea.Name, err)
		}

		for _, sec := range ea.Secrets {
			val, err := dec(sec.Value)
			if err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
			}
			if err := s.appSvc.SetSecret(ap.ID, sec.Key, val); err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("set secret %q for %q: %w", sec.Key, ea.Name, err)
			}
		}

		for _, f := range ea.Files {
			content, err := dec(f.Content)
			if err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
			}
			if err := s.appSvc.SetFile(ap.ID, f.Path, content); err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("set file %q for %q: %w", f.Path, ea.Name, err)
			}
		}

		logLines = append(logLines, fmt.Sprintf("Created app %q (%s) with %d secrets and %d files", ea.Name, ea.Domain, len(ea.Secrets), len(ea.Files)))
	}

	return strings.Join(logLines, "\n"), nil
}

func (s *Service) findServerByName(name string) (*server.Server, error) {
	servers, err := s.serverSvc.List()
	if err != nil {
		return nil, err
	}
	for _, svr := range servers {
		if svr.Name == name {
			return &svr, nil
		}
	}
	return nil, nil
}

func (s *Service) findAppByName(name string) (*app.App, error) {
	apps, err := s.appSvc.List()
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		if a.Name == name {
			return &a, nil
		}
	}
	return nil, nil
}
