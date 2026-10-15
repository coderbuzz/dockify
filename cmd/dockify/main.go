package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/backup"
	"github.com/coderbuzz/dockify/internal/cloudflare"
	"github.com/coderbuzz/dockify/internal/config"
	"github.com/coderbuzz/dockify/internal/db"
	httppkg "github.com/coderbuzz/dockify/internal/http"
	"github.com/coderbuzz/dockify/internal/scheduler"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/settings"
	"github.com/coderbuzz/dockify/internal/ssh"
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
	if err := cfg.CheckAuth(); err != nil {
		log.Fatal(err)
	}

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	appRepo := app.NewRepository(database)
	if n, err := appRepo.FailInterrupted(); err != nil {
		log.Fatalf("failed to recover interrupted deploys: %v", err)
	} else if n > 0 {
		log.Printf("Marked %d app(s) failed: deploy interrupted by restart", n)
	}

	// Cancelled on SIGINT/SIGTERM: stops the background loops, then the server.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverRepo := server.NewRepository(database)
	svc := server.NewService(serverRepo)
	svc.StartMonitor(ctx)

	var cfClient *cloudflare.Client
	if cfg.CloudflareAPIToken != "" && cfg.CloudflareZoneID != "" {
		cfClient = cloudflare.New(cfg.CloudflareAPIToken, cfg.CloudflareZoneID)
		log.Println("Cloudflare DNS integration enabled")
	} else {
		log.Println("Cloudflare DNS not configured (set CLOUDFLARE_API_TOKEN and CLOUDFLARE_ZONE_ID)")
	}

	sch := scheduler.New(serverRepo)

	appSvc := app.NewService(appRepo, serverRepo, cfClient, sch)

	if cfg.DevMock {
		svc.SetConnFactory(ssh.MockFactory())
		appSvc.SetConnFactory(ssh.MockFactory())
		log.Println("DEV MOCK MODE: using mock SSH client")
	}

	appSvc.StartStatsCollector(ctx)

	settingsSvc := settings.NewService(database, version)
	settingsHandler := settings.NewHandler(settingsSvc)

	backupSvc := backup.NewService(svc, appSvc, cfg.SSHKeyDir)
	backupHandler := backup.NewHandler(backupSvc)

	serverListAdapter := &serverLister{svc: svc}

	webhookSecretFn := func() string { s, _ := settingsSvc.GetWebhookSecret(); return s }
	router := httppkg.NewRouter(svc, appSvc, httppkg.Render, serverListAdapter, cfg.AdminUser, cfg.AdminPass, cfg.APIToken, cfg.SSHKeyDir, webhookSecretFn, settingsHandler, backupHandler, version, cfg.DevMock, cfg.TrustProxy)

	if cfg.AdminPass == "" {
		log.Println("WARNING: No admin password set (DOCKIFY_ADMIN_PASSWORD). Web UI has no authentication.")
	} else {
		log.Println("Web UI authentication enabled")
	}

	// No WriteTimeout: console and stats websockets, prune and import run long.
	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Dockify v%s starting on %s", version, cfg.Addr())
		log.Printf("Open: http://localhost:%s", cfg.Port)
		log.Printf("Data dir: %s", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	// Websockets are hijacked and not waited for; deploys still running are
	// cut off and marked failed by FailInterrupted on the next start.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown: %v", err)
	}
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
			Host:   s.Host,
			Status: s.Status,
		}
	}
	return infos, nil
}
