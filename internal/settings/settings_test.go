package settings

import (
	"strings"
	"testing"
)

func TestUpgradeArgv(t *testing.T) {
	argv, err := upgradeArgv("v0.7.0")
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(argv, " ")
	if !strings.Contains(all, "https://raw.githubusercontent.com/coderbuzz/dockify/v0.7.0/scripts/update.sh") {
		t.Errorf("argv lacks the tag URL: %s", all)
	}
	if strings.Contains(all, "/tmp") || strings.Contains(all, "/main/") {
		t.Errorf("argv uses /tmp or main: %s", all)
	}
	for _, bad := range []string{"", "v", "main", "v1.0.0; rm -rf /"} {
		if _, err := upgradeArgv(bad); err == nil {
			t.Errorf("tag %q accepted", bad)
		}
	}
}
