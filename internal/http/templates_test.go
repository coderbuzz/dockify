package http

import (
	"html/template"
	"strings"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"lower":        strings.ToLower,
		"upper":        strings.ToUpper,
		"relativeTime": relativeTime,
		"usedAmount":   usedAmount,
		"freeAmount":   freeAmount,
	})
	_, err := tmpl.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}
}

func TestTemplatesLookup(t *testing.T) {
	names := []string{
		"layout.html",
		"dashboard.html",
		"servers.html",
		"servers_add.html",
		"servers_detail.html",
		"servers_edit.html",
		"apps.html",
		"apps_add.html",
		"apps_detail.html",
		"login.html",
		"error.html",
		"settings.html",
		"export.html",
		"import.html",
	}
	for _, name := range names {
		if tmpl.Lookup(name) == nil {
			t.Errorf("template %q not found", name)
		}
	}
}

func TestTemplatesRender(t *testing.T) {
	data := map[string]interface{}{
		"Title":   "Test",
		"BasePath": "/",
		"Stats": &struct {
			TotalApps     int
			RunningApps   int
			TotalServers  int
			OnlineServers int
		}{1, 0, 1, 1},
		"Servers": []interface{}{},
		"Apps":    []interface{}{},
		"App": map[string]interface{}{
			"ID":             int64(1),
			"Name":           "test",
			"Domain":         "test.example.com",
			"Port":           80,
			"Status":         "running",
			"Compose":        "services:\n  app:\n    image: nginx",
			"GitRepo":        "",
			"GitBranch":      "main",
			"AuthUser":       "",
			"CreatedAt":      "2026-01-01",
			"UpdatedAt":      "2026-01-01",
		},
		"Secrets":    []interface{}{},
		"Deployments": []interface{}{},
		"Server": map[string]interface{}{
			"ID":        int64(1),
			"Name":      "test-server",
			"Host":      "1.2.3.4",
			"Port":      22,
			"User":      "root",
			"Status":    "online",
			"CPUCores":  2,
			"CPUUsage":  10.5,
			"RAMMB":     2048,
			"RAMUsage":  50.0,
			"DiskGB":    20,
			"CreatedAt": "2026-01-01",
		},
		"Error":     "something went wrong",
		"Message":   "import complete",
		"Log":       "line1\nline2",
		"WebhookSecret": "abc123",
	}

	pageTemplates := []string{
		"dashboard.html",
		"servers.html",
		"servers_add.html",
		"servers_detail.html",
		"servers_edit.html",
		"apps.html",
		"apps_add.html",
		"apps_detail.html",
		"login.html",
		"error.html",
		"settings.html",
		"export.html",
		"import.html",
	}

	for _, name := range pageTemplates {
		t.Run(name, func(t *testing.T) {
			tpl := tmpl.Lookup(name)
			if tpl == nil {
				t.Skipf("template %q not found", name)
				return
			}
			err := tpl.Execute(new(strings.Builder), data)
			if err != nil {
				t.Errorf("template %q render failed: %v", name, err)
			}
		})
	}
}
