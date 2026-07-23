package ssh

import (
	"context"
	"os"
	"strings"
	"time"
)

type mockResponse struct {
	pattern  string
	response string
}

type MockClient struct {
	responses []mockResponse
}

func NewMockClient() *MockClient {
	return &MockClient{
		responses: []mockResponse{
			{"docker stats", `{"BlockIO":"1.5MB / 0B","CPUPerc":"12.34%","Container":"abc12345","ID":"abc12345","MemPerc":"8.20%","MemUsage":"256MiB / 3.125GiB","Name":"app","NetIO":"45.6MB / 12.3MB","PIDs":"3"}`},
			{"docker compose version", "Docker Compose version v2.24.0"},
			{"docker compose", "app\n"},
			{"docker", "Docker version 28.0.4, build b8034c0"},
			{"uptime", "10:00:00 up 30 days,  0 users,  load average: 0.08, 0.03, 0.01"},
			{"nproc", "4"},
			{"MemTotal/{printf", "16000"},
			{"100*(t-a)/t", "45.2"},
			{"NR==2 {gsub(/G/,\"\"); print $2}", "100"},
			{"NR==2 {gsub(/%/,\"\"); print $5}", "67"},
			{"head -1 /proc/stat", "23.5"},
			{"du -sb", "123456789	/opt/dockify/apps/app-16\n987654321	/opt/dockify/apps/app-27"},
			{"caddy_http_requests_total", mockMetrics()},
		},
	}
}

func mockMetrics() string {
	return `# HELP caddy_http_requests_total Counter of total HTTP(S) requests.
# TYPE caddy_http_requests_total counter
caddy_http_requests_total{host="app.example.com",method="GET",status="200"} 15234
caddy_http_requests_total{host="app.example.com",method="POST",status="201"} 890
caddy_http_requests_total{host="app.example.com",method="GET",status="404"} 12
caddy_http_requests_total{host="app.example.com",method="GET",status="500"} 3
caddy_http_request_duration_seconds_count{host="app.example.com"} 16139
caddy_http_request_duration_seconds_sum{host="app.example.com"} 423.56
`
}

func (m *MockClient) Exec(cmd string) (string, error) {
	for _, r := range m.responses {
		if strings.Contains(cmd, r.pattern) {
			return r.response, nil
		}
	}
	return "", nil
}

func (m *MockClient) ExecStream(ctx context.Context, cmd string) (<-chan string, error) {
	outCh := make(chan string, 16)
	go func() {
		defer close(outCh)
		for _, r := range m.responses {
			if strings.Contains(cmd, r.pattern) {
				// Simulate real docker stats streaming output, which includes
				// ANSI escape sequences (\x1b[2J\x1b[H = clear screen + cursor home)
				// before each JSON line. This ensures the app stats parser correctly
				// strips them.
				resp := "\x1b[2J\x1b[H" + r.response
				select {
				case outCh <- resp:
				case <-ctx.Done():
					return
				}
				return
			}
		}
	}()
	return outCh, nil
}

func (m *MockClient) ExecPTY(ctx context.Context, cmd string, rows, cols int) (<-chan Output, chan<- Input, error) {
	outCh := make(chan Output, 64)
	inCh := make(chan Input, 64)

	go func() {
		defer close(outCh)

		prompt := []byte("\r\ncontainer:$ ")

		outCh <- Output{Data: []byte("\r\nDockify Dev Mock — Container console simulation\r\n")}
		outCh <- Output{Data: []byte("Running: " + cmd + "\r\n")}
		outCh <- Output{Data: prompt}

		var buf []byte
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				outCh <- Output{Data: []byte("\r\nSession closed.\r\n")}
				outCh <- Output{Closed: true}
				return
			case input, ok := <-inCh:
				if !ok {
					return
				}
				if input.Resize != nil {
					continue
				}
				for _, b := range []byte(input.Data) {
					if b == '\r' {
						outCh <- Output{Data: []byte("\r\n")}
						cmdStr := string(buf)
						buf = nil

						if cmdStr == "exit" || cmdStr == "exit " {
							outCh <- Output{Data: []byte("logout\r\n")}
							outCh <- Output{Closed: true}
							return
						}

						result := "Mock container response for: " + cmdStr + "\r\n"
						outCh <- Output{Data: []byte(result)}
						outCh <- Output{Data: prompt}
					} else if b == '\x7f' {
						if len(buf) > 0 {
							buf = buf[:len(buf)-1]
							outCh <- Output{Data: []byte{'\b', ' ', '\b'}}
						}
					} else {
						buf = append(buf, b)
						outCh <- Output{Data: []byte{b}}
					}
				}
			case <-ticker.C:
			}
		}
	}()

	return outCh, inCh, nil
}

func (m *MockClient) WriteFile(path, content string, mode os.FileMode) error {
	return nil
}

func (m *MockClient) Close() error {
	return nil
}

func (m *MockClient) Shell(ctx context.Context, rows, cols int) (<-chan Output, chan<- Input, error) {
	outCh := make(chan Output, 64)
	inCh := make(chan Input, 64)

	go func() {
		defer close(outCh)

		prompt := []byte("\r\n$ ")

		// Initial prompt
		outCh <- Output{Data: []byte("\r\nDockify Dev Mock — SSH console simulation\r\n")}
		outCh <- Output{Data: prompt}

		var buf []byte
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				outCh <- Output{Data: []byte("\r\nSession closed.\r\n")}
				outCh <- Output{Closed: true}
				return
			case input, ok := <-inCh:
				if !ok {
					return
				}
				if input.Resize != nil {
					continue
				}
				for _, b := range []byte(input.Data) {
					if b == '\r' {
						outCh <- Output{Data: []byte("\r\n")}
						cmd := string(buf)
						buf = nil

						// Simulate command response
						if cmd == "exit" || cmd == "exit " {
							outCh <- Output{Data: []byte("logout\r\n")}
							outCh <- Output{Closed: true}
							return
						}

						// Echo back the command
						result := "Mock response for: " + cmd + "\r\n"
						outCh <- Output{Data: []byte(result)}
						outCh <- Output{Data: prompt}
					} else if b == '\x7f' { // backspace
						if len(buf) > 0 {
							buf = buf[:len(buf)-1]
							outCh <- Output{Data: []byte{'\b', ' ', '\b'}}
						}
					} else {
						buf = append(buf, b)
						outCh <- Output{Data: []byte{b}}
					}
				}
			case <-ticker.C:
				// Keep the goroutine alive for cleanup
			}
		}
	}()

	return outCh, inCh, nil
}

func MockFactory() Factory {
	return func(host string, port int, user, keyPath string) (Connector, error) {
		return NewMockClient(), nil
	}
}
