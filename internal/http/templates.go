package http

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

var funcMap = template.FuncMap{
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
}

var tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

type contextKey string

const basePathKey contextKey = "basePath"

func SetBasePath(r *http.Request, path string) *http.Request {
	ctx := context.WithValue(r.Context(), basePathKey, path)
	return r.WithContext(ctx)
}

func GetBasePath(r *http.Request) string {
	if v, ok := r.Context().Value(basePathKey).(string); ok && v != "" {
		return v
	}
	return "/"
}

type RenderFunc func(w http.ResponseWriter, r *http.Request, status int, name string, data interface{})

func Render(w http.ResponseWriter, r *http.Request, status int, name string, data interface{}) {
	t := tmpl.Lookup(name)
	if t == nil {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	basePath := GetBasePath(r)
	m, ok := data.(map[string]interface{})
	if ok {
		m["BasePath"] = basePath
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.Execute(w, data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
	}
}
