package http

import (
	"net/http"
	"os"
	"strings"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/backup"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/settings"
	"github.com/coderbuzz/dockify/internal/webhook"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(svc *server.Service, appSvc *app.Service, render RenderFunc, serverListAdapter app.ServerRepo, cfgUser, cfgPass, sshKeyDir, webhookSecret string, settingsHandler *settings.Handler, backupHandler *backup.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(PrefixMiddleware)
	r.Use(CORSMiddleware)

	authEnabled := cfgPass != ""

	apiHandler := server.NewHandler(svc, sshKeyDir)
	webHandler := server.NewWebHandler(svc, sshKeyDir)

	appAPIHandler := app.NewHandler(appSvc)
	appWebHandler := app.NewWebHandler(appSvc, serverListAdapter)

	whHandler := webhook.NewHandler(appSvc, webhookSecret)

	// Public routes (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		if !authEnabled || getSessionUser(r) != "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		HandleLogin(w, r, cfgUser, cfgPass, render)
	})
	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		HandleLogin(w, r, cfgUser, cfgPass, render)
	})
	r.Post("/logout", HandleLogout)

	r.Post("/api/webhook/github", whHandler.GitHub)
	r.Post("/api/webhook/gitlab", whHandler.GitLab)

	staticDir := http.Dir("web/static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(staticDir)))

	// Protected routes (auth required)
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return AuthMiddleware(authEnabled, next)
		})

		r.Route("/api/servers", func(r chi.Router) {
			r.Get("/", apiHandler.List)
			r.Post("/", apiHandler.Create)
			r.Get("/{id}", apiHandler.Get)
			r.Patch("/{id}", apiHandler.Update)
			r.Delete("/{id}", apiHandler.Delete)
			r.Post("/{id}/init", apiHandler.Init)
			r.Post("/{id}/refresh", apiHandler.Refresh)
		})

		r.Route("/servers", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerListPage(w, r, render)
			})
			r.Get("/add", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerAddPage(w, r, render)
			})
			r.Post("/add", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerAddForm(w, r, render)
			})
			r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerDetailPage(w, r, render)
			})
			r.Get("/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerEditPage(w, r, render)
			})
			r.Post("/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerEditForm(w, r, render)
			})
			r.Post("/{id}/init", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerInit(w, r, render)
			})
			r.Post("/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerRefreshWeb(w, r, render)
			})
			r.Post("/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerDeleteWeb(w, r, render)
			})
			r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
				webHandler.ServerDeleteWeb(w, r, render)
			})
		})

		r.Route("/api/apps", func(r chi.Router) {
			r.Get("/", appAPIHandler.List)
			r.Post("/", appAPIHandler.Create)
			r.Get("/{id}", appAPIHandler.Get)
			r.Delete("/{id}", appAPIHandler.Delete)
			r.Post("/{id}/redeploy", appAPIHandler.Redeploy)
			r.Post("/{id}/rollback", appAPIHandler.Rollback)
			r.Post("/{id}/stop", appAPIHandler.Stop)
			r.Post("/{id}/start", appAPIHandler.Start)
			r.Get("/{id}/deployments", appAPIHandler.ListDeployments)
			r.Get("/{id}/logs", appAPIHandler.Logs)
			r.Get("/{id}/secrets", appAPIHandler.ListSecrets)
			r.Post("/{id}/secrets", appAPIHandler.SetSecret)
			r.Delete("/{id}/secrets/{key}", appAPIHandler.DeleteSecret)
			r.Get("/{id}/files", appAPIHandler.ListFiles)
			r.Post("/{id}/files", appAPIHandler.SetFile)
			r.Delete("/{id}/files/{path}", appAPIHandler.DeleteFile)
		})

		r.Get("/api/deployments/{id}", appAPIHandler.GetDeployment)

		r.Route("/apps", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppListPage(w, r, render)
			})
			r.Get("/add", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppAddPage(w, r, render)
			})
			r.Post("/add", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppAddForm(w, r, render)
			})
			r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppDetailPage(w, r, render)
			})
			r.Delete("/{id}/undeploy", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppDeleteWeb(w, r, render)
			})
			r.Post("/{id}/undeploy", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppDeleteWeb(w, r, render)
			})
			r.Post("/{id}/redeploy", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppRedeployWeb(w, r, render)
			})
			r.Post("/{id}/rollback", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppRollbackWeb(w, r, render)
			})
			r.Post("/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppStopWeb(w, r, render)
			})
			r.Post("/{id}/start", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppStartWeb(w, r, render)
			})
			r.Get("/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppEditPage(w, r, render)
			})
			r.Post("/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
				appWebHandler.AppEditForm(w, r, render)
			})
		})

		r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
			settingsHandler.SettingsPage(w, r, render)
		})
		r.Get("/api/settings/webhook-secret", settingsHandler.GetWebhookSecret)
		r.Post("/api/settings/webhook-secret/roll", settingsHandler.RollWebhookSecret)

		r.Get("/export", func(w http.ResponseWriter, r *http.Request) {
			backupHandler.ExportPage(w, r, render)
		})
		r.Get("/api/backup/export", backupHandler.ExportDownload)
		r.Get("/import", func(w http.ResponseWriter, r *http.Request) {
			backupHandler.ImportPage(w, r, render)
		})
		r.Post("/api/backup/import", func(w http.ResponseWriter, r *http.Request) {
			backupHandler.ImportUpload(w, r, render)
		})

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			stats := appSvc.DashboardStats()
			servers, _ := svc.List()
			apps, _ := appSvc.List()
			render(w, r, http.StatusOK, "dashboard.html", map[string]interface{}{
				"Title":   "Dashboard",
				"Stats":   stats,
				"Servers": servers,
				"Apps":    apps,
			})
		})
	})

	return r
}

func PrefixMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.Header.Get("X-Forwarded-Prefix")
		if prefix == "" {
			prefix = os.Getenv("DOCKIFY_BASE_PATH")
		}
		if prefix != "" {
			prefix = strings.TrimSuffix(prefix, "/") + "/"
			r = SetBasePath(r, prefix)
		}
		next.ServeHTTP(w, r)
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
