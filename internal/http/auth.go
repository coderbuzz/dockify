package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	sessions      = map[string]time.Time{}
	sessionsMu    sync.RWMutex
	sessionName   = "dockify_session"
	sessionMaxAge = 24 * time.Hour
)

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
	sessionsMu.Lock()
	sessions[id] = time.Now().Add(sessionMaxAge)
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

		http.Redirect(w, r, "/login", http.StatusFound)
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

		user := r.FormValue("user")
		pass := r.FormValue("pass")

		if user == cfgUser && pass == cfgPass {
			setSession(w, r)
			log.Printf("Login: %s", user)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

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
	http.Redirect(w, r, "/login", http.StatusFound)
}
