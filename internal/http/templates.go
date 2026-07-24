package http

import (
	"context"
	"encoding/json"
	"embed"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/coderbuzz/dockify/internal/model"
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

func formatBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func chartPoints(points []model.ChartPoint, width, height int, maxVal float64, rangeStr string) template.HTMLAttr {
	if len(points) == 0 || maxVal <= 0 {
		return ""
	}
	now := time.Now().UTC()
	dur := rangeDuration(rangeStr)
	start := now.Add(-dur)
	layout := "2006-01-02 15:04:05"
	var parts []string
	for _, p := range points {
		t, err := time.Parse(layout, p.Time)
		if err != nil {
			if t, err = time.Parse(time.RFC3339, p.Time); err != nil {
				continue
			}
		}
		if t.Before(start) {
			continue
		}
		x := float64(width) * float64(t.Sub(start)) / float64(dur)
		if x < 0 {
			x = 0
		} else if x > float64(width) {
			x = float64(width)
		}
		y := float64(height) - (p.Value/maxVal)*float64(height)
		parts = append(parts, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	if len(parts) == 0 {
		return ""
	}
	return template.HTMLAttr(strings.Join(parts, " "))
}

func rangeDuration(rangeStr string) time.Duration {
	switch rangeStr {
	case "realtime":
		return 2 * time.Minute
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 2 * time.Minute
	}
}

func chartMax(points []model.ChartPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	maxVal := points[0].Value
	for _, p := range points {
		if p.Value > maxVal {
			maxVal = p.Value
		}
	}
	if maxVal == 0 {
		return 1
	}
	magnitude := math.Pow(10, math.Floor(math.Log10(maxVal)))
	return math.Ceil(maxVal/magnitude) * magnitude
}

func chartMax100(points []model.ChartPoint) float64 {
	maxVal := chartMax(points)
	if maxVal < 100 {
		return 100
	}
	return maxVal
}

func chartThresholdY(maxVal float64, height int) float64 {
	if maxVal <= 0 {
		return float64(height)
	}
	y := float64(height) - (100.0/maxVal)*float64(height)
	if y < 0 {
		y = 0
	}
	return y
}

func chartPointsJSON(points []model.ChartPoint) template.HTMLAttr {
	b, err := json.Marshal(points)
	if err != nil {
		return template.HTMLAttr("[]")
	}
	return template.HTMLAttr(string(b))
}

func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func mul(a, b float64) float64 {
	return a * b
}

func clamp100(v float64) float64 {
	if v > 100 {
		return 100
	}
	if v < 0 {
		return 0
	}
	return v
}

var funcMap = template.FuncMap{
	"lower":        strings.ToLower,
	"upper":        strings.ToUpper,
	"relativeTime": relativeTime,
	"usedAmount":   usedAmount,
	"freeAmount":   freeAmount,
	"formatBytes":  formatBytes,
	"chartPoints":  chartPoints,
	"chartMax":     chartMax,
	"chartMax100":  chartMax100,
	"chartPointsJSON": chartPointsJSON,
	"chartThresholdY": chartThresholdY,
	"div":          div,
	"mul":          mul,
	"clamp100":     clamp100,
	"nl2br": func(s string) template.HTML {
		return template.HTML(strings.ReplaceAll(s, "\n", "<br>"))
	},
}

var tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

type contextKey string

const basePathKey contextKey = "basePath"
const devMockKey contextKey = "devMock"

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

func SetDevMock(r *http.Request, val bool) *http.Request {
	ctx := context.WithValue(r.Context(), devMockKey, val)
	return r.WithContext(ctx)
}

func GetDevMock(r *http.Request) bool {
	v, _ := r.Context().Value(devMockKey).(bool)
	return v
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
		m["DevMock"] = GetDevMock(r)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.Execute(w, data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
	}
}
