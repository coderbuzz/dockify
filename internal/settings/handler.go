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
	secret, _ := h.service.GetWebhookSecret()
	render(w, r, http.StatusOK, "settings.html", map[string]interface{}{
		"Title":         "Settings",
		"WebhookSecret": secret,
	})
}

func (h *Handler) RollWebhookSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := h.service.RegenerateWebhookSecret()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"secret": secret})
}

func (h *Handler) EnableWebhookSecret(w http.ResponseWriter, r *http.Request) {
	secret, err := h.service.GetWebhookSecret()
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

func (h *Handler) DisableWebhookSecret(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DisableWebhookSecret(); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"secret": ""})
}

func (h *Handler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := h.service.CheckUpdate()
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
