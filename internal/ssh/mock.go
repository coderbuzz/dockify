package ssh

import (
	"os"
	"strings"
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

func MockFactory() Factory {
	return func(host string, port int, user, keyPath string) (Connector, error) {
		return NewMockClient(), nil
	}
}
