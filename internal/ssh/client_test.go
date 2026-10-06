package ssh

import (
	"os/exec"
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
