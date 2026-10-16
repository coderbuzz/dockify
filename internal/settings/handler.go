package settings

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request, render func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})) {
	// The webhook secret is not rendered; the page loads it from GET /api/settings/webhook-secret on Show/Copy.
	render(w, r, http.StatusOK, "settings.html", map[string]interface{}{"Title": "Settings"})
}

func (h *Handler) RollWebhookSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := h.service.RegenerateWebhookSecret()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"secret": secret})
}

func (h *Handler) GetWebhookSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := h.service.GetWebhookSecret()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"secret": secret})
}

func (h *Handler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1"
	info, err := h.service.CheckUpdate(force)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"current":    h.service.version,
			"latest":     "",
			"has_update": false,
			"error":      err.Error(),
		})
		return
	}
	jsonResponse(w, http.StatusOK, info)
}

func (h *Handler) RunUpdate(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RunUpdate(); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "update started"})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
