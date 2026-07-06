package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/coderbuzz/dockify/internal/app"
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
	appSvc    *app.Service
	sshKeyDir string
}

func NewConsoleHandler(svc *server.Service, appSvc *app.Service, sshKeyDir string) *ConsoleHandler {
	return &ConsoleHandler{serverSvc: svc, appSvc: appSvc, sshKeyDir: sshKeyDir}
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
			messageType, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			switch messageType {
			case websocket.TextMessage:
				// Keystrokes — raw text, no JSON parsing
				inCh <- ssh.Input{Data: string(msg)}

			case websocket.BinaryMessage:
				// Resize events — JSON
				var input ssh.Input
				if json.Unmarshal(msg, &input) == nil && input.Resize != nil {
					inCh <- input
				}
			}
		}
	}()

	// Block until error or context done
	select {
	case <-errCh:
	case <-ctx.Done():
	}
}

func (h *ConsoleHandler) ServeAppContainerWS(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	appID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	appData, err := h.appSvc.Get(appID)
	if err != nil || appData == nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	svr, err := h.serverSvc.Get(appData.ServerID)
	if err != nil || svr == nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("container console ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var client ssh.Connector

	if GetDevMock(r) {
		client = ssh.NewMockClient()
	} else {
		client, err = ssh.Connect(svr.Host, svr.Port, svr.User, svr.SSHKey)
		if err != nil {
			log.Printf("container console ssh connect error: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte("SSH connection failed: "+err.Error()+"\r\n"))
			return
		}
		defer client.Close()
	}

	composePath := fmt.Sprintf("/opt/dockify/apps/app-%d/docker-compose.yml", appData.ID)
	composeCmd := app.DockerComposeCmd(client)
	serviceName := appData.ContainerServiceName()

	execCmd := fmt.Sprintf("%s -f %s exec %s sh -c 'command -v bash >/dev/null 2>&1 && exec bash || exec sh'",
		composeCmd, composePath, serviceName)

	outCh, inCh, err := client.ExecPTY(ctx, execCmd, 24, 80)
	if err != nil {
		log.Printf("container console exec pty error: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Container exec failed: "+err.Error()+"\r\n"))
		return
	}

	errCh := make(chan error, 2)

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

	go func() {
		for {
			messageType, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			switch messageType {
			case websocket.TextMessage:
				inCh <- ssh.Input{Data: string(msg)}

			case websocket.BinaryMessage:
				var input ssh.Input
				if json.Unmarshal(msg, &input) == nil && input.Resize != nil {
					inCh <- input
				}
			}
		}
	}()

	select {
	case <-errCh:
	case <-ctx.Done():
	}
}
