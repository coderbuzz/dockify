package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/coderbuzz/dockify/internal/server"
	"github.com/coderbuzz/dockify/internal/ssh"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type ConsoleHandler struct {
	serverSvc *server.Service
	sshKeyDir string
}

func NewConsoleHandler(svc *server.Service, sshKeyDir string) *ConsoleHandler {
	return &ConsoleHandler{serverSvc: svc, sshKeyDir: sshKeyDir}
}

func (h *ConsoleHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid server id", http.StatusBadRequest)
		return
	}

	svr, err := h.serverSvc.Get(id)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket before SSH connect (fail fast for WS issues)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("console ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var client ssh.Connector

	if GetDevMock(r) {
		client = ssh.NewMockClient()
	} else {
		var err error
		client, err = ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
		if err != nil {
			log.Printf("console ssh connect error: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte("SSH connection failed: "+err.Error()+"\r\n"))
			return
		}
		defer client.Close()
	}

	outCh, inCh, err := client.Shell(ctx, 24, 80)
	if err != nil {
		log.Printf("console shell error: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("SSH shell failed: "+err.Error()+"\r\n"))
		return
	}

	errCh := make(chan error, 2)

	// SSH stdout → WebSocket (raw binary)
	go func() {
		for out := range outCh {
			if out.Closed {
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, out.Data); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// WebSocket → SSH stdin
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			var input ssh.Input
			if json.Unmarshal(msg, &input) == nil && input.Resize != nil {
				inCh <- input
				continue
			}

			inCh <- ssh.Input{Data: msg}
		}
	}()

	// Block until error or context done
	select {
	case <-errCh:
	case <-ctx.Done():
	}
}
