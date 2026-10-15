package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type noDeploy struct{}

func (noDeploy) DeployByGit(repo, branch, commitSHA string) {}

func TestWebhookAuth(t *testing.T) {
	body := `{"ref":"refs/heads/main","after":"abc"}`
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	cases := []struct {
		name, secret, path, header, value, body string
		want                                    int
	}{
		{"github signed", "s3cret", "github", "X-Hub-Signature-256", sig, body, http.StatusOK},
		{"github without configured secret", "", "github", "X-Hub-Signature-256", sig, body, http.StatusUnauthorized},
		{"gitlab token", "s3cret", "gitlab", "X-Gitlab-Token", "s3cret", body, http.StatusOK},
		{"gitlab wrong token", "s3cret", "gitlab", "X-Gitlab-Token", "nope", body, http.StatusUnauthorized},
		{"gitlab without configured secret", "", "gitlab", "X-Gitlab-Token", "", body, http.StatusUnauthorized},
		{"body over 1 MiB", "s3cret", "gitlab", "X-Gitlab-Token", "s3cret", strings.Repeat(" ", 1<<20) + body, http.StatusBadRequest},
	}
	for _, c := range cases {
		h := NewHandler(noDeploy{}, func() string { return c.secret })
		req := httptest.NewRequest("POST", "/api/webhook/"+c.path, strings.NewReader(c.body))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Gitlab-Event", "Push Hook")
		req.Header.Set(c.header, c.value)
		rec := httptest.NewRecorder()
		if c.path == "github" {
			h.GitHub(rec, req)
		} else {
			h.GitLab(rec, req)
		}
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
