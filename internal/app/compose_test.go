package app

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateSimple(t *testing.T) {
	c := generateCompose("nginx:alpine", 80, "FOO=bar,BAZ=qux", "")
	if c == "" {
		t.Fatal("empty compose")
	}

	names, err := parseServiceNames(c)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(names) != 1 || names[0] != "app" {
		t.Fatalf("expected service [app], got %v", names)
	}

	sn := getServiceName(c)
	if sn != "app" {
		t.Fatalf("expected app, got %s", sn)
	}
}

func TestGenerateWithVolumes(t *testing.T) {
	c := generateCompose("postgres:16", 5432, "POSTGRES_PASSWORD=secret", "./db:/var/lib/postgresql/data")
	if c == "" {
		t.Fatal("empty compose")
	}
	if !strings.Contains(c, "./db:/var/lib/postgresql/data") {
		t.Fatal("expected volume mount in compose")
	}
}

func TestParseAdvanced(t *testing.T) {
	compose := "services:\n  web:\n    image: nginx\n  redis:\n    image: redis"
	names, err := parseServiceNames(compose)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 services, got %d", len(names))
	}

	sn := getServiceName(compose)
	if sn != "web" && sn != "redis" {
		t.Fatalf("expected web or redis, got %s", sn)
	}
}

func TestAppNetworkAlias(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "myapp", "myapp"},
		{"with dots", "kv-dev.amg.id", "kv-dev-amg-id"},
		{"with underscores", "my_app", "my-app"},
		{"with spaces", "my app", "my-app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appNetworkAlias(tt.input)
			if got != tt.want {
				t.Errorf("appNetworkAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnsureDockifyNetworkAddsAlias(t *testing.T) {
	compose := `services:
  app:
    image: nginx:alpine
`
	result := ensureDockifyNetwork(compose, "my-app.example.com")

	if !strings.Contains(result, "dockify") {
		t.Fatal("expected dockify network in output")
	}
	if !strings.Contains(result, "my-app-example-com") {
		t.Fatalf("expected alias my-app-example-com in output:\n%s", result)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &doc); err != nil {
		t.Fatalf("result is not valid YAML: %v", err)
	}

	services := doc["services"].(map[string]interface{})
	svc := services["app"].(map[string]interface{})
	nets := svc["networks"].([]interface{})
	found := false
	for _, net := range nets {
		if m, ok := net.(map[string]interface{}); ok {
			if cfg, ok := m["dockify"].(map[string]interface{}); ok {
				aliases := cfg["aliases"].([]interface{})
				for _, a := range aliases {
					if a.(string) == "my-app-example-com" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("alias not found in dockify network config")
	}
}

func TestEnsureDockifyNetworkExistingDockify(t *testing.T) {
	compose := `services:
  app:
    image: nginx:alpine
    networks:
      - dockify
networks:
  dockify:
    external: true
`
	result := ensureDockifyNetwork(compose, "existing.app")

	if !strings.Contains(result, "existing-app") {
		t.Fatalf("expected alias existing-app in output:\n%s", result)
	}
}

func TestEnsureDockifyNetworkPreservesOtherNets(t *testing.T) {
	compose := `services:
  web:
    image: nginx
    networks:
      - frontend
      - dockify
networks:
  dockify:
    external: true
  frontend:
    external: true
`
	result := ensureDockifyNetwork(compose, "cool-app")

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &doc); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}
	services := doc["services"].(map[string]interface{})
	svc := services["web"].(map[string]interface{})
	nets := svc["networks"].([]interface{})

	hasFrontend := false
	hasDockify := false
	for _, net := range nets {
		switch n := net.(type) {
		case string:
			if n == "frontend" {
				hasFrontend = true
			}
		case map[string]interface{}:
			if _, ok := n["dockify"]; ok {
				hasDockify = true
			}
		}
	}
	if !hasFrontend {
		t.Fatal("frontend network was lost")
	}
	if !hasDockify {
		t.Fatal("dockify network not found")
	}
	if !strings.Contains(result, "cool-app") {
		t.Fatal("alias cool-app not found")
	}
}

