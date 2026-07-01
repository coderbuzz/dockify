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
	"strings"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/server"
	"gopkg.in/yaml.v3"
)

const Version = 1

type ExportData struct {
	Version int            `yaml:"version"`
	Servers []ExportServer `yaml:"servers"`
	Apps    []ExportApp    `yaml:"apps"`
}

type ExportServer struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
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
	UniqueServiceName  bool           `yaml:"unique_service_name,omitempty"`
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

	serverNameMap := map[int64]string{}
	for _, svr := range servers {
		serverNameMap[svr.ID] = svr.Name
		data.Servers = append(data.Servers, ExportServer{
			Name: svr.Name,
			Host: svr.Host,
			Port: svr.Port,
			User: svr.User,
		})
	}

	for _, a := range apps {
		ea := ExportApp{
			Name:              a.Name,
			ServerName:        serverNameMap[a.ServerID],
			Domain:            a.Domain,
			Port:              a.Port,
			Compose:           a.Compose,
			GitRepo:           a.GitRepo,
			GitBranch:         a.GitBranch,
			AuthUser:          a.AuthUser,
			UniqueServiceName: a.UniqueServiceName,
		}

		if a.AuthPass != "" {
			if passphrase != "" {
				ea.AuthPass, err = encrypt(a.AuthPass, passphrase)
				if err != nil {
					return "", fmt.Errorf("encrypt auth_pass for %q: %w", a.Name, err)
				}
			} else {
				ea.AuthPass = a.AuthPass
			}
		}

		secrets, err := s.appSvc.ListSecrets(a.ID)
		if err != nil {
			return "", fmt.Errorf("list secrets for %q: %w", a.Name, err)
		}
		for _, sec := range secrets {
			val := sec.Value
			if passphrase != "" {
				val, err = encrypt(sec.Value, passphrase)
				if err != nil {
					return "", fmt.Errorf("encrypt secret %q for %q: %w", sec.Key, a.Name, err)
				}
			}
			ea.Secrets = append(ea.Secrets, ExportSecret{Key: sec.Key, Value: val})
		}

		files, err := s.appSvc.ListFiles(a.ID)
		if err != nil {
			return "", fmt.Errorf("list files for %q: %w", a.Name, err)
		}
		for _, f := range files {
			content := f.Content
			if passphrase != "" {
				content, err = encrypt(f.Content, passphrase)
				if err != nil {
					return "", fmt.Errorf("encrypt file %q for %q: %w", f.Path, a.Name, err)
				}
			}
			ea.Files = append(ea.Files, ExportFile{Path: f.Path, Content: content})
		}

		data.Apps = append(data.Apps, ea)
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	sb := strings.Builder{}
	sb.WriteString("# Dockify Configuration Export\n")
	sb.WriteString("# Auth passwords (if present) are included as saved. SSH keys are not exported.\n")
	if passphrase != "" {
		sb.WriteString("# Secret values and config file contents are encrypted with the provided passphrase.\n")
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

	for _, ea := range data.Apps {
		if err := validateDecrypt(ea.AuthPass, passphrase); err != nil {
			return "", fmt.Errorf("%q: %w", ea.Name, err)
		}
		for _, sec := range ea.Secrets {
			if err := validateDecrypt(sec.Value, passphrase); err != nil {
				return "", fmt.Errorf("%q secret %q: %w", ea.Name, sec.Key, err)
			}
		}
		for _, f := range ea.Files {
			if err := validateDecrypt(f.Content, passphrase); err != nil {
				return "", fmt.Errorf("%q file %q: %w", ea.Name, f.Path, err)
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
	var err error

	for _, es := range data.Servers {
		existing, _ := s.findServerByName(es.Name)
		if existing != nil {
			logLines = append(logLines, fmt.Sprintf("Server %q already exists, skipping", es.Name))
			serverIDs[es.Name] = existing.ID
			continue
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

		authPass := ea.AuthPass
		if passphrase != "" {
			authPass, err = decrypt(ea.AuthPass, passphrase)
			if err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
			}
		}

		ap := &app.App{
			Name:              ea.Name,
			ServerID:          sid,
			Domain:            ea.Domain,
			Port:              ea.Port,
			Compose:           ea.Compose,
			GitRepo:           ea.GitRepo,
			GitBranch:         ea.GitBranch,
			AuthUser:          ea.AuthUser,
			AuthPass:          authPass,
			UniqueServiceName: ea.UniqueServiceName,
		}
		if ap.GitBranch == "" {
			ap.GitBranch = "main"
		}
		if err := s.appSvc.Create(ap); err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("create app %q: %w", ea.Name, err)
		}

		for _, sec := range ea.Secrets {
			val := sec.Value
			if passphrase != "" {
				val, err = decrypt(sec.Value, passphrase)
				if err != nil {
					return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
				}
			}
			if err := s.appSvc.SetSecret(ap.ID, sec.Key, val); err != nil {
				return strings.Join(logLines, "\n"), fmt.Errorf("set secret %q for %q: %w", sec.Key, ea.Name, err)
			}
		}

		for _, f := range ea.Files {
			content := f.Content
			if passphrase != "" {
				content, err = decrypt(f.Content, passphrase)
				if err != nil {
					return strings.Join(logLines, "\n"), fmt.Errorf("%q: wrong passphrase or corrupted data", ea.Name)
				}
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
