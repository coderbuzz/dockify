// Package web holds request helpers shared by the HTTP layer and the app/server handlers.
package web

import (
	"context"
	"net/http"
	"strings"
)

type contextKey struct{}

// SetBasePath stores the URL prefix (always ending in "/") Dockify is served under.
func SetBasePath(r *http.Request, path string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), contextKey{}, path))
}

// GetBasePath returns the URL prefix, "/" when Dockify is served at the root.
func GetBasePath(r *http.Request) string {
	if v, ok := r.Context().Value(contextKey{}).(string); ok && v != "" {
		return v
	}
	return "/"
}

// Redirect is http.Redirect with the base path prepended to a path starting with "/".
func Redirect(w http.ResponseWriter, r *http.Request, path string, code int) {
	if strings.HasPrefix(path, "/") {
		path = strings.TrimSuffix(GetBasePath(r), "/") + path
	}
	http.Redirect(w, r, path, code)
}
