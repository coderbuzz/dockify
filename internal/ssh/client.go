package ssh

import (
	"bytes"
	"fmt"
	"os"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type Client struct {
	conn  *gossh.Client
	Host  string
	Port  int
	User  string
}

func Connect(host string, port int, user, keyPath string) (*Client, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH key: %w", err)
	}

	signer, err := gossh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse SSH key: %w", err)
	}

	config := &gossh.ClientConfig{
		User: user,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := gossh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}

	return &Client{conn: conn, Host: host, Port: port, User: user}, nil
}

func (c *Client) Exec(cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return stdout.String(), fmt.Errorf("exec %q: %w: %s", cmd, err, stderr.String())
	}

	return stdout.String(), nil
}

func (c *Client) WriteFile(path, content string, mode os.FileMode) error {
	cmd := fmt.Sprintf(
		"mkdir -p $(dirname '%s') && cat > '%s' << 'DOCKIFY_EOF'\n%s\nDOCKIFY_EOF\nchmod %o '%s'",
		path, path, content, mode, path,
	)
	_, err := c.Exec(cmd)
	return err
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
