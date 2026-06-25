package app

import (
	"strings"
	"testing"
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

func TestSanitizeAppName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"myapp", "myapp"},
		{"kv-dev.amg.id", "kv-dev-amg-id"},
		{"my_app", "my-app"},
		{"my app", "my-app"},
	}
	for _, tt := range tests {
		got := sanitizeAppName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeAppName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRenameFirstService(t *testing.T) {
	compose := `services:
  app:
    image: nginx:alpine
    restart: unless-stopped
`
	result := renameFirstService(compose, "my-unique-app")

	if !strings.Contains(result, "my-unique-app") {
		t.Fatalf("expected renamed service in output:\n%s", result)
	}
	if strings.Contains(result, "  app:") {
		t.Fatal("old service name 'app' should be gone")
	}

	sn := getServiceName(result)
	if sn != "my-unique-app" {
		t.Fatalf("expected my-unique-app, got %s", sn)
	}
}

func TestRenameFirstServiceMultiService(t *testing.T) {
	compose := `services:
  web:
    image: nginx
  db:
    image: postgres
  redis:
    image: redis
`
	result := renameFirstService(compose, "my-app")

	if !strings.Contains(result, "my-app") {
		t.Fatalf("renamed service 'my-app' not found in output:\n%s", result)
	}
	if strings.Contains(result, "  web:") {
		t.Fatal("old first service name 'web' should be gone")
	}
	if !strings.Contains(result, "db:") {
		t.Fatal("second service 'db' should still exist")
	}
	if !strings.Contains(result, "redis:") {
		t.Fatal("third service 'redis' should still exist")
	}
}

func TestRenameFirstServiceSameName(t *testing.T) {
	compose := "services:\n  myapp:\n    image: nginx\n"
	result := renameFirstService(compose, "myapp")
	sn := getServiceName(result)
	if sn != "myapp" {
		t.Fatalf("should keep same name, got %s", sn)
	}
}

func TestRenameFirstServiceNoServices(t *testing.T) {
	compose := "networks:\n  dockify:\n    external: true\n"
	result := renameFirstService(compose, "whatever")
	if result != compose {
		t.Fatal("compose without services should be returned unchanged")
	}
}
