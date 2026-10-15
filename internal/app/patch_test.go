package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// patchApp sends PATCH /api/apps/{id} through a synchronous handler.
func patchApp(t *testing.T, f *moveFixture, id int64, body string) (int, map[string]interface{}) {
	t.Helper()
	h := NewHandler(f.svc)
	h.run = func(fn func()) { fn() }
	r := chi.NewRouter()
	r.Patch("/api/apps/{id}", h.Patch)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/apps/%d", id), strings.NewReader(body)))
	var out map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func withSecrets(t *testing.T, f *moveFixture) {
	t.Helper()
	for k, v := range map[string]string{"MONGO_INITDB_ROOT_PASSWORD": "s3cret", "TZ": "Asia/Jakarta"} {
		if err := f.svc.SetSecret(f.app.ID, k, v); err != nil {
			t.Fatal(err)
		}
	}
}

func assertSecretsKept(t *testing.T, f *moveFixture) {
	t.Helper()
	secrets, _ := f.svc.ListSecrets(f.app.ID)
	got := map[string]string{}
	for _, s := range secrets {
		got[s.Key] = s.Value
	}
	if len(got) != 2 || got["MONGO_INITDB_ROOT_PASSWORD"] != "s3cret" || got["TZ"] != "Asia/Jakarta" {
		t.Errorf("env vars changed: %v", got)
	}
}

func TestPatchComposeKeepsEnvAndRedeploys(t *testing.T) {
	f := newMoveFixture(t)
	withSecrets(t, f)
	compose := "services:\n  mongo:\n    image: mongo:4\n    mem_limit: 2g"

	code, out := patchApp(t, f, f.app.ID, fmt.Sprintf(`{"compose": %q}`, compose))
	if code != http.StatusAccepted || out["action"] != "redeploy" {
		t.Fatalf("got %d %v", code, out)
	}
	got, _ := f.repo.Get(f.app.ID)
	if got.Compose != compose || got.ComposeMode != "advanced" || got.ServerID != f.src.ID {
		t.Errorf("app not updated as expected: %+v", got)
	}
	assertSecretsKept(t, f)
	if !f.ranOn(f.src, "up -d") || f.ranOn(f.dst, "up -d") {
		t.Errorf("expected a redeploy on the source only; commands: %v", f.cmds)
	}
}

func TestPatchMoveWithFiles(t *testing.T) {
	f := newMoveFixture(t)
	f.direct = true
	withSecrets(t, f)

	code, out := patchApp(t, f, f.app.ID, fmt.Sprintf(`{"server_id": %d, "move_files": true}`, f.dst.ID))
	if code != http.StatusAccepted || out["action"] != "move" {
		t.Fatalf("got %d %v", code, out)
	}
	got, _ := f.repo.Get(f.app.ID)
	if got.ServerID != f.dst.ID || got.Status != StatusRunning {
		t.Fatalf("want running on %d, got %q on %d", f.dst.ID, got.Status, got.ServerID)
	}
	routes, _ := f.repo.GetRoutes(f.app.ID)
	if len(routes) != 1 || routes[0].ServerID != f.dst.ID {
		t.Errorf("routes not moved: %+v", routes)
	}
	assertSecretsKept(t, f)
	if !f.ranOn(f.src, "PIPE") || f.ranOn(f.src, "rm -rf /opt") {
		t.Errorf("want files copied and old folder kept; commands: %v", f.cmds)
	}
}

func TestPatchMoveWithoutFiles(t *testing.T) {
	f := newMoveFixture(t)
	withSecrets(t, f)

	code, out := patchApp(t, f, f.app.ID, fmt.Sprintf(`{"server_id": %d}`, f.dst.ID))
	if code != http.StatusAccepted || out["action"] != "move_without_files" {
		t.Fatalf("got %d %v", code, out)
	}
	got, _ := f.repo.Get(f.app.ID)
	if got.ServerID != f.dst.ID {
		t.Errorf("server_id = %d, want %d", got.ServerID, f.dst.ID)
	}
	routes, _ := f.repo.GetRoutes(f.app.ID)
	if len(routes) != 1 || routes[0].ServerID != f.dst.ID {
		t.Errorf("routes not moved: %+v", routes)
	}
	assertSecretsKept(t, f)
	if f.ranOn(f.src, "PIPE") || !f.ranOn(f.dst, "up -d") || !f.ranOn(f.src, " down") || f.ranOn(f.src, "rm -rf /opt") {
		t.Errorf("want deploy on target, old stopped not purged, no copy; commands: %v", f.cmds)
	}
}

func TestPatchRejectsBadInputWithoutChanges(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"unknown field":           {`{"server": 2}`, http.StatusBadRequest},
		"env not patchable":       {`{"env_vars": []}`, http.StatusBadRequest},
		"empty compose":           {`{"compose": "  "}`, http.StatusBadRequest},
		"move_files without move": {`{"move_files": true}`, http.StatusBadRequest},
		"unknown server":          {`{"server_id": 999, "move_files": true}`, http.StatusBadRequest},
		"not json":                {`server_id=2`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMoveFixture(t)
			before, _ := f.repo.Get(f.app.ID)
			code, out := patchApp(t, f, f.app.ID, c.body)
			if code != c.want {
				t.Fatalf("got %d %v, want %d", code, out, c.want)
			}
			after, _ := f.repo.Get(f.app.ID)
			if after.Compose != before.Compose || after.ServerID != before.ServerID || len(f.cmds) != 0 {
				t.Errorf("app changed or commands ran: %+v %v", after, f.cmds)
			}
		})
	}

	f := newMoveFixture(t)
	if code, _ := patchApp(t, f, 4242, `{}`); code != http.StatusNotFound {
		t.Errorf("missing app: got %d, want 404", code)
	}
}

// While a deploy (or move) holds the app's lock, every lifecycle request is
// refused with 409 and nothing runs on the worker.
func TestLifecycleRefusedWhileBusy(t *testing.T) {
	f := newMoveFixture(t)
	h := NewHandler(f.svc)
	h.run = func(fn func()) { fn() }
	r := chi.NewRouter()
	r.Post("/api/apps/{id}/redeploy", h.Redeploy)
	r.Post("/api/apps/{id}/stop", h.Stop)
	r.Post("/api/apps/{id}/start", h.Start)
	r.Post("/api/apps/{id}/rollback", h.Rollback)
	r.Delete("/api/apps/{id}", h.Delete)
	do := func(method, path string) (int, string) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, fmt.Sprintf("/api/apps/%d%s", f.app.ID, path), nil))
		return rec.Code, rec.Body.String()
	}

	unlock := f.svc.lock(f.app.ID)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/redeploy"}, {http.MethodPost, "/stop"}, {http.MethodPost, "/start"},
		{http.MethodPost, "/rollback"}, {http.MethodDelete, ""},
	} {
		if code, body := do(c.method, c.path); code != http.StatusConflict || !strings.Contains(body, "operation in progress") {
			t.Errorf("%s %s: got %d %s, want 409", c.method, c.path, code, body)
		}
	}
	if code, _ := patchApp(t, f, f.app.ID, `{"compose": "services: {}"}`); code != http.StatusConflict {
		t.Errorf("patch: got %d, want 409", code)
	}
	if len(f.cmds) != 0 {
		t.Errorf("commands ran while busy: %v", f.cmds)
	}
	unlock()

	if code, body := do(http.MethodPost, "/redeploy"); code != http.StatusAccepted || !f.ranOn(f.src, "up -d") {
		t.Errorf("redeploy after unlock: got %d %s; commands: %v", code, body, f.cmds)
	}
	if f.svc.Busy(f.app.ID) {
		t.Error("lock still held after the redeploy finished")
	}
}

func TestSetSecretIsSecretFlag(t *testing.T) {
	f := newMoveFixture(t)
	h := NewHandler(f.svc)
	r := chi.NewRouter()
	r.Post("/api/apps/{id}/secrets", h.SetSecret)
	post := func(body string) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/apps/%d/secrets", f.app.ID), strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d %s", body, rec.Code, rec.Body)
		}
	}
	flag := func() bool {
		secrets, _ := f.svc.ListSecrets(f.app.ID)
		return len(secrets) == 1 && secrets[0].IsSecret
	}
	post(`{"key": "PW", "value": "a", "is_secret": true}`)
	if !flag() {
		t.Fatal("is_secret true not stored")
	}
	post(`{"key": "PW", "value": "b"}`)
	if !flag() {
		t.Fatal("omitting is_secret must keep the stored flag")
	}
	post(`{"key": "PW", "value": "c", "is_secret": false}`)
	if flag() {
		t.Fatal("is_secret false not stored")
	}
}

type noServers struct{}

func (noServers) List() ([]ServerInfo, error) { return nil, nil }

// B22: saving the edit form used to add the primary domain twice per save.
func TestEditFormKeepsOneRoutePerDomain(t *testing.T) {
	f := newMoveFixture(t)
	f.repo.UpdateStatus(f.app.ID, StatusDraft) // no background deploy
	h := NewWebHandler(f.svc, noServers{})
	r := chi.NewRouter()
	r.Post("/apps/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		h.AppEditForm(w, r, func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{}) {
			t.Fatalf("form rendered %s (%d): %v", name, status, data)
		})
	})
	edit := func(domains ...string) {
		form := url.Values{"name": {"mongo"}, "compose": {f.app.Compose}, "server_id": {fmt.Sprint(f.src.ID)}, "domain": domains}
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/apps/%d/edit", f.app.ID), strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("edit: %d %s", rec.Code, rec.Body)
		}
	}
	domainsOf := func() []string {
		routes, _ := f.repo.GetRoutes(f.app.ID)
		var out []string
		for _, rt := range routes {
			out = append(out, rt.Domain)
		}
		sort.Strings(out)
		return out
	}

	// A new primary domain was saved by the app.Domain block and the loop.
	edit("www.example.com", "db.example.com")
	edit("www.example.com", "db.example.com")
	if got := domainsOf(); !reflect.DeepEqual(got, []string{"db.example.com", "www.example.com"}) {
		t.Fatalf("routes after two identical edits: %v", got)
	}
	edit("db.example.com")
	if got := domainsOf(); !reflect.DeepEqual(got, []string{"db.example.com"}) {
		t.Fatalf("routes after removing a domain: %v", got)
	}
}
