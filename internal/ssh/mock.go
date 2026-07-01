package ssh

import (
	"context"
	"os"
	"strings"
	"time"
)

type MockClient struct {
	responses map[string]string
}

func NewMockClient() *MockClient {
	return &MockClient{
		responses: map[string]string{
			"uptime":                    "10:00:00 up 30 days,  0 users,  load average: 0.08, 0.03, 0.01",
			"nproc":                     "4",
			"Mem:":                      "16000",
			"NR==2 {gsub(/G/,\"\"); print $2}": "100",
			"cpu /":                      "23.5",
			"$3/$2 * 100":               "45.2",
			"gsub(/%/,\"\"); print $5}": "67",
			"docker":                    "Docker version 28.0.4, build b8034c0",
		},
	}
}

func (m *MockClient) Exec(cmd string) (string, error) {
	for pattern, response := range m.responses {
		if strings.Contains(cmd, pattern) {
			return response, nil
		}
	}
	return "", nil
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
				for _, b := range input.Data {
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
