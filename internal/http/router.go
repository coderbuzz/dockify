package http

import (
	"net/http"

	"github.com/coderbuzz/dockify/internal/app"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/webhook"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(svc *server.Service, appSvc *app.Service, render RenderFunc, serverListAdapter app.ServerRepo) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(CORSMiddleware)

	apiHandler := server.NewHandler(svc)
	webHandler := server.NewWebHandler(svc)

	appAPIHandler := app.NewHandler(appSvc)
	appWebHandler := app.NewWebHandler(appSvc, serverListAdapter)

	r.Route("/api/servers", func(r chi.Router) {
		r.Get("/", apiHandler.List)
		r.Post("/", apiHandler.Create)
		r.Get("/{id}", apiHandler.Get)
		r.Delete("/{id}", apiHandler.Delete)
		r.Post("/{id}/init", apiHandler.Init)
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
		r.Post("/{id}/init", func(w http.ResponseWriter, r *http.Request) {
			webHandler.ServerInit(w, r, render)
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
		r.Get("/{id}/deployments", appAPIHandler.ListDeployments)
		r.Get("/{id}/logs", appAPIHandler.Logs)
	})

	r.Get("/api/deployments/{id}", appAPIHandler.GetDeployment)

	whHandler := webhook.NewHandler(appSvc)
	r.Post("/api/webhook/github", whHandler.GitHub)
	r.Post("/api/webhook/gitlab", whHandler.GitLab)

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

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	staticDir := http.Dir("web/static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(staticDir)))

	return r
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
