package caddy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/ssh"
	"golang.org/x/crypto/bcrypt"
)

// TestAuthRouteJSON checks the route against Caddy's module ids:
// http.handlers.authentication with provider http.authentication.providers.http_basic.
func TestAuthRouteJSON(t *testing.T) {
	route, err := authRoute("app.example.com", "app-1-web:80", "admin", "secret", nil)
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

// fakeCaddy answers the GET of /id/<route> with current (404 when empty) and
// records every admin API write.
type fakeCaddy struct {
	ssh.Connector
	current string
	writes  []string
}

func (f *fakeCaddy) Exec(cmd string) (string, error) {
	if strings.Contains(cmd, "curl -sf http://localhost:2019/id/") {
		if f.current == "" {
			return "", errors.New("exit status 22")
		}
		return f.current, nil
	}
	if strings.Contains(cmd, " -X ") {
		f.writes = append(f.writes, cmd)
	}
	return "200\n", nil
}

// TestPostRouteSkipsUnchanged: every admin API write reloads Caddy and closes all
// websockets on the server with 1001, so an unchanged route must not be written.
// current is shaped like Caddy's GET /id/ output (keys sorted).
func TestPostRouteSkipsUnchanged(t *testing.T) {
	plain := `{"@id":"dockify-a-example-com","handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"web:80"}]}],"match":[{"host":["a.example.com"]}]}` + "\n"
	h, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	auth := `{"@id":"dockify-a-example-com","handle":[{"handler":"authentication","providers":{"http_basic":{"accounts":[{"password":"` + string(h) + `","username":"u"}],"hash":{"algorithm":"bcrypt"},"hash_cache":{}}}},{"handler":"reverse_proxy","upstreams":[{"dial":"web:80"}]}],"match":[{"host":["a.example.com"]}]}`

	cases := []struct {
		name, current string
		add           func(*Client) error
		wantWrites    bool
	}{
		{"unchanged", plain, func(c *Client) error { return c.AddRoute("a.example.com", "web:80") }, false},
		{"changed target", plain, func(c *Client) error { return c.AddRoute("a.example.com", "web:81") }, true},
		{"missing", "", func(c *Client) error { return c.AddRoute("a.example.com", "web:80") }, true},
		{"auth added", plain, func(c *Client) error { return c.AddRouteWithAuth("a.example.com", "web:80", "u", "p") }, true},
		{"auth unchanged", auth, func(c *Client) error { return c.AddRouteWithAuth("a.example.com", "web:80", "u", "p") }, false},
		{"auth new password", auth, func(c *Client) error { return c.AddRouteWithAuth("a.example.com", "web:80", "u", "p2") }, true},
		{"auth removed", auth, func(c *Client) error { return c.AddRoute("a.example.com", "web:80") }, true},
	}
	for _, tc := range cases {
		f := &fakeCaddy{current: tc.current}
		if err := tc.add(NewClient(f)); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !tc.wantWrites && len(f.writes) > 0 {
			t.Errorf("%s: wrote to Caddy: %q", tc.name, f.writes)
		}
		if tc.wantWrites && (len(f.writes) != 3 || !strings.Contains(f.writes[0], "-X DELETE") || !strings.Contains(f.writes[2], "-X POST")) {
			t.Errorf("%s: want DELETE, POST [], POST route; got %q", tc.name, f.writes)
		}
	}
}
