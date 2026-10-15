package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestEnvFile(t *testing.T) {
	for _, tc := range []struct{ name, value, line string }{
		{"plain", "abc", `K="abc"`},
		{"inner spaces", "a b  c", `K="a b  c"`},
		{"leading/trailing spaces", "  abc \t", `K="abc"`},
		{"inner quote", `say "hi"`, `K="say \"hi\""`},
		{"dollar", "pa$$w$rd", `K="pa$$$$w$$rd"`},
		{"hash", "abc #not-a-comment", `K="abc #not-a-comment"`},
		{"newline", "line1\nline2", `K="line1\nline2"`},
		{"backslash", `C:\dir\n`, `K="C:\\dir\\n"`},
	} {
		if got := envFile([]AppSecret{{Key: "K", Value: tc.value}}); got != tc.line+"\n" {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.line+"\n")
		}
	}
}

// written returns what a deploy wrote to path on the source server.
func (f *moveFixture) written(t *testing.T, path string) (header, content string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if rest, ok := strings.CutPrefix(c, f.src.Host+" WRITE "+path+" "); ok {
			mode, content, _ := strings.Cut(rest, "\n")
			return mode, content
		}
	}
	t.Fatalf("%s not written; commands: %v", path, f.cmds)
	return "", ""
}

// A deploy writes the values exactly as stored, quoted for compose.
func TestDeployWritesQuotedEnv(t *testing.T) {
	f := newMoveFixture(t)
	for k, v := range map[string]string{"A": `p"w d$x#y`, "B": " spaced "} {
		if err := f.svc.SetSecret(f.app.ID, k, v); err != nil {
			t.Fatal(err)
		}
	}
	f.svc.Deploy(f.app.ID)
	_, env := f.written(t, fmt.Sprintf("/opt/dockify/apps/app-%d/.env", f.app.ID))
	if want := "A=\"p\\\"w d$$x#y\"\nB=\"spaced\"\n"; env != want {
		t.Fatalf(".env = %q, want %q", env, want)
	}
}

func TestValidators(t *testing.T) {
	for in, want := range map[string]string{"App.Example.com ": "app.example.com", "a-b.c-d.id": "a-b.c-d.id"} {
		if got, err := ValidDomain(in); err != nil || got != want {
			t.Errorf("ValidDomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"localhost", "a b.com", "a.com;id", "-a.com", "a-.com", "a..com", "a.com/x", "*.a.com", "a_b.com"} {
		if _, err := ValidDomain(in); err == nil {
			t.Errorf("ValidDomain(%q) accepted", in)
		}
	}

	for in, want := range map[string]string{"config.yml": "config.yml", "conf/./nginx.conf": "conf/nginx.conf", "a/b/../c.txt": "a/c.txt"} {
		if got, err := ValidFilePath(in); err != nil || got != want {
			t.Errorf("ValidFilePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", ".", "/etc/passwd", "../x", "a/../../x", "..", "a b.txt", "a'b", "$(id)"} {
		if _, err := ValidFilePath(in); err == nil {
			t.Errorf("ValidFilePath(%q) accepted", in)
		}
	}

	for _, k := range []string{"A", "_A1", "db_URL"} {
		if ValidEnvKey(k) != nil {
			t.Errorf("ValidEnvKey(%q) rejected", k)
		}
	}
	for _, k := range []string{"", "1A", "A-B", "A B", "A=B", "A\nB"} {
		if ValidEnvKey(k) == nil {
			t.Errorf("ValidEnvKey(%q) accepted", k)
		}
	}
}

// The API answers 400 naming the bad value instead of storing it.
func TestAPIRejectsInvalidInput(t *testing.T) {
	f := newMoveFixture(t)
	h := NewHandler(f.svc)
	r := chi.NewRouter()
	r.Post("/api/apps", h.Create)
	r.Post("/api/apps/{id}/secrets", h.SetSecret)
	r.Post("/api/apps/{id}/files", h.SetFile)
	r.Delete("/api/apps/{id}/files/{path}", h.DeleteFile)

	for _, tc := range []struct{ method, url, body, want string }{
		{"POST", "/api/apps", `{"name":"x","image":"nginx","domain":"bad domain.com","draft":true}`, `bad domain.com`},
		{"POST", "/api/apps", `{"name":"x","image":"nginx","domains":["ok.com","a.com;id"],"draft":true}`, `a.com;id`},
		{"POST", "/api/apps", `{"name":"x","image":"nginx","env_vars":[{"key":"A-B"}],"draft":true}`, `A-B`},
		{"POST", fmt.Sprintf("/api/apps/%d/secrets", f.app.ID), `{"key":"A=B","value":"x"}`, `A=B`},
		{"POST", fmt.Sprintf("/api/apps/%d/files", f.app.ID), `{"path":"../../etc/cron.d/x","content":"x"}`, `../../etc/cron.d/x`},
		{"DELETE", fmt.Sprintf("/api/apps/%d/files/..", f.app.ID), ``, `..`},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body)))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s %s %s: got %d %s", tc.method, tc.url, tc.body, rec.Code, rec.Body)
		}
	}
	if apps, _ := f.svc.List(); len(apps) != 1 {
		t.Errorf("invalid create stored an app: %d apps", len(apps))
	}
}

// The edit form re-renders with 400 and saves nothing on a bad domain, key or path.
func TestEditFormRejectsInvalidInput(t *testing.T) {
	f := newMoveFixture(t)
	h := NewWebHandler(f.svc, noServers{})
	for _, extra := range []url.Values{
		{"domain": {"bad domain.com"}},
		{"env_key": {"A-B"}, "env_val": {"x"}, "env_type": {"plain"}},
		{"file_path": {"../x"}, "file_content": {"x"}},
	} {
		form := url.Values{"name": {"renamed"}, "compose": {f.app.Compose}, "server_id": {fmt.Sprint(f.src.ID)}}
		for k, v := range extra {
			form[k] = v
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{URLParams: chi.RouteParams{Keys: []string{"id"}, Values: []string{fmt.Sprint(f.app.ID)}}}))
		status := 0
		h.AppEditForm(httptest.NewRecorder(), req, func(w http.ResponseWriter, r *http.Request, s int, name string, data interface{}) { status = s })
		if status != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", extra, status)
		}
	}
	if got, _ := f.repo.Get(f.app.ID); got.Name != "mongo" {
		t.Errorf("app saved despite invalid input: %q", got.Name)
	}
}
