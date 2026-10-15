package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type Client struct {
	conn *gossh.Client
	Host string
	Port int
	User string
}

// HostKeyStore holds the pinned SSH host key (base64 wire format) per host and port.
type HostKeyStore interface {
	HostKey(host string, port int) (string, error)
	SaveHostKey(host string, port int, key string) error
}

// pinHostKey trusts the key presented on first use and stores it; afterwards
// only that key is accepted.
func pinHostKey(store HostKeyStore, host string, port int) gossh.HostKeyCallback {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		pinned, err := store.HostKey(host, port)
		if err != nil {
			return fmt.Errorf("load host key for %s: %w", addr, err)
		}
		if pinned == "" {
			if err := store.SaveHostKey(host, port, base64.StdEncoding.EncodeToString(key.Marshal())); err != nil {
				return fmt.Errorf("save host key for %s: %w", addr, err)
			}
			log.Printf("pinned host key for %s (%s)", addr, gossh.FingerprintSHA256(key))
			return nil
		}
		raw, err := base64.StdEncoding.DecodeString(pinned)
		if err != nil {
			return fmt.Errorf("decode pinned host key for %s: %w", addr, err)
		}
		want, err := gossh.ParsePublicKey(raw)
		if err != nil {
			return fmt.Errorf("parse pinned host key for %s: %w", addr, err)
		}
		if gossh.FixedHostKey(want)(hostname, remote, key) != nil {
			return fmt.Errorf("host key mismatch for %s — if the server was reinstalled, clear its host key in the server settings", addr)
		}
		return nil
	}
}

// Fingerprint returns the SHA256 fingerprint of a pinned host key, or "" when
// none is pinned or it cannot be parsed.
func Fingerprint(pinned string) string {
	raw, err := base64.StdEncoding.DecodeString(pinned)
	if err != nil || pinned == "" {
		return ""
	}
	key, err := gossh.ParsePublicKey(raw)
	if err != nil {
		return ""
	}
	return gossh.FingerprintSHA256(key)
}

func Connect(host string, port int, user, keyPath string, store HostKeyStore) (*Client, error) {
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
		HostKeyCallback: pinHostKey(store, host, port),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	netConn, err := (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 15 * time.Second,
	}).Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}

	conn, chans, reqs, err := gossh.NewClientConn(netConn, addr, config)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}

	sshClient := gossh.NewClient(conn, chans, reqs)

	client := &Client{conn: sshClient, Host: host, Port: port, User: user}

	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			_, _, err := sshClient.Conn.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				return
			}
		}
	}()

	return client, nil
}

// Remote commands are wrapped in coreutils `timeout` so they die on the worker
// even if the SSH session drops (no PTY means no SIGHUP reaches them).
const (
	ExecTimeout     = 2 * time.Minute
	ExecLongTimeout = time.Hour
	// ExecPipeTimeout bounds streaming copies (app data can be many GB).
	ExecPipeTimeout = 6 * time.Hour
)

// Exec runs a command that is expected to finish quickly (reads, inspects, small writes).
func (c *Client) Exec(cmd string) (string, error) {
	return c.exec(cmd, ExecTimeout)
}

// ExecLong runs a slow command such as pull, up, prune or install.
func (c *Client) ExecLong(cmd string) (string, error) {
	return c.exec(cmd, ExecLongTimeout)
}

// wrapTimeout makes the worker kill cmd (and its whole process group) after d.
func wrapTimeout(cmd string, d time.Duration) string {
	return fmt.Sprintf("timeout -k 10 %d sh -c %s", int(d.Seconds()), shellQuote(cmd))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c *Client) exec(cmd string, d time.Duration) (string, error) {
	var stdout bytes.Buffer
	err := c.run(cmd, nil, &stdout, d)
	return stdout.String(), err
}

func (c *Client) ExecPipe(cmd string, stdin io.Reader, stdout io.Writer) error {
	return c.run(cmd, stdin, stdout, ExecPipeTimeout)
}

// run executes cmd under a remote timeout d, with optional stdin and stdout.
func (c *Client) run(cmd string, stdin io.Reader, stdout io.Writer, d time.Duration) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	if stdin != nil {
		session.Stdin = stdin
	}
	if stdout != nil {
		session.Stdout = stdout
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Start(wrapTimeout(cmd, d)); err != nil {
		return fmt.Errorf("exec %q: %w", cmd, err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	// Backstop in case the connection itself hangs; the remote timeout fires first.
	timer := time.NewTimer(d + 30*time.Second)
	defer timer.Stop()
	select {
	case err = <-done:
	case <-timer.C:
		session.Close()
		<-done
		return fmt.Errorf("exec %q: timed out after %s", cmd, d)
	}
	if err != nil {
		return fmt.Errorf("exec %q: %w: %s", cmd, err, stderr.String())
	}
	return nil
}

func (c *Client) ExecStream(ctx context.Context, cmd string) (<-chan string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return nil, fmt.Errorf("exec stream %q: %w", cmd, err)
	}

	outCh := make(chan string, 256)
	go func() {
		defer close(outCh)
		defer session.Close()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case outCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		<-ctx.Done()
		session.Close()
	}()

	return outCh, nil
}

// WriteFile sends content on stdin, so the shell never parses any of it. A new
// file is created 0600 (umask) and only then chmod'ed to mode.
func (c *Client) WriteFile(name, content string, mode os.FileMode) error {
	return c.run(writeFileCmd(name, mode), strings.NewReader(content), nil, ExecTimeout)
}

func writeFileCmd(name string, mode os.FileMode) string {
	q := shellQuote(name)
	return fmt.Sprintf("mkdir -p %s && (umask 077; cat > %s) && chmod %o %s", shellQuote(path.Dir(name)), q, mode, q)
}

func (c *Client) ExecPTY(ctx context.Context, cmd string, rows, cols int) (<-chan Output, chan<- Input, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("new session: %w", err)
	}

	if err := session.RequestPty("xterm-256color", rows, cols, gossh.TerminalModes{
		gossh.ECHO:          1,
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

	session.Stdout = channelWriter{outCh, ctx.Done()}
	session.Stderr = channelWriter{outCh, ctx.Done()}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("exec pty: %w", err)
	}

	pump(ctx, session, wIn, outCh, inCh)
	return outCh, inCh, nil
}

// pump runs an interactive session until the remote side exits or ctx ends.
// The caller closes inCh when done sending; outCh ends with Closed and is
// closed once the session's output is fully drained.
func pump(ctx context.Context, session *gossh.Session, wIn io.WriteCloser, outCh chan Output, inCh chan Input) {
	// Forward input channel → SSH stdin
	go func() {
		defer wIn.Close()
		for input := range inCh {
			if input.Resize != nil {
				session.WindowChange(input.Resize.Height, input.Resize.Width)
				continue
			}
			if len(input.Data) > 0 {
				wIn.Write([]byte(input.Data))
			}
		}
	}()

	// Wait for remote exit. Wait returns after the output copies finished, so
	// nothing writes to outCh once it is closed.
	go func() {
		session.Wait()
		session.Close()
		select {
		case outCh <- Output{Closed: true}:
		case <-ctx.Done():
		}
		close(outCh)
	}()

	// Context cancellation → kill session
	go func() {
		<-ctx.Done()
		session.Close()
	}()
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func RealFactory(store HostKeyStore) Factory {
	return func(host string, port int, user, keyPath string) (Connector, error) {
		return Connect(host, port, user, keyPath, store)
	}
}

func (c *Client) Shell(ctx context.Context, rows, cols int) (<-chan Output, chan<- Input, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("new session: %w", err)
	}

	if err := session.RequestPty("xterm-256color", rows, cols, gossh.TerminalModes{
		gossh.ECHO:          1,
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

	session.Stdout = channelWriter{outCh, ctx.Done()}
	session.Stderr = channelWriter{outCh, ctx.Done()}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("shell: %w", err)
	}

	pump(ctx, session, wIn, outCh, inCh)
	return outCh, inCh, nil
}

// channelWriter hands session output to the console; once done is closed (the
// console went away) it fails instead of blocking the session forever.
type channelWriter struct {
	ch   chan<- Output
	done <-chan struct{}
}

func (w channelWriter) Write(p []byte) (int, error) {
	select {
	case w.ch <- Output{Data: bytes.Clone(p)}:
		return len(p), nil
	case <-w.done:
		return 0, io.ErrClosedPipe
	}
}
