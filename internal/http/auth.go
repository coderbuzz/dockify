package http

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	sessions     = map[string]time.Time{}
	sessionsMu   sync.RWMutex
	sessionName  = "dockify_session"
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

func setSession(w http.ResponseWriter, user string) {
	id := generateSessionID()
	sessionsMu.Lock()
	sessions[id] = time.Now().Add(sessionMaxAge)
	sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
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

func AuthMiddleware(enabled bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}

		if getSessionUser(r) != "" {
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
			setSession(w, user)
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
