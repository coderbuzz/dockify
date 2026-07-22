package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/backup"
	"github.com/coderbuzz/dockify/internal/cloudflare"
	"github.com/coderbuzz/dockify/internal/config"
	"github.com/coderbuzz/dockify/internal/db"
	httppkg "github.com/coderbuzz/dockify/internal/http"
	"github.com/coderbuzz/dockify/internal/scheduler"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/coderbuzz/dockify/internal/settings"
)

var version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("Dockify v" + version)
			return
		case "serve":
		default:
			fmt.Println("Usage: dockify [serve|version]")
			fmt.Println("  serve    Start the Dockify server (default)")
			fmt.Println("  version  Print version")
			return
		}
	}

	cfg := config.Load()

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	serverRepo := server.NewRepository(database)
	svc := server.NewService(serverRepo)
	svc.StartMonitor()

	var cfClient *cloudflare.Client
	if cfg.CloudflareAPIToken != "" && cfg.CloudflareZoneID != "" {
		cfClient = cloudflare.New(cfg.CloudflareAPIToken, cfg.CloudflareZoneID)
		log.Println("Cloudflare DNS integration enabled")
	} else {
		log.Println("Cloudflare DNS not configured (set CLOUDFLARE_API_TOKEN and CLOUDFLARE_ZONE_ID)")
	}

	sch := scheduler.New(serverRepo)

	appRepo := app.NewRepository(database)
	appSvc := app.NewService(appRepo, serverRepo, cfClient, sch)

	if cfg.DevMock {
		svc.SetConnFactory(ssh.MockFactory())
		appSvc.SetConnFactory(ssh.MockFactory())
		log.Println("DEV MOCK MODE: using mock SSH client")
	}

	appSvc.StartStatsCollector()

	settingsSvc := settings.NewService(database, version)
	settingsHandler := settings.NewHandler(settingsSvc)

	backupSvc := backup.NewService(svc, appSvc, cfg.SSHKeyDir)
	backupHandler := backup.NewHandler(backupSvc)

	serverListAdapter := &serverLister{svc: svc}

	webhookSecretFn := func() string { s, _ := settingsSvc.GetWebhookSecret(); return s }
	router := httppkg.NewRouter(svc, appSvc, httppkg.Render, serverListAdapter, cfg.AdminUser, cfg.AdminPass, cfg.SSHKeyDir, webhookSecretFn, settingsHandler, backupHandler, version, cfg.DevMock)

	if cfg.AdminPass == "" {
		log.Println("WARNING: No admin password set (DOCKIFY_ADMIN_PASSWORD). Web UI has no authentication.")
	} else {
		log.Println("Web UI authentication enabled")
	}

	srv := &http.Server{
		Addr:    cfg.Addr(),
		Handler: router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Dockify v%s starting on %s", version, cfg.Addr())
		log.Printf("Open: http://localhost:%s", cfg.Port)
		log.Printf("Data dir: %s", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")
	srv.Close()
}

type serverLister struct {
	svc *server.Service
}

func (a *serverLister) List() ([]app.ServerInfo, error) {
	servers, err := a.svc.List()
	if err != nil {
		return nil, err
	}
	infos := make([]app.ServerInfo, len(servers))
	for i, s := range servers {
		infos[i] = app.ServerInfo{
			ID:     s.ID,
			Name:   s.Name,
			Status: s.Status,
		}
	}
	return infos, nil
}
