package backup

import (
	"fmt"
	"strings"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/server"
	"gopkg.in/yaml.v3"
)

const Version = 1

type ExportData struct {
	Version int              `yaml:"version"`
	Servers []ExportServer   `yaml:"servers"`
	Apps    []ExportApp      `yaml:"apps"`
}

type ExportServer struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
}

type ExportApp struct {
	Name       string `yaml:"name"`
	ServerName string `yaml:"server_name"`
	Domain     string `yaml:"domain"`
	Port       int    `yaml:"port"`
	Compose    string `yaml:"compose"`
	GitRepo    string `yaml:"git_repo,omitempty"`
	GitBranch  string `yaml:"git_branch,omitempty"`
	AuthUser   string `yaml:"auth_user,omitempty"`
	AuthPass   string `yaml:"auth_pass,omitempty"`
}

type Service struct {
	serverSvc *server.Service
	appSvc    *app.Service
	keyDir    string
}

func NewService(serverSvc *server.Service, appSvc *app.Service, keyDir string) *Service {
	return &Service{serverSvc: serverSvc, appSvc: appSvc, keyDir: keyDir}
}

func (s *Service) Export() (string, error) {
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
			Name:       a.Name,
			ServerName: serverNameMap[a.ServerID],
			Domain:     a.Domain,
			Port:       a.Port,
			Compose:    a.Compose,
			GitRepo:    a.GitRepo,
			GitBranch:  a.GitBranch,
			AuthUser:   a.AuthUser,
			AuthPass:   a.AuthPass,
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
	sb.WriteString("# Remove or edit entries before import as needed.\n")
	sb.Write(out)
	return sb.String(), nil
}

func (s *Service) Import(yamlData string, mode string) (string, error) {
	var data ExportData
	if err := yaml.Unmarshal([]byte(yamlData), &data); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}

	if data.Version != Version {
		return "", fmt.Errorf("unsupported version %d (expected %d)", data.Version, Version)
	}

	var logLines []string

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

		ap := &app.App{
			Name:      ea.Name,
			ServerID:  sid,
			Domain:    ea.Domain,
			Port:      ea.Port,
			Compose:   ea.Compose,
			GitRepo:   ea.GitRepo,
			GitBranch: ea.GitBranch,
			AuthUser:  ea.AuthUser,
			AuthPass:  ea.AuthPass,
		}
		if ap.GitBranch == "" {
			ap.GitBranch = "main"
		}
		if err := s.appSvc.Create(ap); err != nil {
			return strings.Join(logLines, "\n"), fmt.Errorf("create app %q: %w", ea.Name, err)
		}
		logLines = append(logLines, fmt.Sprintf("Created app %q (%s)", ea.Name, ea.Domain))
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


