package http

import (
	"html/template"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/model"
)

func TestTemplatesParse(t *testing.T) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"lower":           strings.ToLower,
		"upper":           strings.ToUpper,
		"relativeTime":    relativeTime,
		"usedAmount":      usedAmount,
		"freeAmount":      freeAmount,
		"formatBytes":     formatBytes,
		"chartPoints":     chartPoints,
		"chartMax":        chartMax,
		"chartMax100":     chartMax100,
		"chartPointsJSON": chartPointsJSON,
		"chartThresholdY": chartThresholdY,
		"latestChartVal":  latestChartVal,
		"div":             div,
		"mul":             mul,
		"clamp100":        clamp100,
		"nl2br": func(s string) template.HTML {
			return template.HTML(strings.ReplaceAll(s, "\n", "<br>"))
		},
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
		"servers_resources_card",
		"servers_stats_card.html",
		"apps.html",
		"apps_add.html",
		"apps_detail.html",
		"apps_stats_card.html",
		"login.html",
		"error.html",
		"about.html",
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
		"Version": "0.3.15",
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
		"Secrets":         []interface{}{},
		"Deployments":     []interface{}{},
		"Routes":          []interface{}{},
		"Domains":         []string{"test.example.com"},
		"DomainCount":     1,
		"ExtraDomainCount": 0,
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
			"DiskUsage": 25.0,
			"CreatedAt": "2026-01-01",
		},
		"AppStats": map[string]interface{}{
			"CPUPercent":     10.5,
			"MemPercent":     50.0,
			"MemUsageBytes":  int64(1024000),
			"MemLimitBytes":  int64(2048000),
			"DiskUsageBytes": int64(5000000),
			"NetIORxBytes":   int64(1000),
			"NetIOTxBytes":   int64(2000),
			"BlockIORead":    int64(0),
			"BlockIOWrite":   int64(0),
		},
		"ServerGroups": []interface{}{
			map[string]interface{}{
				"ServerID":   int64(1),
				"ServerName": "worker-1",
				"Host":       "1.2.3.4",
				"Status":     "online",
				"Apps": []map[string]interface{}{
					{
						"ID":     int64(1),
						"Name":   "web-app",
						"Domain": "web.example.com",
						"Port":   8080,
						"Status": "running",
					},
				},
			},
		},
	}

	pageTemplates := []string{
		"about.html",
		"dashboard.html",
		"servers.html",
		"servers_add.html",
		"servers_detail.html",
		"servers_edit.html",
		"servers_resources_card",
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
			// Copy data map per template test to allow page-specific overrides if needed
			pageData := make(map[string]interface{})
			for k, v := range data {
				pageData[k] = v
			}
			if name == "apps.html" {
				pageData["AppStats"] = map[int64]interface{}{
					int64(1): map[string]interface{}{
						"CPUPercent":     12.5,
						"MemPercent":     45.0,
						"MemUsageBytes":  int64(104857600),
						"MemLimitBytes":  int64(1073741824),
						"DiskUsageBytes": int64(524288000),
					},
				}
			}
			err := tpl.Execute(new(strings.Builder), pageData)
			if err != nil {
				t.Errorf("template %q render failed: %v", name, err)
			}
		})
	}
}

func TestChartThresholdY(t *testing.T) {
	tests := []struct {
		name     string
		maxVal   float64
		height   int
		expected float64
	}{
		{"100 maxVal (100% threshold)", 100.0, 100, 0.0},
		{"200 maxVal (100% threshold)", 200.0, 100, 50.0},
		{"zero maxVal", 0.0, 100, 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chartThresholdY(tt.maxVal, tt.height)
			if got != tt.expected {
				t.Errorf("chartThresholdY(%v, %v) = %v; want %v", tt.maxVal, tt.height, got, tt.expected)
			}
		})
	}
}

func TestLatestChartVal(t *testing.T) {
	pts := []model.ChartPoint{
		{Time: "2026-01-01 00:00:00", Value: 10.0},
		{Time: "2026-01-01 00:01:00", Value: 45.5},
	}
	if got := latestChartVal(pts, 99.0); got != 45.5 {
		t.Errorf("expected 45.5, got %v", got)
	}
	if got := latestChartVal(nil, 99.0); got != 99.0 {
		t.Errorf("expected fallback 99.0, got %v", got)
	}
}
