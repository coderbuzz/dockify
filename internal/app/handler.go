package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	apps, err := h.service.List()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, apps)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string `json:"name"`
		ServerID  int64  `json:"server_id"`
		Domain    string `json:"domain"`
		Port      int    `json:"port"`
		Compose   string `json:"compose"`
		Image     string `json:"image"`
		EnvVars   string `json:"env_vars"`
		Volumes   string `json:"volumes"`
		GitRepo   string `json:"git_repo"`
		GitBranch string `json:"git_branch"`
		AuthUser  string `json:"auth_user"`
		AuthPass  string `json:"auth_pass"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if input.Name == "" || input.Domain == "" || input.Port == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "name, domain, and port are required"})
		return
	}

	compose := input.Compose
	if compose == "" && input.Image != "" {
		compose = generateCompose(input.Image, input.Port, input.EnvVars, input.Volumes)
	}
	if compose == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "provide either compose or image"})
		return
	}

	if input.ServerID == 0 {
		id, err := h.service.PickServerID()
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "auto-select failed: " + err.Error()})
			return
		}
		input.ServerID = id
	}

	app := &App{
		Name:      input.Name,
		ServerID:  input.ServerID,
		Domain:    input.Domain,
		Port:      input.Port,
		Compose:   compose,
		GitRepo:   input.GitRepo,
		GitBranch: input.GitBranch,
		AuthUser:  input.AuthUser,
		AuthPass:  input.AuthPass,
	}

	if err := h.service.Create(app); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go h.service.Deploy(app.ID)

	jsonResponse(w, http.StatusCreated, app)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	app, err := h.service.Get(id)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if app == nil {
		jsonResponse(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return
	}

	jsonResponse(w, http.StatusOK, app)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	go h.service.Undeploy(id)

	jsonResponse(w, http.StatusAccepted, map[string]string{"message": "undeploy started"})
}

func (h *Handler) Redeploy(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	go h.service.Redeploy(id)

	jsonResponse(w, http.StatusAccepted, map[string]string{"message": "redeploy started"})
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Rollback(id); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusAccepted, map[string]string{"message": "rollback started"})
}

func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	deps, err := h.service.ListDeployments(id)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if deps == nil {
		deps = []Deployment{}
	}

	jsonResponse(w, http.StatusOK, deps)
}

func (h *Handler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	dep, err := h.service.GetDeployment(id)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if dep == nil {
		jsonResponse(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}

	jsonResponse(w, http.StatusOK, dep)
}

func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 100
	}

	logs, err := h.service.FetchLogs(id, tail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(logs))
}

type WebHandler struct {
	service    *Service
	serverRepo ServerRepo
}

type ServerRepo interface {
	List() ([]ServerInfo, error)
}

type ServerInfo struct {
	ID     int64
	Name   string
	Status string
}

func NewWebHandler(service *Service, serverRepo ServerRepo) *WebHandler {
	return &WebHandler{service: service, serverRepo: serverRepo}
}

func (h *WebHandler) AppListPage(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	apps, err := h.service.List()
	if err != nil {
		apps = nil
	}

	render(w, r, http.StatusOK, "apps.html", map[string]interface{}{
		"Title": "Apps",
		"Apps":  apps,
	})
}

func (h *WebHandler) AppAddPage(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	servers, err := h.serverRepo.List()
	if err != nil {
		servers = nil
	}

	render(w, r, http.StatusOK, "apps_add.html", map[string]interface{}{
		"Title":   "Deploy App",
		"Servers": servers,
	})
}

func (h *WebHandler) AppAddForm(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusBadRequest, "_error", map[string]interface{}{
			"Error": "invalid form data",
		})
		return
	}

	serverID, _ := strconv.ParseInt(r.FormValue("server_id"), 10, 64)
	port, _ := strconv.Atoi(r.FormValue("port"))
	gitBranch := strings.TrimSpace(r.FormValue("git_branch"))
	if gitBranch == "" {
		gitBranch = "main"
	}

	compose := strings.TrimSpace(r.FormValue("compose"))
	image := strings.TrimSpace(r.FormValue("image"))
	envVars := strings.TrimSpace(r.FormValue("env_vars"))
	volumes := strings.TrimSpace(r.FormValue("volumes"))

	if compose == "" && image != "" {
		compose = generateCompose(image, port, envVars, volumes)
	}

	app := &App{
		Name:      strings.TrimSpace(r.FormValue("name")),
		ServerID:  serverID,
		Domain:    strings.TrimSpace(r.FormValue("domain")),
		Port:      port,
		Compose:   compose,
		GitRepo:   strings.TrimSpace(r.FormValue("git_repo")),
		GitBranch: gitBranch,
		AuthUser:  strings.TrimSpace(r.FormValue("auth_user")),
		AuthPass:  strings.TrimSpace(r.FormValue("auth_pass")),
	}

	if app.Name == "" || app.Domain == "" || app.Port == 0 || app.Compose == "" {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusBadRequest, "apps_add.html", map[string]interface{}{
			"Title":   "Deploy App",
			"Servers": servers,
			"Error":   "name, domain, port, and either compose or image are required",
		})
		return
	}

	if app.ServerID == 0 {
		id, err := h.service.PickServerID()
		if err != nil {
			servers, _ := h.serverRepo.List()
			render(w, r, http.StatusBadRequest, "apps_add.html", map[string]interface{}{
				"Title":   "Deploy App",
				"Servers": servers,
				"Error":   "auto-select failed: " + err.Error(),
			})
			return
		}
		app.ServerID = id
	}

	if err := h.service.Create(app); err != nil {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusInternalServerError, "apps_add.html", map[string]interface{}{
			"Title":   "Deploy App",
			"Servers": servers,
			"Error":   err.Error(),
		})
		return
	}

	go h.service.Deploy(app.ID)

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(app.ID, 10), http.StatusSeeOther)
}

func (h *WebHandler) AppDetailPage(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	app, err := h.service.Get(id)
	if err != nil {
		render(w, r, http.StatusInternalServerError, "error.html", map[string]interface{}{"Message": err.Error()})
		return
	}
	if app == nil {
		render(w, r, http.StatusNotFound, "error.html", map[string]interface{}{"Message": "app not found"})
		return
	}

	deps, err := h.service.ListDeployments(id)
	if err != nil {
		deps = nil
	}

	render(w, r, http.StatusOK, "apps_detail.html", map[string]interface{}{
		"Title":       app.Name,
		"App":         app,
		"Deployments": deps,
	})
}

func (h *WebHandler) AppDeleteWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	go h.service.Undeploy(id)

	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

func (h *WebHandler) AppRedeployWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	go h.service.Redeploy(id)

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *WebHandler) AppRollbackWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Rollback(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

type RenderFunc = func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
