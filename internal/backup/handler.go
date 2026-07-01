package backup

import (
	"io"
	"net/http"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ExportDownload(w http.ResponseWriter, r *http.Request) {
	passphrase := r.FormValue("passphrase")

	yamlStr, err := h.service.Export(passphrase)
	if err != nil {
		http.Error(w, "export: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	if passphrase != "" {
		w.Header().Set("Content-Disposition", "attachment; filename=dockify-export.yaml")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=dockify-export.yaml")
	}
	w.Write([]byte(yamlStr))
}

func (h *Handler) ExportPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	render(w, r, http.StatusOK, "export.html", map[string]interface{}{
		"Title": "Export",
	})
}

func (h *Handler) ImportPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	render(w, r, http.StatusOK, "import.html", map[string]interface{}{
		"Title":   "Import",
		"Message": r.URL.Query().Get("message"),
	})
}

func (h *Handler) ImportUpload(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	mode := r.FormValue("mode")
	if mode != "replace" && mode != "merge" {
		mode = "merge"
	}
	passphrase := r.FormValue("passphrase")

	file, _, err := r.FormFile("file")
	if err != nil {
		render(w, r, http.StatusOK, "import.html", map[string]interface{}{
			"Title": "Import",
			"Error": "No file selected",
		})
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		render(w, r, http.StatusOK, "import.html", map[string]interface{}{
			"Title": "Import",
			"Error": "Failed to read file",
		})
		return
	}

	logOutput, err := h.service.Import(string(raw), passphrase, mode)
	if err != nil {
		render(w, r, http.StatusOK, "import.html", map[string]interface{}{
			"Title": "Import",
			"Error": "Import failed: " + err.Error(),
			"Log":   logOutput,
		})
		return
	}
	render(w, r, http.StatusOK, "import.html", map[string]interface{}{
		"Title":   "Import",
		"Message": "Import complete",
		"Log":     logOutput,
	})
}
