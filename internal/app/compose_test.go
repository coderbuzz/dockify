package app

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateSimple(t *testing.T) {
	c := generateCompose("nginx:alpine", 80, "", "", "", "", "", "", nil, "", "", "")
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

func TestGenerateSimpleWithLogging(t *testing.T) {
	c := generateCompose("nginx:alpine", 80, "", "my-app", "", "", "10m", "3", nil, "", "", "")
	if c == "" {
		t.Fatal("empty compose")
	}

	if sf := parseSimpleFields(c); sf.LogMaxSize != "10m" || sf.LogMaxFile != "3" {
		t.Fatalf("expected logging configuration in compose, got:\n%s", c)
	}
}

// Values with ':' or '#' must not break the YAML, numeric limits stay numbers,
// and parseSimpleFields reads back what the form sent.
func TestGenerateComposeRoundTrip(t *testing.T) {
	cmd := `sh -c 'echo a: b # not a comment'`
	c := generateCompose("ghcr.io/x/app:1.2", 8080, "./data:/data\n./conf:/etc/app:ro", "my.app", "512m", "0.5", "10m", "3",
		[]string{"FOO", "BAR"}, cmd, "8080:80, 9000:9000", "65535")

	var doc struct {
		Services map[string]struct {
			Image   string
			CPUs    float64 `yaml:"cpus"`
			Ulimits struct{ Nofile struct{ Soft, Hard int } }
		}
		Networks map[string]struct{ External bool }
	}
	if err := yaml.Unmarshal([]byte(c), &doc); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, c)
	}
	svc := doc.Services["my-app"]
	if svc.Image != "ghcr.io/x/app:1.2" || svc.CPUs != 0.5 || svc.Ulimits.Nofile.Soft != 65535 || svc.Ulimits.Nofile.Hard != 65535 || !doc.Networks["dockify"].External {
		t.Fatalf("unexpected compose:\n%s", c)
	}

	want := simpleFields{
		Image: "ghcr.io/x/app:1.2", Port: 8080, EnvKeys: []string{"FOO", "BAR"},
		Volumes: "./data:/data\n./conf:/etc/app:ro", MemoryLimit: "512m", CPULimit: "0.5",
		LogMaxSize: "10m", LogMaxFile: "3", Command: cmd, Ports: "8080:80\n9000:9000", UlimitsNofile: "65535",
	}
	if got := parseSimpleFields(c); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestGenerateSimpleWithAppName(t *testing.T) {
	c := generateCompose("nginx:alpine", 80, "", "my-app", "", "", "", "", nil, "", "", "")
	if c == "" {
		t.Fatal("empty compose")
	}

	names, err := parseServiceNames(c)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(names) != 1 || names[0] != "my-app" {
		t.Fatalf("expected service [my-app], got %v", names)
	}

	sn := getServiceName(c)
	if sn != "my-app" {
		t.Fatalf("expected my-app, got %s", sn)
	}
}

func TestGenerateWithVolumes(t *testing.T) {
	c := generateCompose("postgres:16", 5432, "./db:/var/lib/postgresql/data", "", "", "", "", "", nil, "", "", "")
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

func TestParseSimpleFields(t *testing.T) {
	compose := `services:
  my-app:
    image: nginx:alpine
    restart: unless-stopped
    networks:
      - dockify
    environment:
      - FOO=bar
      - BAZ=qux
    expose:
      - "80"
    volumes:
      - ./data:/data
networks:
  dockify:
    external: true`

	sf := parseSimpleFields(compose)
	if sf.Image != "nginx:alpine" {
		t.Fatalf("expected nginx:alpine, got %q", sf.Image)
	}
	if sf.Port != 80 {
		t.Fatalf("expected 80, got %d", sf.Port)
	}
	if len(sf.EnvKeys) != 2 || sf.EnvKeys[0] != "FOO" || sf.EnvKeys[1] != "BAZ" {
		t.Fatalf("expected [FOO BAZ], got %v", sf.EnvKeys)
	}
	if sf.Volumes != "./data:/data" {
		t.Fatalf("expected ./data:/data, got %q", sf.Volumes)
	}
}

func TestParseSimpleFieldsEmpty(t *testing.T) {
	sf := parseSimpleFields("")
	if sf.Image != "" || sf.Port != 0 || len(sf.EnvKeys) != 0 || sf.Volumes != "" {
		t.Fatal("expected empty fields for empty compose")
	}
}

func TestParseSimpleFieldsMultipleServices(t *testing.T) {
	compose := `services:
  web:
    image: nginx
  db:
    image: postgres`
	sf := parseSimpleFields(compose)
	if sf.Image != "nginx" {
		t.Fatalf("expected nginx, got %q", sf.Image)
	}
}
