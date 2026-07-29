package ssh

import (
	"context"
	"io"
	"os"
)

type Output struct {
	Data   []byte
	Closed bool
}

type WindowSize struct {
	Width  int
	Height int
}

type Input struct {
	Data   string
	Resize *WindowSize
}

type Connector interface {
	Exec(cmd string) (string, error)
	Shell(ctx context.Context, rows, cols int) (<-chan Output, chan<- Input, error)
	ExecPTY(ctx context.Context, cmd string, rows, cols int) (<-chan Output, chan<- Input, error)
	ExecStream(ctx context.Context, cmd string) (<-chan string, error)
	ExecPipe(cmd string, stdin io.Reader, stdout io.Writer) error
	WriteFile(path, content string, mode os.FileMode) error
	Close() error
}

type Factory func(host string, port int, user, keyPath string) (Connector, error)
