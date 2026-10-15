package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/go-chi/chi/v5"
)

// newTestRouter builds the real router on an in-memory DB, auth disabled.
func newTestRouter(t *testing.T) (*chi.Mux, *app.Service, *server.Service) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	srvRepo := server.NewRepository(database)
	svc := server.NewService(srvRepo)
	appSvc := app.NewService(app.NewRepository(database), srvRepo, nil, nil)
	return NewRouter(svc, appSvc, Render, nil, "admin", "", "", t.TempDir(), func() string { return "" }, nil, nil, "test", true), appSvc, svc
}

// Vendored assets are served from the binary with the security headers; no listing (B20, S12).
func TestStaticAssetsAndCSP(t *testing.T) {
	r, _, _ := newTestRouter(t)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	for _, f := range []string{"htmx.min.js", "xterm.min.js", "xterm.min.css", "xterm-addon-fit.min.js"} {
		rec := get("/static/" + f)
		if rec.Code != http.StatusOK || rec.Body.Len() < 1000 || rec.Header().Get("Cache-Control") == "" {
			t.Errorf("%s: %d, %d bytes, Cache-Control %q", f, rec.Code, rec.Body.Len(), rec.Header().Get("Cache-Control"))
		}
	}
	for _, p := range []string{"/static/", "/static/SOURCES.md", "/static/../router.go"} {
		if rec := get(p); rec.Code == http.StatusOK {
			t.Errorf("%s: got 200 %q", p, rec.Body.String()[:min(80, rec.Body.Len())])
		}
	}
	rec := get("/login")
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("security headers missing: %v", rec.Header())
	}
}

func TestDeleteNestedFilePath(t *testing.T) {
	r, appSvc, svc := newTestRouter(t)
	svr := &server.Server{Name: "w1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "pending", Status: "online"}
	if err := svc.Create(svr); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Name: "web", ServerID: svr.ID, Compose: "services: {}"}
	if err := appSvc.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := appSvc.SetFile(a.ID, "conf/app.ini", "x=1"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/apps/1/files/conf/app.ini", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE nested file: %d %s", rec.Code, rec.Body)
	}
	if files, _ := appSvc.ListFiles(a.ID); len(files) != 0 {
		t.Errorf("file still there: %+v", files)
	}
}
