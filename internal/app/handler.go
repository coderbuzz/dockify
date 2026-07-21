package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
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

type envVarInput struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string        `json:"name"`
		ServerID    int64         `json:"server_id"`
		Domain      string        `json:"domain"`
		Domains     []string      `json:"domains"`
		Port        int           `json:"port"`
		Compose     string        `json:"compose"`
		Image       string        `json:"image"`
		EnvVars     []envVarInput `json:"env_vars"`
		Volumes     string        `json:"volumes"`
		GitRepo     string        `json:"git_repo"`
		GitBranch   string        `json:"git_branch"`
		AuthUser    string        `json:"auth_user"`
		AuthPass    string        `json:"auth_pass"`
		MemoryLimit string        `json:"memory_limit"`
		CPULimit    string        `json:"cpu_limit"`
		LogMaxSize  string        `json:"log_max_size"`
		LogMaxFile  string        `json:"log_max_file"`
		Command     string        `json:"command"`
		Ports       string        `json:"ports"`
		UlimitsNofile string       `json:"ulimits_nofile"`
		Draft       bool          `json:"draft"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if input.Name == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	envKeys := make([]string, 0, len(input.EnvVars))
	for _, e := range input.EnvVars {
		if strings.TrimSpace(e.Key) != "" {
			envKeys = append(envKeys, strings.TrimSpace(e.Key))
		}
	}

	compose := input.Compose
	if compose == "" && input.Image != "" {
		compose = generateCompose(input.Image, input.Port, input.Volumes, input.Name, input.MemoryLimit, input.CPULimit, input.LogMaxSize, input.LogMaxFile, envKeys, input.Command, input.Ports, input.UlimitsNofile)
	}
	if compose == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "provide either compose or image"})
		return
	}

	if input.ServerID == 0 && !input.Draft {
		id, err := h.service.PickServerID()
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "auto-select failed: " + err.Error()})
			return
		}
		input.ServerID = id
	}

	composeMode := "advanced"
	if input.Image != "" && (input.Compose == "" || !strings.Contains(input.Compose, "services:")) {
		composeMode = "simple"
	}
	app := &App{
		Name:        input.Name,
		ServerID:    input.ServerID,
		Domain:      input.Domain,
		Port:        input.Port,
		Compose:     compose,
		GitRepo:     input.GitRepo,
		GitBranch:   input.GitBranch,
		AuthUser:    input.AuthUser,
		AuthPass:    input.AuthPass,
		ComposeMode: composeMode,
		MemoryLimit: input.MemoryLimit,
		CPULimit:    input.CPULimit,
		LogMaxSize:  input.LogMaxSize,
		LogMaxFile:  input.LogMaxFile,
		Command:     input.Command,
		Ports:       input.Ports,
		UlimitsNofile: input.UlimitsNofile,
	}
	if input.Draft {
		app.Status = StatusDraft
	}

	if err := h.service.Create(app); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for _, e := range input.EnvVars {
		if strings.TrimSpace(e.Key) == "" {
			continue
		}
		h.service.SetSecretWithType(app.ID, strings.TrimSpace(e.Key), e.Value, e.IsSecret)
	}

	allDomains := []string{app.Domain}
	for _, d := range input.Domains {
		d = strings.TrimSpace(d)
		if d != "" && d != app.Domain {
			allDomains = append(allDomains, d)
		}
	}
	seen := map[string]bool{}
	for _, d := range allDomains {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		h.service.SaveRoute(&Route{
			AppID:    app.ID,
			ServerID: app.ServerID,
			Domain:   d,
			Target:   "",
		})
	}

	if !input.Draft {
		go h.service.Deploy(app.ID)
	}

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

func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	app, _ := h.service.Get(id)
	if app != nil && app.Status == StatusDraft {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "cannot stop a draft — deploy it first"})
		return
	}

	if err := h.service.Stop(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "stopped"})
}

func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	app, _ := h.service.Get(id)
	if app != nil && app.Status == StatusDraft {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "cannot start a draft — deploy it first"})
		return
	}

	if err := h.service.Start(id); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "started"})
}

func (h *Handler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	secrets, err := h.service.ListSecrets(id)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, secrets)
}

func (h *Handler) SetSecret(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var input struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Key == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}
	if err := h.service.SetSecret(id, input.Key, input.Value); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	key := chi.URLParam(r, "key")
	if err := h.service.DeleteSecret(id, key); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	files, err := h.service.ListFiles(id)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, files)
}

func (h *Handler) SetFile(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Path == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	if err := h.service.SetFile(id, input.Path, input.Content); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	path := chi.URLParam(r, "path")
	if err := h.service.DeleteFile(id, path); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"ok": "true"})
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

type ServerGroup struct {
	ServerID   int64
	ServerName string
	Status     string
	Apps       []App
}

func GroupAppsByServer(apps []App, servers []ServerInfo) []ServerGroup {
	serverMap := make(map[int64]ServerInfo)
	for _, s := range servers {
		serverMap[s.ID] = s
	}

	groupMap := make(map[int64]*ServerGroup)
	var unassigned []App

	for _, app := range apps {
		svrInfo, found := serverMap[app.ServerID]
		if !found {
			unassigned = append(unassigned, app)
			continue
		}
		g, ok := groupMap[app.ServerID]
		if !ok {
			groupMap[app.ServerID] = &ServerGroup{
				ServerID:   svrInfo.ID,
				ServerName: svrInfo.Name,
				Status:     svrInfo.Status,
			}
			g = groupMap[app.ServerID]
		}
		g.Apps = append(g.Apps, app)
	}

	groups := make([]ServerGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ServerName < groups[j].ServerName
	})

	if len(unassigned) > 0 {
		groups = append(groups, ServerGroup{
			ServerName: "Unassigned",
			Apps:       unassigned,
		})
	}

	return groups
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

	servers, _ := h.serverRepo.List()
	groups := GroupAppsByServer(apps, servers)

	render(w, r, http.StatusOK, "apps.html", map[string]interface{}{
		"Title":        "Apps",
		"Apps":         apps,
		"ServerGroups": groups,
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

	mode := r.FormValue("mode")
	compose := strings.TrimSpace(r.FormValue("compose"))
	image := strings.TrimSpace(r.FormValue("image"))
	volumes := strings.TrimSpace(r.FormValue("volumes"))
	memoryLimit := strings.TrimSpace(r.FormValue("memory_limit"))
	cpuLimit := strings.TrimSpace(r.FormValue("cpu_limit"))
	logMaxSize := strings.TrimSpace(r.FormValue("log_max_size"))
	logMaxFile := strings.TrimSpace(r.FormValue("log_max_file"))
	command := strings.TrimSpace(r.FormValue("command"))
	ports := strings.TrimSpace(r.FormValue("ports"))
	ulimitsNofile := strings.TrimSpace(r.FormValue("ulimits_nofile"))
	envKeysRaw := r.Form["env_key"]

	envKeys := make([]string, 0, len(envKeysRaw))
	for _, k := range envKeysRaw {
		k = strings.TrimSpace(k)
		if k != "" {
			envKeys = append(envKeys, k)
		}
	}

	composeMode := "advanced"
	if mode == "simple" && image != "" {
		compose = generateCompose(image, port, volumes, strings.TrimSpace(r.FormValue("name")), memoryLimit, cpuLimit, logMaxSize, logMaxFile, envKeys, command, ports, ulimitsNofile)
		composeMode = "simple"
	}

	domains := parseDomains(r)
	if len(domains) == 0 {
		domains = []string{strings.TrimSpace(r.FormValue("domain"))}
	}
	primaryDomain := ""
	for _, d := range domains {
		if d != "" {
			primaryDomain = d
			break
		}
	}

	app := &App{
		Name:              strings.TrimSpace(r.FormValue("name")),
		ServerID:          serverID,
		Domain:            primaryDomain,
		Port:              port,
		Compose:           compose,
		GitRepo:           strings.TrimSpace(r.FormValue("git_repo")),
		GitBranch:         gitBranch,
		AuthUser:          strings.TrimSpace(r.FormValue("auth_user")),
		AuthPass:          strings.TrimSpace(r.FormValue("auth_pass")),
		ComposeMode:       composeMode,
		MemoryLimit:       memoryLimit,
		CPULimit:          cpuLimit,
		LogMaxSize:        logMaxSize,
		LogMaxFile:        logMaxFile,
		Command:           command,
		Ports:             ports,
		UlimitsNofile:     ulimitsNofile,
	}

	isDraft := r.FormValue("draft") == "1"
	if isDraft {
		app.Status = StatusDraft
	}

	envVals := r.Form["env_val"]
	envTypes := r.Form["env_type"]
	envList := make([]map[string]interface{}, 0, len(envKeysRaw))
	for i, k := range envKeysRaw {
		k = strings.TrimSpace(k)
		if k == "" || i >= len(envVals) {
			continue
		}
		envList = append(envList, map[string]interface{}{
			"key":      k,
			"value":    envVals[i],
			"isSecret": i < len(envTypes) && envTypes[i] == "secret",
		})
	}
	envJSON, _ := json.Marshal(envList)

	formCtx := func(servers interface{}, errMsg string) map[string]interface{} {
		return map[string]interface{}{
			"Title":       "Deploy App",
			"Servers":     servers,
			"App":         app,
			"SecretsJSON": template.JS(envJSON),
			"Image":       image,
			"Volumes":     volumes,
			"Domains":     domains,
			"MemLimit":    memoryLimit,
			"CpuLimit":    cpuLimit,
			"LogMaxSize":  logMaxSize,
			"LogMaxFile":  logMaxFile,
			"Command":     command,
			"Ports":       ports,
			"UlimitsNofile": ulimitsNofile,
			"Error":       errMsg,
		}
	}

	if app.Name == "" || app.Compose == "" {
		servers, _ := h.serverRepo.List()
		msg := "name and compose (or image) are required"
		render(w, r, http.StatusBadRequest, "apps_add.html", formCtx(servers, msg))
		return
	}

	if app.ServerID == 0 && !isDraft {
		id, err := h.service.PickServerID()
		if err != nil {
			servers, _ := h.serverRepo.List()
			msg := "no available servers — save as draft or add a server first"
			render(w, r, http.StatusBadRequest, "apps_add.html", formCtx(servers, msg))
			return
		}
		app.ServerID = id
	}

	if err := h.service.Create(app); err != nil {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusInternalServerError, "apps_add.html", formCtx(servers, friendlyError(err)))
		return
	}

	saveFormEnvVars(r, h.service, app.ID)
	saveFormFiles(r, h.service, app.ID)

	for _, d := range domains {
		if d == "" {
			continue
		}
		h.service.SaveRoute(&Route{
			AppID:    app.ID,
			ServerID: app.ServerID,
			Domain:   d,
			Target:   "",
		})
	}

	if !isDraft {
		go h.service.Deploy(app.ID)
	}

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(app.ID, 10), http.StatusSeeOther)
}

func (h *WebHandler) AppEditPage(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	app, err := h.service.Get(id)
	if err != nil || app == nil {
		render(w, r, http.StatusNotFound, "error.html", map[string]interface{}{"Message": "app not found"})
		return
	}
	servers, _ := h.serverRepo.List()
	secrets, _ := h.service.ListSecrets(id)
	files, _ := h.service.ListFiles(id)

	// Parse simple fields from compose for pre-filling the edit form
	sf := parseSimpleFields(app.Compose)
	image := sf.Image
	volumes := sf.Volumes
	if sf.Port > 0 {
		app.Port = sf.Port
	}

	routes, _ := h.service.GetRoutes(id)
	domainList := make([]string, 0, len(routes)+1)
	if app.Domain != "" {
		domainList = append(domainList, app.Domain)
	}
	for _, r := range routes {
		if r.Domain != app.Domain {
			domainList = append(domainList, r.Domain)
		}
	}

	secretsJSON, _ := json.Marshal(secrets)

	render(w, r, http.StatusOK, "apps_add.html", map[string]interface{}{
		"Title":       "Edit " + app.Name,
		"Servers":     servers,
		"App":         app,
		"Secrets":     secrets,
		"SecretsJSON": template.JS(secretsJSON),
		"Files":       files,
		"IsEdit":      true,
		"Image":       image,
		"Volumes":     volumes,
		"Domains":     domainList,
		"MemLimit":    sf.MemoryLimit,
		"CpuLimit":    sf.CPULimit,
		"LogMaxSize":  sf.LogMaxSize,
		"LogMaxFile":  sf.LogMaxFile,
		"Command":     sf.Command,
		"Ports":       sf.Ports,
		"UlimitsNofile": sf.UlimitsNofile,
	})
}

func (h *WebHandler) AppEditForm(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	app, err := h.service.Get(id)
	if err != nil || app == nil {
		render(w, r, http.StatusNotFound, "error.html", map[string]interface{}{"Message": "app not found"})
		return
	}
	isDraft := app.Status == StatusDraft
	oldServerID := app.ServerID
	if err := r.ParseForm(); err != nil {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusBadRequest, "apps_add.html", map[string]interface{}{
			"Title":   "Edit " + app.Name,
			"Servers": servers,
			"App":     app,
			"IsEdit":  true,
			"Error":   "invalid form data",
		})
		return
	}

	serverID, _ := strconv.ParseInt(r.FormValue("server_id"), 10, 64)
	if serverID == 0 && !isDraft {
		serverID = app.ServerID
	}
	port, _ := strconv.Atoi(r.FormValue("port"))

	volumes := strings.TrimSpace(r.FormValue("volumes"))
	memoryLimit := strings.TrimSpace(r.FormValue("memory_limit"))
	cpuLimit := strings.TrimSpace(r.FormValue("cpu_limit"))
	logMaxSize := strings.TrimSpace(r.FormValue("log_max_size"))
	logMaxFile := strings.TrimSpace(r.FormValue("log_max_file"))
	command := strings.TrimSpace(r.FormValue("command"))
	ports := strings.TrimSpace(r.FormValue("ports"))
	ulimitsNofile := strings.TrimSpace(r.FormValue("ulimits_nofile"))

	mode := r.FormValue("mode")
	compose := strings.TrimSpace(r.FormValue("compose"))
	image := strings.TrimSpace(r.FormValue("image"))
	newName := strings.TrimSpace(r.FormValue("name"))

	envKeysRaw := r.Form["env_key"]

	envKeys := make([]string, 0, len(envKeysRaw))
	for _, k := range envKeysRaw {
		k = strings.TrimSpace(k)
		if k != "" {
			envKeys = append(envKeys, k)
		}
	}

	if mode == "simple" && image != "" {
		compose = generateCompose(image, port, volumes, newName, memoryLimit, cpuLimit, logMaxSize, logMaxFile, envKeys, command, ports, ulimitsNofile)
		app.ComposeMode = "simple"
	} else {
		if compose == "" {
			compose = app.Compose
		}
		app.ComposeMode = "advanced"
	}

	domains := parseDomains(r)
	primaryDomain := ""
	for _, d := range domains {
		if d != "" {
			primaryDomain = d
			break
		}
	}

	app.Name = newName
	app.ServerID = serverID
	app.Domain = primaryDomain
	app.Port = port
	app.Compose = compose
	app.GitRepo = strings.TrimSpace(r.FormValue("git_repo"))
	app.GitBranch = strings.TrimSpace(r.FormValue("git_branch"))
	app.AuthUser = strings.TrimSpace(r.FormValue("auth_user"))
	app.AuthPass = strings.TrimSpace(r.FormValue("auth_pass"))
	app.MemoryLimit = memoryLimit
	app.CPULimit = cpuLimit
	app.LogMaxSize = logMaxSize
	app.LogMaxFile = logMaxFile
	app.Command = command
	app.Ports = ports
	app.UlimitsNofile = ulimitsNofile

	envVals := r.Form["env_val"]
	envTypes := r.Form["env_type"]
	envList := make([]map[string]interface{}, 0, len(envKeysRaw))
	for i, k := range envKeysRaw {
		k = strings.TrimSpace(k)
		if k == "" || i >= len(envVals) {
			continue
		}
		envList = append(envList, map[string]interface{}{
			"key":      k,
			"value":    envVals[i],
			"isSecret": i < len(envTypes) && envTypes[i] == "secret",
		})
	}
	envJSON, _ := json.Marshal(envList)

	editCtx := func(servers interface{}, errMsg string) map[string]interface{} {
		return map[string]interface{}{
			"Title":       "Edit " + app.Name,
			"Servers":     servers,
			"App":         app,
			"SecretsJSON": template.JS(envJSON),
			"Image":       image,
			"Volumes":     volumes,
			"Domains":     domains,
			"MemLimit":    memoryLimit,
			"CpuLimit":    cpuLimit,
			"LogMaxSize":  logMaxSize,
			"LogMaxFile":  logMaxFile,
			"Command":     command,
			"Ports":       ports,
			"UlimitsNofile": ulimitsNofile,
			"IsEdit":      true,
			"Error":       errMsg,
		}
	}

	if app.GitBranch == "" {
		app.GitBranch = "main"
	}
	if app.Name == "" || app.Compose == "" {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusBadRequest, "apps_add.html", editCtx(servers, "name and compose are required"))
		return
	}

	if err := h.service.Update(app); err != nil {
		servers, _ := h.serverRepo.List()
		render(w, r, http.StatusInternalServerError, "apps_add.html", editCtx(servers, err.Error()))
		return
	}

	var flashMsg string
	if !isDraft && oldServerID != 0 && oldServerID != serverID {
		oldServerName := fmt.Sprintf("#%d", oldServerID)
		newServerName := fmt.Sprintf("#%d", serverID)
		if servers, err := h.serverRepo.List(); err == nil {
			for _, s := range servers {
				if s.ID == oldServerID {
					oldServerName = s.Name
				}
				if s.ID == serverID {
					newServerName = s.Name
				}
			}
		}
		h.service.DeleteRoutes(app.ID)
		go h.service.CleanupFromServer(id, oldServerID)
		flashMsg = url.QueryEscape(fmt.Sprintf(
			"App moved from %s to %s. Containers stopped on %s, but app folder remains — check and delete manually if no longer needed.",
			oldServerName, newServerName, oldServerName,
		))
	}

	saveFormEnvVars(r, h.service, id)
	saveFormFiles(r, h.service, id)

	oldRoutes, _ := h.service.GetRoutes(id)
	oldDomains := make(map[string]Route)
	for _, r := range oldRoutes {
		oldDomains[r.Domain] = r
	}
	if app.Domain != "" {
		if _, exists := oldDomains[app.Domain]; !exists {
			if app.Domain != "" {
				h.service.SaveRoute(&Route{
					AppID:    app.ID,
					ServerID: app.ServerID,
					Domain:   app.Domain,
					Target:   "",
				})
			}
		}
	}
	seen := map[string]bool{}
	for _, d := range domains {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		if _, exists := oldDomains[d]; !exists {
			h.service.SaveRoute(&Route{
				AppID:    app.ID,
				ServerID: app.ServerID,
				Domain:   d,
				Target:   "",
			})
		}
	}
	var removedDomains []string
	for _, r := range oldRoutes {
		if !seen[r.Domain] {
			removedDomains = append(removedDomains, r.Domain)
		}
	}
	for _, r := range oldRoutes {
		if !seen[r.Domain] {
			h.service.DeleteRouteByDomain(app.ID, r.Domain)
		}
	}

	if !isDraft {
		go h.service.Redeploy(id, removedDomains...)
	}

	redirectURL := "/apps/" + strconv.FormatInt(id, 10)
	if flashMsg != "" {
		redirectURL += "?flash=" + flashMsg
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
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

	secrets, _ := h.service.ListSecrets(id)
	files, _ := h.service.ListFiles(id)

	// Resolve server name for display
	serverName := fmt.Sprintf("#%d", app.ServerID)
	if servers, err := h.serverRepo.List(); err == nil {
		for _, svr := range servers {
			if svr.ID == app.ServerID {
				serverName = svr.Name
				break
			}
		}
	}

	routes, _ := h.service.GetRoutes(id)
	domains := []string{}
	if app.Domain != "" {
		domains = append(domains, app.Domain)
	}
	for _, r := range routes {
		if r.Domain != app.Domain {
			domains = append(domains, r.Domain)
		}
	}

	render(w, r, http.StatusOK, "apps_detail.html", map[string]interface{}{
		"Title":            app.Name,
		"App":              app,
		"ServerName":       serverName,
		"Deployments":      deps,
		"Secrets":          secrets,
		"Files":            files,
		"Routes":           routes,
		"Domains":          domains,
		"Flash":            r.URL.Query().Get("flash"),
		"DomainCount":      len(domains),
		"ExtraDomainCount": len(domains) - 1,
		"ServiceName":      app.ContainerServiceName(),
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

func (h *WebHandler) AppDeployWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	app, err := h.service.Get(id)
	if err != nil || app == nil {
		render(w, r, http.StatusNotFound, "error.html", map[string]interface{}{"Message": "app not found"})
		return
	}

	if app.ServerID == 0 {
		http.Redirect(w, r, "/apps/"+strconv.FormatInt(id, 10)+"/edit", http.StatusSeeOther)
		return
	}

	h.service.UpdateStatus(id, StatusDeploying)
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

func (h *WebHandler) AppStopWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Stop(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *WebHandler) AppStartWeb(w http.ResponseWriter, r *http.Request, render RenderFunc) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Start(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/apps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func saveFormEnvVars(r *http.Request, svc *Service, appID int64) {
	existing, _ := svc.ListSecrets(appID)
	existingMap := make(map[string]AppSecret)
	for _, ev := range existing {
		existingMap[ev.Key] = ev
	}

	keys := r.Form["env_key"]
	vals := r.Form["env_val"]
	types := r.Form["env_type"]
	submitted := make(map[string]bool)

	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || i >= len(vals) {
			continue
		}
		submitted[k] = true
		v := vals[i]
		isSecret := i < len(types) && types[i] == "secret"

		if isSecret && v == "" {
			continue
		}

		svc.SetSecretWithType(appID, k, v, isSecret)
	}

	for _, ev := range existing {
		if !submitted[ev.Key] {
			svc.DeleteSecret(appID, ev.Key)
		}
	}
}

func parseDomains(r *http.Request) []string {
	domains := r.Form["domain"]
	seen := map[string]bool{}
	var result []string
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d != "" && !seen[d] {
			seen[d] = true
			result = append(result, d)
		}
	}
	return result
}

func saveFormFiles(r *http.Request, svc *Service, appID int64) {
	paths := r.Form["file_path"]
	contents := r.Form["file_content"]
	for i, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || i >= len(contents) {
			continue
		}
		c := contents[i]
		if c == "" {
			continue
		}
		svc.SetFile(appID, p, c)
	}
}

type RenderFunc = func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func friendlyError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "FOREIGN KEY") {
		return "please select a server or save as draft"
	}
	if strings.Contains(msg, "UNIQUE constraint") {
		return "an app with this name already exists"
	}
	return msg
}
