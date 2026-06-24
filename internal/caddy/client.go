package caddy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coderbuzz/dockify/internal/ssh"
)

type Route struct {
	ID      string   `json:"@id"`
	Match   []match  `json:"match"`
	Handle  []handle `json:"handle"`
}

type match struct {
	Host []string `json:"host"`
}

type handle struct {
	Handler   string     `json:"handler"`
	Upstreams []upstream `json:"upstreams"`
}

type upstream struct {
	Dial string `json:"dial"`
}

type Client struct {
	ssh *ssh.Client
}

func NewClient(sshClient *ssh.Client) *Client {
	return &Client{ssh: sshClient}
}

func (c *Client) AddRoute(domain, target string) error {
	route := Route{
		ID: sanitizeID(domain),
		Match: []match{{Host: []string{domain}}},
		Handle: []handle{{
			Handler:   "reverse_proxy",
			Upstreams: []upstream{{Dial: target}},
		}},
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal route: %w", err)
	}

	cmd := fmt.Sprintf(
		`curl -s -o /dev/null -w '%%{http_code}' -X POST http://localhost:2019/config/apps/http/servers/srv0/routes -H 'Content-Type: application/json' -d '%s'`,
		escapeShell(string(body)),
	)
	out, err := c.ssh.Exec(cmd)
	if err != nil {
		return fmt.Errorf("caddy add route: %w", err)
	}
	code := strings.TrimSpace(out)
	if code != "200" {
		return fmt.Errorf("caddy returned HTTP %s", code)
	}
	return nil
}

func (c *Client) RemoveRoute(domain string) error {
	id := sanitizeID(domain)
	cmd := fmt.Sprintf(
		`curl -s -o /dev/null -w '%%{http_code}' -X DELETE http://localhost:2019/id/%s`,
		id,
	)
	out, err := c.ssh.Exec(cmd)
	if err != nil {
		return fmt.Errorf("caddy remove route: %w", err)
	}
	code := strings.TrimSpace(out)
	if code != "200" {
		return fmt.Errorf("caddy returned HTTP %s", code)
	}
	return nil
}

func sanitizeID(s string) string {
	r := strings.NewReplacer(
		".", "-",
		"*", "-",
	)
	return "dockify-" + r.Replace(s)
}

func escapeShell(s string) string {
	r := strings.NewReplacer("'", `'"'"'`)
	return r.Replace(s)
}
