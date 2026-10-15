package backup

import (
	"reflect"
	"strings"
	"testing"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
)

func newBackupService(t *testing.T) (*Service, *app.Service, *server.Service) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	srvRepo := server.NewRepository(database)
	srvSvc := server.NewService(srvRepo)
	srvSvc.SetConnFactory(ssh.MockFactory())
	appSvc := app.NewService(app.NewRepository(database), srvRepo, nil, nil)
	appSvc.SetConnFactory(ssh.MockFactory())
	return NewService(srvSvc, appSvc, t.TempDir()), appSvc, srvSvc
}

func TestExportImportRoundTrip(t *testing.T) {
	src, srcApps, srcServers := newBackupService(t)
	svr := &server.Server{Name: "w1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "pending", Status: "online"}
	if err := srcServers.Create(svr); err != nil {
		t.Fatal(err)
	}
	want := app.App{
		Name: "web", ServerID: svr.ID, Domain: "a.example.com", Port: 8080, Compose: "services: {}",
		GitBranch: "main", Status: app.StatusRunning, MemoryLimit: "512m", CPULimit: "0.5", LogMaxSize: "10m",
		LogMaxFile: "3", Command: "serve --port 8080", Ports: "9000:9000", UlimitsNofile: "65536",
	}
	a := want
	if err := srcApps.Create(&a); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a.example.com", "b.example.com"} {
		if err := srcApps.SaveRoute(&app.Route{AppID: a.ID, ServerID: svr.ID, Domain: d}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := src.Export("pw")
	if err != nil {
		t.Fatal(err)
	}

	dst, dstApps, _ := newBackupService(t)
	if log, err := dst.Import(out, "pw", "merge"); err != nil {
		t.Fatalf("import: %v\n%s", err, log)
	}
	apps, _ := dstApps.List()
	if len(apps) != 1 {
		t.Fatalf("imported %d apps, want 1", len(apps))
	}
	got := apps[0]
	if got.MemoryLimit != want.MemoryLimit || got.CPULimit != want.CPULimit || got.LogMaxSize != want.LogMaxSize ||
		got.LogMaxFile != want.LogMaxFile || got.Command != want.Command || got.Ports != want.Ports ||
		got.UlimitsNofile != want.UlimitsNofile || got.Domain != want.Domain {
		t.Errorf("imported app %+v, want settings of %+v", got, want)
	}
	routes, _ := dstApps.GetRoutes(got.ID)
	var domains []string
	for _, rt := range routes {
		domains = append(domains, rt.Domain)
	}
	if !reflect.DeepEqual(domains, []string{"a.example.com", "b.example.com"}) {
		t.Errorf("routes = %v", domains)
	}

	// The source still runs its app: replace must refuse before touching anything.
	if _, err := src.Import(out, "pw", "replace"); err == nil || !strings.Contains(err.Error(), "stop or undeploy running apps before a replace import") {
		t.Fatalf("replace with a running app: err = %v", err)
	}
	if apps, _ := srcApps.List(); len(apps) != 1 {
		t.Errorf("replace refusal changed the apps: %d left", len(apps))
	}
}
