package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareAPIToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	cases := []struct {
		name, token, header string
		want                int
	}{
		{"valid token", "s3cret", "Bearer s3cret", http.StatusTeapot},
		{"wrong token", "s3cret", "Bearer nope", http.StatusUnauthorized},
		{"bearer while no token configured", "", "Bearer s3cret", http.StatusUnauthorized},
		{"no header falls back to login", "s3cret", "", http.StatusFound},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/apps", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		AuthMiddleware(true, c.token, ok).ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
