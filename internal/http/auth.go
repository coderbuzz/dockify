package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coderbuzz/dockify/internal/web"
)

var (
	sessions      = map[string]time.Time{}
	sessionsMu    sync.RWMutex
	sessionName   = "dockify_session"
	sessionMaxAge = 24 * time.Hour
)

// Login throttle: more than loginMaxFailures failed logins from one client within
// loginWindow (counted from the first failure) answer 429 until the window ends.
const (
	loginMaxFailures = 10
	loginWindow      = 15 * time.Minute
)

type loginFailure struct {
	count int
	since time.Time
}

var (
	loginFailures   = map[string]*loginFailure{}
	loginFailuresMu sync.Mutex
	loginFailDelay  = time.Second
)

// clientKey is the client IP: r.RemoteAddr, which chimw.RealIP rewrites only
// when DOCKIFY_TRUST_PROXY=true.
func clientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// loginBlocked reports whether the client used up its failures in the current
// window, and drops entries whose window has ended.
func loginBlocked(key string) bool {
	loginFailuresMu.Lock()
	defer loginFailuresMu.Unlock()
	now := time.Now()
	for k, f := range loginFailures {
		if now.Sub(f.since) >= loginWindow {
			delete(loginFailures, k)
		}
	}
	f := loginFailures[key]
	return f != nil && f.count >= loginMaxFailures
}

func recordLoginFailure(key string) {
	loginFailuresMu.Lock()
	defer loginFailuresMu.Unlock()
	if f := loginFailures[key]; f != nil {
		f.count++
	} else {
		loginFailures[key] = &loginFailure{count: 1, since: time.Now()}
	}
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func getSessionUser(r *http.Request) string {
	c, err := r.Cookie(sessionName)
	if err != nil {
		return ""
	}
	sessionsMu.RLock()
	expiry, ok := sessions[c.Value]
	sessionsMu.RUnlock()
	if !ok || time.Now().After(expiry) {
		return ""
	}
	return "admin"
}

func setSession(w http.ResponseWriter, r *http.Request) {
	id := generateSessionID()
	now := time.Now()
	sessionsMu.Lock()
	for k, expiry := range sessions {
		if now.After(expiry) {
			delete(sessions, k)
		}
	}
	sessions[id] = now.Add(sessionMaxAge)
	sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   int(sessionMaxAge.Seconds()),
	})
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionName)
	if err == nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// AuthMiddleware lets a request through with a valid session cookie or, when apiToken is set,
// an "Authorization: Bearer <apiToken>" header (for scripts and agents; no login round trip,
// survives controller restarts). A wrong bearer token gets 401 instead of the login redirect.
// Other requests with an unsafe method sent by another site get 403 (CSRF), also when auth is
// disabled: the stdlib check uses Sec-Fetch-Site, or Origin vs Host for older browsers.
func AuthMiddleware(enabled bool, apiToken string, next http.Handler) http.Handler {
	csrf := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && enabled {
			if apiToken != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(apiToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "invalid api token", http.StatusUnauthorized)
			return
		}

		if err := csrf.Check(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		if !enabled || getSessionUser(r) != "" {
			next.ServeHTTP(w, r)
			return
		}

		web.Redirect(w, r, "/login", http.StatusFound)
	})
}

func HandleLogin(w http.ResponseWriter, r *http.Request, cfgUser, cfgPass string, render RenderFunc) {
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, "login.html", map[string]interface{}{
				"Title": "Login",
				"Error": "invalid form",
			})
			return
		}

		key := clientKey(r)
		if loginBlocked(key) {
			render(w, r, http.StatusTooManyRequests, "login.html", map[string]interface{}{
				"Title": "Login",
				"Error": "too many failed logins, try again later",
			})
			return
		}

		user := r.FormValue("user")
		pass := r.FormValue("pass")
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(cfgUser))
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(cfgPass))
		if userOK&passOK == 1 {
			loginFailuresMu.Lock()
			delete(loginFailures, key)
			loginFailuresMu.Unlock()
			setSession(w, r)
			log.Printf("Login: %s", user)
			web.Redirect(w, r, "/", http.StatusFound)
			return
		}

		recordLoginFailure(key)
		log.Printf("Login failed from %s", key)
		time.Sleep(loginFailDelay)
		render(w, r, http.StatusUnauthorized, "login.html", map[string]interface{}{
			"Title": "Login",
			"Error": "invalid username or password",
		})
		return
	}

	render(w, r, http.StatusOK, "login.html", map[string]interface{}{
		"Title": "Login",
	})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w, r)
	web.Redirect(w, r, "/login", http.StatusFound)
}
