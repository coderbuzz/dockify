package caddy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coderbuzz/dockify/internal/ssh"
	"golang.org/x/crypto/bcrypt"
)

type Route struct {
	ID     string   `json:"@id"`
	Match  []match  `json:"match"`
	Handle []handle `json:"handle"`
}

type match struct {
	Host []string `json:"host"`
}

type handle struct {
	Handler   string     `json:"handler"`
	Upstreams []upstream `json:"upstreams,omitempty"`
	Username  string     `json:"username,omitempty"`
	Hash      string     `json:"hash,omitempty"`
}

type upstream struct {
	Dial string `json:"dial"`
}

type Client struct {
	ssh ssh.Connector
}

func NewClient(sshClient ssh.Connector) *Client {
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
	return c.postRoute(route)
}

func (c *Client) AddRouteWithAuth(domain, target, user, pass string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), 14)
	if err != nil {
		return fmt.Errorf("bcrypt hash: %w", err)
	}

	route := Route{
		ID: sanitizeID(domain),
		Match: []match{{Host: []string{domain}}},
		Handle: []handle{
			{
				Handler:  "basic_auth",
				Username: user,
				Hash:     string(hash),
			},
			{
				Handler:   "reverse_proxy",
				Upstreams: []upstream{{Dial: target}},
			},
		},
	}
	return c.postRoute(route)
}

func (c *Client) postRoute(route Route) error {
	// Hapus route lama dengan ID yang sama (kalau ada, ignore error)
	c.ssh.Exec(fmt.Sprintf(
		`docker exec caddy curl -s -o /dev/null -X DELETE http://localhost:2019/id/%s`,
		route.ID,
	))

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal route: %w", err)
	}

	// Pastikan routes array ada (bisa null di Caddy fresh). POST [] hanya
	// berhasil kalau routes null; kalau sudah ada, gagal harmless tanpa side effect.
	c.ssh.Exec(`docker exec caddy curl -s -o /dev/null -X POST http://localhost:2019/config/apps/http/servers/srv0/routes -H 'Content-Type: application/json' -d '[]'`)

	cmd := fmt.Sprintf(
		`docker exec caddy curl -s -w '%%{http_code}' -o /tmp/cr.txt -X POST http://localhost:2019/config/apps/http/servers/srv0/routes -H 'Content-Type: application/json' -d '%s'; echo; docker exec caddy cat /tmp/cr.txt 2>/dev/null`,
		escapeShell(string(body)),
	)
	out, err := c.ssh.Exec(cmd)
	if err != nil {
		verifyCmd := fmt.Sprintf(
			`docker exec caddy curl -s -o /dev/null -w '%%{http_code}' http://localhost:2019/id/%s`,
			route.ID,
		)
		vOut, vErr := c.ssh.Exec(verifyCmd)
		if vErr == nil && strings.TrimSpace(vOut) == "200" {
			return nil
		}
		return fmt.Errorf("caddy add route: %w (output: %s)", err, strings.TrimSpace(out))
	}
	lines := strings.SplitN(out, "\n", 2)
	code := strings.TrimSpace(lines[0])
	if code != "200" {
		if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
			return fmt.Errorf("caddy returned HTTP %s (body: %s)", code, strings.TrimSpace(lines[1]))
		}
		verifyCmd := fmt.Sprintf(
			`docker exec caddy curl -s -o /dev/null -w '%%{http_code}' http://localhost:2019/id/%s`,
			route.ID,
		)
		vOut, vErr := c.ssh.Exec(verifyCmd)
		if vErr == nil && strings.TrimSpace(vOut) == "200" {
			return nil
		}
		return fmt.Errorf("caddy returned HTTP %s", code)
	}
	return nil
}

func (c *Client) RemoveRoute(domain string) error {
	id := sanitizeID(domain)
	cmd := fmt.Sprintf(
		`docker exec caddy curl -s -w '%%{http_code}' -o /tmp/cr.txt -X DELETE http://localhost:2019/id/%s; echo; docker exec caddy cat /tmp/cr.txt 2>/dev/null`,
		id,
	)
	out, err := c.ssh.Exec(cmd)
	if err != nil {
		return fmt.Errorf("caddy remove route: %w (output: %s)", err, strings.TrimSpace(out))
	}
	lines := strings.SplitN(out, "\n", 2)
	code := strings.TrimSpace(lines[0])
	if code != "200" {
		return fmt.Errorf("caddy returned HTTP %s", code)
	}
	return nil
}

func sanitizeID(s string) string {
	r := strings.NewReplacer(".", "-", "*", "-")
	return "dockify-" + r.Replace(s)
}

func escapeShell(s string) string {
	r := strings.NewReplacer("'", `'"'"'`)
	return r.Replace(s)
}
