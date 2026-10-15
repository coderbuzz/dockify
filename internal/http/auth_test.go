package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestLoginThrottle(t *testing.T) {
	loginFailDelay = 0
	t.Cleanup(func() { loginFailDelay = time.Second })
	login := func(pass, remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"user": {"admin"}, "pass": {pass}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		HandleLogin(rec, req, "admin", "pw", Render)
		return rec
	}
	const client = "198.51.100.7:4000"

	for i := 1; i <= loginMaxFailures; i++ {
		if rec := login("wrong", client); rec.Code != http.StatusUnauthorized || rec.Header().Get("Set-Cookie") != "" {
			t.Fatalf("failure %d: %d, cookie %q", i, rec.Code, rec.Header().Get("Set-Cookie"))
		}
	}
	if rec := login("wrong", client); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th failure: %d, want 429", rec.Code)
	}
	if rec := login("pw", client); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password inside the window: %d, want 429", rec.Code)
	}
	if rec := login("wrong", "203.0.113.9:4000"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("other client: %d, want 401", rec.Code)
	}

	// Window over; an expired session is pruned by the next login.
	loginFailuresMu.Lock()
	loginFailures["198.51.100.7"].since = time.Now().Add(-loginWindow)
	loginFailuresMu.Unlock()
	sessionsMu.Lock()
	sessions["expired"] = time.Now().Add(-time.Minute)
	sessionsMu.Unlock()
	if rec := login("pw", client); rec.Code != http.StatusFound || rec.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login after the window: %d, cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	sessionsMu.RLock()
	_, stale := sessions["expired"]
	sessionsMu.RUnlock()
	if stale {
		t.Error("expired session not pruned on login")
	}
}

func TestAuthMiddlewareCrossSite(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	sessionsMu.Lock()
	sessions["csrf-test"] = time.Now().Add(time.Hour)
	sessionsMu.Unlock()
	cases := []struct {
		name    string
		enabled bool
		method  string
		hdr     map[string]string
		want    int
	}{
		{"cross-site POST", true, "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign Origin POST", true, "POST", map[string]string{"Origin": "http://evil.test"}, http.StatusForbidden},
		{"same-origin POST", true, "POST", map[string]string{"Origin": "http://example.com"}, http.StatusTeapot},
		{"cross-site GET", true, "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusTeapot},
		{"bearer token skips check", true, "POST", map[string]string{"Origin": "http://evil.test", "Authorization": "Bearer s3cret"}, http.StatusTeapot},
		{"cross-site POST without password", false, "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "/apps/1/stop", nil) // Host: example.com
		req.AddCookie(&http.Cookie{Name: sessionName, Value: "csrf-test"})
		for k, v := range c.hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		AuthMiddleware(c.enabled, "s3cret", ok).ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
