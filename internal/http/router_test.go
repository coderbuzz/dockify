package http

import (
	"net/http"
	"net/http/httptest"
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
