package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/settings"
)

type noServers struct{}

func (noServers) List() ([]app.ServerInfo, error) { return nil, nil }

// Secrets are never rendered into HTML; the edit form keeps stored values it did not change (A20).
func TestSecretsNotInFormHTML(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	srvRepo := server.NewRepository(database)
	appSvc := app.NewService(app.NewRepository(database), srvRepo, nil, nil)
	setSvc := settings.NewService(database, "test")
	r := NewRouter(server.NewService(srvRepo), appSvc, Render, noServers{}, "admin", "", "", t.TempDir(), func() string { return "" }, settings.NewHandler(setSvc), nil, "test", true, false)
	do := func(method, path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	whSecret, _ := setSvc.GetWebhookSecret()
	if rec := do(http.MethodGet, "/settings", nil); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), whSecret) {
		t.Errorf("/settings: %d, contains webhook secret %v", rec.Code, strings.Contains(rec.Body.String(), whSecret))
	}
	if rec := do(http.MethodGet, "/api/settings/webhook-secret", nil); !strings.Contains(rec.Body.String(), whSecret) {
		t.Errorf("API must still return the secret: %d %s", rec.Code, rec.Body)
	}

	a := &app.App{Name: "web", Compose: "services: {}", Status: app.StatusDraft, AuthUser: "u", AuthPass: "old-pass-123"}
	if err := appSvc.Create(a); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct {
		k, v   string
		secret bool
	}{{"API_KEY", "secret-a-123", true}, {"TOKEN", "secret-c-123", true}, {"DROP", "secret-d-123", true}, {"PLAIN", "plain-b", false}} {
		if err := appSvc.SetSecretWithType(a.ID, s.k, s.v, s.secret); err != nil {
			t.Fatal(err)
		}
	}
	editPath := "/apps/" + strconv.FormatInt(a.ID, 10) + "/edit"
	page := do(http.MethodGet, editPath, nil).Body.String()
	for _, v := range []string{"old-pass-123", "secret-a-123", "secret-c-123", "secret-d-123"} {
		if strings.Contains(page, v) {
			t.Errorf("edit page contains %q", v)
		}
	}
	if !strings.Contains(page, "plain-b") || !strings.Contains(page, `placeholder="unchanged"`) {
		t.Errorf("edit page: plain value or unchanged placeholder missing")
	}

	// Unchanged password and masked rows: API_KEY renamed, TOKEN made plain, DROP removed, PLAIN kept.
	form := url.Values{
		"name": {"web"}, "mode": {"advanced"}, "compose": {"services: {}"}, "draft": {"1"},
		"auth_user": {"u"}, "auth_pass": {""},
		"env_key":  {"API_KEY2", "TOKEN", "PLAIN"},
		"env_val":  {"", "", "plain-b"},
		"env_type": {"secret", "plain", "plain"},
		"env_orig": {"API_KEY", "TOKEN", ""},
	}
	if rec := do(http.MethodPost, editPath, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	got, _ := appSvc.Get(a.ID)
	if got.AuthPass != "old-pass-123" {
		t.Errorf("AuthPass = %q, want the old one", got.AuthPass)
	}
	list, _ := appSvc.ListSecrets(a.ID)
	m := map[string]app.AppSecret{}
	for _, s := range list {
		m[s.Key] = s
	}
	if s := m["API_KEY2"]; s.Value != "secret-a-123" || !s.IsSecret {
		t.Errorf("renamed masked secret: %+v", s)
	}
	if s := m["TOKEN"]; s.Value != "secret-c-123" || s.IsSecret {
		t.Errorf("masked secret made plain: %+v", s)
	}
	if _, ok := m["API_KEY"]; ok || len(m) != 3 {
		t.Errorf("secrets after edit: %+v", m)
	}

	dup := url.Values{}
	for k, v := range form {
		dup[k] = v
	}
	dup["env_key"] = []string{"PLAIN", "TOKEN", "PLAIN"}
	if rec := do(http.MethodPost, editPath, dup); rec.Code != http.StatusBadRequest {
		t.Errorf("duplicate env key: %d, want 400", rec.Code)
	}

	form.Set("auth_pass", "new-pass")
	do(http.MethodPost, editPath, form)
	if got, _ = appSvc.Get(a.ID); got.AuthPass != "new-pass" {
		t.Errorf("AuthPass = %q, want new-pass", got.AuthPass)
	}
	form.Set("auth_user", "")
	form.Set("auth_pass", "")
	do(http.MethodPost, editPath, form)
	if got, _ = appSvc.Get(a.ID); got.AuthPass != "" || got.AuthUser != "" {
		t.Errorf("empty username must remove basic auth: %q/%q", got.AuthUser, got.AuthPass)
	}
}
