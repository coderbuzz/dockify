package webhook

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

type WebhookService interface {
	DeployByGit(repo, branch, commitSHA string)
}

type Handler struct {
	service WebhookService
}

func NewHandler(service WebhookService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GitHub(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	if event != "push" {
		log.Printf("Webhook: ignoring GitHub event %q", event)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ignored"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var payload struct {
		Ref  string `json:"ref"`
		After string `json:"after"`
		Repo struct {
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	repo := payload.Repo.CloneURL
	commitSHA := payload.After

	log.Printf("GitHub webhook: repo=%s branch=%s commit=%s", repo, branch, commitSHA)
	go h.service.DeployByGit(repo, branch, commitSHA)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) GitLab(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-Gitlab-Event")
	if event != "Push Hook" {
		log.Printf("Webhook: ignoring GitLab event %q", event)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ignored"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var payload struct {
		Ref  string `json:"ref"`
		After string `json:"after"`
		Project struct {
			GitHTTPURL string `json:"git_http_url"`
		} `json:"project"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	repo := payload.Project.GitHTTPURL
	commitSHA := payload.After

	log.Printf("GitLab webhook: repo=%s branch=%s commit=%s", repo, branch, commitSHA)
	go h.service.DeployByGit(repo, branch, commitSHA)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
