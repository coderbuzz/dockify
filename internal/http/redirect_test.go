package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// A form POST that redirects keeps the X-Forwarded-Prefix in its Location (B8).
func TestRedirectKeepsBasePath(t *testing.T) {
	r := chi.NewRouter()
	r.Use(PrefixMiddleware)
	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		HandleLogin(w, r, "admin", "pw", Render)
	})
	for prefix, want := range map[string]string{"": "/", "/proxy/9898": "/proxy/9898/"} {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"user": {"admin"}, "pass": {"pw"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if prefix != "" {
			req.Header.Set("X-Forwarded-Prefix", prefix)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if got := rec.Header().Get("Location"); rec.Code != http.StatusFound || got != want {
			t.Errorf("prefix %q: %d Location %q, want %q", prefix, rec.Code, got, want)
		}
	}
}
