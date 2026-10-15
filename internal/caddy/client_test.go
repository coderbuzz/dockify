package caddy

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestAuthRouteJSON checks the route against Caddy's module ids:
// http.handlers.authentication with provider http.authentication.providers.http_basic.
func TestAuthRouteJSON(t *testing.T) {
	route, err := authRoute("app.example.com", "app-1-web:80", "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Handle []struct {
			Handler   string `json:"handler"`
			Username  *string
			Providers *struct {
				HTTPBasic struct {
					Hash struct {
						Algorithm string `json:"algorithm"`
					} `json:"hash"`
					HashCache *struct{} `json:"hash_cache"`
					Accounts  []struct {
						Username string `json:"username"`
						Password string `json:"password"`
					} `json:"accounts"`
				} `json:"http_basic"`
			} `json:"providers"`
			Upstreams []struct {
				Dial string `json:"dial"`
			} `json:"upstreams"`
		} `json:"handle"`
	}
	if err := json.Unmarshal(body, &got); err != nil || len(got.Handle) != 2 {
		t.Fatalf("route = %s, err %v", body, err)
	}

	auth := got.Handle[0]
	if auth.Handler != "authentication" || auth.Providers == nil || auth.Username != nil || auth.Upstreams != nil {
		t.Fatalf("auth handler wrong: %s", body)
	}
	hb := auth.Providers.HTTPBasic
	if hb.Hash.Algorithm != "bcrypt" || hb.HashCache == nil || len(hb.Accounts) != 1 ||
		hb.Accounts[0].Username != "admin" || !strings.HasPrefix(hb.Accounts[0].Password, "$2a$") {
		t.Errorf("http_basic provider wrong: %s", body)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hb.Accounts[0].Password), []byte("secret")); err != nil {
		t.Errorf("password hash does not match: %v", err)
	}

	proxy := got.Handle[1]
	if proxy.Handler != "reverse_proxy" || proxy.Providers != nil || len(proxy.Upstreams) != 1 || proxy.Upstreams[0].Dial != "app-1-web:80" {
		t.Errorf("proxy handler wrong: %s", body)
	}
}

func TestConfigPath(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{`["caddy","run","--config","/etc/caddy/config.json"]` + "\n", "/etc/caddy/config.json"}, // InitWorker layout (prod-heavy-1)
		{`["caddy","run","--config","/data/config.json"]` + "\n", "/data/config.json"},           // older init (staging)
		{`["caddy","run","--resume"]`, "/etc/caddy/config.json"},
	}
	for _, c := range cases {
		if got, err := configPath(c.cmd); err != nil || got != c.want {
			t.Errorf("configPath(%s) = %q, %v; want %q", c.cmd, got, err, c.want)
		}
	}
	for _, bad := range []string{
		`["caddy","run","--config","/etc/caddy/Caddyfile","--adapter","caddyfile"]`,
		"Error: No such object: caddy",
	} {
		if got, err := configPath(bad); err == nil {
			t.Errorf("configPath(%s) = %q, want error", bad, got)
		}
	}
}
