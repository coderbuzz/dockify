package ssh

import "os"

type Connector interface {
	Exec(cmd string) (string, error)
	WriteFile(path, content string, mode os.FileMode) error
	Close() error
}

type Factory func(host string, port int, user, keyPath string) (Connector, error)
