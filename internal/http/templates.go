package http

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

func relativeTime(v interface{}) string {
	t, ok := v.(time.Time)
	if !ok || t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	default:
		return "on " + t.Format("Jan 2")
	}
}

func usedAmount(total int, pct float64) float64 {
	return float64(total) * pct / 100.0
}

func freeAmount(total int, pct float64) float64 {
	return float64(total) - usedAmount(total, pct)
}

var funcMap = template.FuncMap{
	"lower":        strings.ToLower,
	"upper":        strings.ToUpper,
	"relativeTime": relativeTime,
	"usedAmount":   usedAmount,
	"freeAmount":   freeAmount,
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
