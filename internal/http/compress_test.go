package http

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompress(t *testing.T) {
	r, _, _ := newTestRouter(t)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	for _, p := range []string{"/static/htmx.min.js", "/static/app.css", "/static/sw.js", "/"} {
		rec := get(p)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("%s: %d, Content-Encoding %q", p, rec.Code, rec.Header().Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if b, err := io.ReadAll(zr); err != nil || len(b) < compressMinSize {
			t.Errorf("%s: %d bytes, %v", p, len(b), err)
		}
	}
	if cc := get("/static/sw.js").Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("sw.js Cache-Control %q", cc)
	}
	if cc := get("/static/app.css").Header().Get("Cache-Control"); cc != "public, max-age=604800" {
		t.Errorf("app.css Cache-Control %q", cc)
	}
	// Under 1 KB: sent plain. Images are never compressed.
	for _, p := range []string{"/health", "/static/icon-192.png"} {
		if rec := get(p); rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: %d, Content-Encoding %q", p, rec.Code, rec.Header().Get("Content-Encoding"))
		}
	}
	if b := get("/health").Body.String(); b != "ok" {
		t.Errorf("/health body %q", b)
	}
}
