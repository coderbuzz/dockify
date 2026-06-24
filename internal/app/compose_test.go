package app

import "testing"

func TestGenerateSimple(t *testing.T) {
	c := generateCompose("nginx:alpine", 80, "FOO=bar,BAZ=qux")
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
