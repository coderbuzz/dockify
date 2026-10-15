package caddy

import "testing"

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
