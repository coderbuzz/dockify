package ssh

import (
	"bytes"
	"context"
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

func RealFactory() Factory {
	return func(host string, port int, user, keyPath string) (Connector, error) {
		return Connect(host, port, user, keyPath)
	}
}


func (c *Client) Shell(ctx context.Context, rows, cols int) (<-chan Output, chan<- Input, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("new session: %w", err)
	}

	if err := session.RequestPty("xterm-256color", rows, cols, gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.OCRNL:         1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("request pty: %w", err)
	}

	wIn, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}

	outCh := make(chan Output, 64)
	inCh := make(chan Input, 64)

	session.Stdout = channelWriter{outCh}
	session.Stderr = channelWriter{outCh}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("shell: %w", err)
	}

	// Forward input channel → SSH stdin
	go func() {
		defer wIn.Close()
		for input := range inCh {
			if input.Resize != nil {
				session.WindowChange(input.Resize.Height, input.Resize.Width)
				continue
			}
			if len(input.Data) > 0 {
				wIn.Write(input.Data)
			}
		}
	}()

	// Wait for remote exit
	go func() {
		session.Wait()
		session.Close()
		outCh <- Output{Closed: true}
	}()

	// Context cancellation → kill session
	go func() {
		<-ctx.Done()
		session.Close()
	}()

	return outCh, inCh, nil
}

type channelWriter struct {
	ch chan<- Output
}

func (w channelWriter) Write(p []byte) (int, error) {
	data := make([]byte, len(p))
	copy(data, p)
	w.ch <- Output{Data: data}
	return len(p), nil
}
