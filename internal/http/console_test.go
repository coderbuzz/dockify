package http

import (
	"fmt"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coderbuzz/dockify/internal/db"
	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// B13: console sessions opened and closed in mock mode leave no goroutines.
func TestConsoleNoGoroutineLeak(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := server.NewRepository(database)
	svc := server.NewService(repo)
	svc.SetConnFactory(ssh.MockFactory())
	svr := &server.Server{Name: "w1", Host: "10.0.0.1", Port: 22, User: "root", SSHKey: "/tmp/key"}
	if err := repo.Create(svr); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/servers/{id}/console", NewConsoleHandler(svc, nil, "").ServeWS)
	ts := httptest.NewServer(r)
	defer ts.Close()
	url := fmt.Sprintf("ws%s/api/servers/%d/console", strings.TrimPrefix(ts.URL, "http"), svr.ID)

	session := func() {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte("ls\r")); err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("no reply: %v", err)
			}
			if strings.Contains(string(msg), "Mock response for: ls") {
				return
			}
		}
	}

	session() // warm up the client and server connection pools
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	for range 20 {
		session()
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines: %d before, %d after 20 console sessions", before, after)
	}
}
