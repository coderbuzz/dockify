package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

type mapStore map[string]string

func (m mapStore) HostKey(host string, port int) (string, error) { return m[host], nil }
func (m mapStore) SaveHostKey(host string, port int, key string) error {
	m[host] = key
	return nil
}

func newHostKey(t *testing.T) gossh.PublicKey {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// S17: a generated key parses back and yields an authorized_keys line.
func TestGenerateKey(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "1.pem")
	if err := os.WriteFile(path, []byte(priv), 0600); err != nil {
		t.Fatal(err)
	}
	pub, err := PublicKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := gossh.ParseAuthorizedKey([]byte(pub)); err != nil || !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Fatalf("public key %q: %v", pub, err)
	}
}

func TestPinHostKey(t *testing.T) {
	store := mapStore{}
	check := pinHostKey(store, "w1", 22)
	key := newHostKey(t)

	if err := check("w1:22", nil, key); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if store["w1"] == "" || Fingerprint(store["w1"]) != gossh.FingerprintSHA256(key) {
		t.Fatalf("key not stored: %q", store["w1"])
	}
	if err := check("w1:22", nil, key); err != nil {
		t.Fatalf("same key: %v", err)
	}
	err := check("w1:22", nil, newHostKey(t))
	if err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("other key: got %v, want host key mismatch", err)
	}
}
