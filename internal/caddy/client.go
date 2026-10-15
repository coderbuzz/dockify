package caddy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	Providers *providers `json:"providers,omitempty"`
}

// providers configures Caddy's http.handlers.authentication module.
type providers struct {
	HTTPBasic httpBasic `json:"http_basic"`
}

type httpBasic struct {
	Hash      hashConfig `json:"hash"`
	HashCache struct{}   `json:"hash_cache"` // without it every request re-runs bcrypt cost 14 (~1 s CPU)
	Accounts  []account  `json:"accounts"`
}

type hashConfig struct {
	Algorithm string `json:"algorithm"`
}

type account struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
		ID:    sanitizeID(domain),
		Match: []match{{Host: []string{domain}}},
		Handle: []handle{{
			Handler:   "reverse_proxy",
			Upstreams: []upstream{{Dial: target}},
		}},
	}
	return c.postRoute(route)
}

func (c *Client) AddRouteWithAuth(domain, target, user, pass string) error {
	route, err := authRoute(domain, target, user, pass)
	if err != nil {
		return err
	}
	return c.postRoute(route)
}

func authRoute(domain, target, user, pass string) (Route, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), 14)
	if err != nil {
		return Route{}, fmt.Errorf("bcrypt hash: %w", err)
	}
	return Route{
		ID:    sanitizeID(domain),
		Match: []match{{Host: []string{domain}}},
		Handle: []handle{
			{
				Handler: "authentication",
				Providers: &providers{HTTPBasic: httpBasic{
					Hash:     hashConfig{Algorithm: "bcrypt"},
					Accounts: []account{{Username: user, Password: string(hash)}},
				}},
			},
			{
				Handler:   "reverse_proxy",
				Upstreams: []upstream{{Dial: target}},
			},
		},
	}, nil
}

func (c *Client) postRoute(route Route) error {
	// Remove old route with same ID (if any, ignore error)
	c.ssh.Exec(fmt.Sprintf(
		`docker exec caddy curl -s -o /dev/null -X DELETE http://localhost:2019/id/%s`,
		route.ID,
	))

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal route: %w", err)
	}

	// Ensure routes array exists (may be null on fresh Caddy). POST [] only
	// succeeds if routes is null; if already populated, fails harmlessly.
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

// SaveConfig writes the running config to the file Caddy was started with
// (`caddy run --config <path>`) so routes added through the admin API survive a
// Caddy restart or reboot. Workers differ: InitWorker uses /etc/caddy/config.json,
// older inits /data/config.json (a single-file bind mount, so the write is in place).
func (c *Client) SaveConfig() error {
	out, err := c.ssh.Exec(`docker inspect caddy --format '{{json .Config.Cmd}}'`)
	if err != nil {
		return fmt.Errorf("caddy save config: %w", err)
	}
	path, err := configPath(out)
	if err != nil {
		return fmt.Errorf("caddy save config: %w", err)
	}
	// rmdir drops a stray empty directory at the path and never a non-empty one;
	// curl -f leaves the file untouched when the admin API answers with an error.
	cmd := fmt.Sprintf(`docker exec caddy sh -c '[ ! -d "$1" ] || rmdir "$1"; curl -sf http://localhost:2019/config/ -o "$1"' sh '%s'`, escapeShell(path))
	if _, err := c.ssh.Exec(cmd); err != nil {
		return fmt.Errorf("caddy save config to %s: %w", path, err)
	}
	return nil
}

// configPath returns the --config argument of the Caddy container's command, given
// as JSON by docker inspect. A non-JSON file (a Caddyfile) is refused: the admin
// API's JSON must not overwrite it.
func configPath(cmdJSON string) (string, error) {
	var args []string
	if err := json.Unmarshal([]byte(cmdJSON), &args); err != nil {
		return "", fmt.Errorf("parse caddy command %q: %w", strings.TrimSpace(cmdJSON), err)
	}
	path := "/etc/caddy/config.json"
	for i, a := range args {
		if a == "--config" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	if !strings.HasSuffix(path, ".json") {
		return "", fmt.Errorf("caddy runs with non-JSON config %s, not overwriting it", path)
	}
	return path, nil
}

// CheckCertificate checks whether Caddy has obtained a valid TLS certificate
// for the given domain by performing a TLS handshake via localhost.
func (c *Client) CheckCertificate(domain string) (bool, error) {
	cmd := fmt.Sprintf(
		`docker exec caddy curl -sI -o /dev/null -w '%%{http_code}' --connect-timeout 5 --resolve %s:443:127.0.0.1 https://%s/ 2>&1`,
		domain, domain,
	)
	out, err := c.ssh.Exec(cmd)
	if err != nil {
		return false, nil
	}
	code := strings.TrimSpace(out)
	if code == "" || code == "000" {
		return false, nil
	}
	n, err := strconv.Atoi(code)
	if err != nil || n == 0 {
		return false, nil
	}
	return true, nil
}

// WaitForCertificate polls CheckCertificate until the cert is ready or timeout.
func (c *Client) WaitForCertificate(domain string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ready, err := c.CheckCertificate(domain)
		if err == nil && ready {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func sanitizeID(s string) string {
	r := strings.NewReplacer(".", "-", "*", "-")
	return "dockify-" + r.Replace(s)
}

func escapeShell(s string) string {
	r := strings.NewReplacer("'", `'"'"'`)
	return r.Replace(s)
}
