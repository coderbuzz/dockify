package ssh

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWrapTimeout(t *testing.T) {
	cmd := `echo "it's $((1+1))" && printf '%s\n' 'a b' | cat`
	got := wrapTimeout(cmd, 2*time.Minute)
	prefix := "timeout -k 10 120 sh -c "
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("got %q, want prefix %q", got, prefix)
	}

	// The quoted part must hand the original command to sh unchanged.
	out, err := exec.Command("sh", "-c", "sh -c "+strings.TrimPrefix(got, prefix)).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	if want := "it's 2\na b\n"; string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// B13: a console writer must not block once nobody reads its output.
func TestChannelWriterDone(t *testing.T) {
	done := make(chan struct{})
	close(done)
	w := channelWriter{make(chan Output), done} // unbuffered, never read
	finished := make(chan error, 1)
	go func() {
		_, err := w.Write([]byte("x"))
		finished <- err
	}()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("Write succeeded without a reader")
		}
	case <-time.After(time.Second):
		t.Fatal("Write blocked after done was closed")
	}
}

// writeFileCmd must store content byte for byte, whatever the path or the
// content contain, and apply the mode.
func TestWriteFileCmd(t *testing.T) {
	name := filepath.Join(t.TempDir(), "it's a dir", "$(touch pwned)", ".env")
	content := "A=\"x\"\nDOCKIFY_EOF\n'; touch pwned; '\n$HOME `id`"

	cmd := exec.Command("sh", "-c", writeFileCmd(name, 0600))
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("got %q, want %q", got, content)
	}
	if st, _ := os.Stat(name); st.Mode().Perm() != 0600 {
		t.Fatalf("mode %o, want 600", st.Mode().Perm())
	}
}
