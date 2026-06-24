package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service   *Service
	sshKeyDir string
}

func NewHandler(service *Service, sshKeyDir string) *Handler {
	return &Handler{service: service, sshKeyDir: sshKeyDir}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	servers, err := h.service.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	JSON(w, http.StatusOK, servers)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string `json:"name"`
		Host   string `json:"host"`
		Port   int    `json:"port"`
		User   string `json:"user"`
		SSHKey string `json:"ssh_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if input.Name == "" || input.Host == "" || input.SSHKey == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "name, host, and ssh_key are required"})
		return
	}

	server := &Server{
		Name:   input.Name,
		Host:   input.Host,
		Port:   input.Port,
		User:   input.User,
		SSHKey: "pending",
	}

	if err := h.service.Create(server); err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	path, err := saveKeyFile(h.sshKeyDir, server.ID, input.SSHKey)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	server.SSHKey = path
	if err := h.service.Update(server); err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go func(id int64) {
		h.service.TestConnection(id)
		h.service.InitWorker(id)
		h.service.RefreshResources(id)
	}(server.ID)

	JSON(w, http.StatusCreated, server)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	server, err := h.service.Get(id)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if server == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "server not found"})
		return
	}

	JSON(w, http.StatusOK, server)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Delete(id); err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	JSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (h *Handler) Init(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	server, err := h.service.Get(id)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if server == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "server not found"})
		return
	}

	go func(id int64) {
		h.service.InitWorker(id)
	}(id)

	JSON(w, http.StatusAccepted, map[string]string{"message": "initialization started"})
}

type WebHandler struct {
	service   *Service
	sshKeyDir string
}

func NewWebHandler(service *Service, sshKeyDir string) *WebHandler {
	return &WebHandler{service: service, sshKeyDir: sshKeyDir}
}

func saveKeyFile(dir string, id int64, content string) (string, error) {
	path := filepath.Join(dir, fmt.Sprintf("%d.pem", id))
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("save SSH key: %w", err)
	}
	return path, nil
}

func (h *WebHandler) ServerListPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	servers, err := h.service.List()
	if err != nil {
		render(w, r, http.StatusInternalServerError, "error.html", map[string]interface{}{"Message": err.Error()})
		return
	}

	render(w, r, http.StatusOK, "servers.html", map[string]interface{}{
		"Title":   "Servers",
		"Servers": servers,
	})
}

func (h *WebHandler) ServerAddPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	render(w, r, http.StatusOK, "servers_add.html", map[string]interface{}{
		"Title": "Add Server",
	})
}

func (h *WebHandler) ServerAddForm(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	if err := r.ParseForm(); err != nil {
		render(w, r, http.StatusBadRequest, "servers_add.html", map[string]interface{}{
			"Title": "Add Server",
			"Error": "invalid form data",
		})
		return
	}

	port, _ := strconv.Atoi(r.FormValue("port"))
	if port == 0 {
		port = 22
	}
	user := r.FormValue("user")
	if user == "" {
		user = "root"
	}

	keyContent := strings.TrimSpace(r.FormValue("ssh_key"))
	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))

	if name == "" || host == "" || keyContent == "" {
		render(w, r, http.StatusBadRequest, "servers_add.html", map[string]interface{}{
			"Title": "Add Server",
			"Error": "name, host, and ssh_key are required",
		})
		return
	}

	server := &Server{
		Name:   name,
		Host:   host,
		Port:   port,
		User:   user,
		SSHKey: "pending",
	}

	if err := h.service.Create(server); err != nil {
		render(w, r, http.StatusInternalServerError, "servers_add.html", map[string]interface{}{
			"Title": "Add Server",
			"Error": err.Error(),
		})
		return
	}

	path, err := saveKeyFile(h.sshKeyDir, server.ID, keyContent)
	if err != nil {
		render(w, r, http.StatusInternalServerError, "servers_add.html", map[string]interface{}{
			"Title": "Add Server",
			"Error": err.Error(),
		})
		return
	}

	server.SSHKey = path
	if err := h.service.Update(server); err != nil {
		render(w, r, http.StatusInternalServerError, "servers_add.html", map[string]interface{}{
			"Title": "Add Server",
			"Error": err.Error(),
		})
		return
	}

	go func(id int64) {
		h.service.TestConnection(id)
		h.service.InitWorker(id)
		h.service.RefreshResources(id)
	}(server.ID)

	http.Redirect(w, r, "/servers", http.StatusSeeOther)
}

func (h *WebHandler) ServerDetailPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	server, err := h.service.Get(id)
	if err != nil {
		render(w, r, http.StatusInternalServerError, "error.html", map[string]interface{}{"Message": err.Error()})
		return
	}
	if server == nil {
		render(w, r, http.StatusNotFound, "error.html", map[string]interface{}{"Message": "server not found"})
		return
	}

	render(w, r, http.StatusOK, "servers_detail.html", map[string]interface{}{
		"Title":  server.Name,
		"Server": server,
	})
}

func (h *WebHandler) ServerInit(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	go func(id int64) {
		h.service.InitWorker(id)
	}(id)

	http.Redirect(w, r, "/servers/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *WebHandler) ServerDeleteWeb(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := h.service.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/servers", http.StatusSeeOther)
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
