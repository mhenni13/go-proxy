package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"
	"github.com/mhenni13/go-proxy/internal/logging"
	"github.com/mhenni13/go-proxy/internal/config"

)

func handleWebSocket(w http.ResponseWriter, r *http.Request, upstream config.Upstream) {
	requestID := uuid.NewString()
	r.Header.Set("X-Request-ID", requestID)

	upstreamURL := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", upstream.Host, upstream.Port), Path: r.URL.Path}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, EnableCompression: true}
	upstreamConn, _, err := dialer.Dial(upstreamURL.String(), r.Header)
	if err != nil {
		logging.LoggerInstance.Log(map[string]interface{}{
			"type":       "websocket_error",
			"request_id": requestID,
			"event":      "connect_upstream",
			"url":        upstreamURL.String(),
			"client":     r.RemoteAddr,
			"error":      err.Error(),
		})
		http.Error(w, "Failed to connect upstream", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LoggerInstance.Log(map[string]interface{}{
			"type":       "websocket_error",
			"request_id": requestID,
			"event":      "upgrade_client",
			"client":     r.RemoteAddr,
			"error":      err.Error(),
		})
		return
	}
	defer clientConn.Close()

	logging.LoggerInstance.Log(map[string]interface{}{
		"type":       "websocket_event",
		"request_id": requestID,
		"event":      "connection_established",
		"client":     r.RemoteAddr,
		"upstream":   upstreamURL.String(),
	})

	errCh := make(chan error, 2)
	startKeepAlive(clientConn, requestID)
	startKeepAlive(upstreamConn, requestID)

	// Client -> Upstream
	go func() {
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			err = upstreamConn.WriteMessage(mt, msg)
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Upstream -> Client
	go func() {
		for {
			mt, msg, err := upstreamConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			err = clientConn.WriteMessage(mt, msg)
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	if err := <-errCh; err != nil {
		logging.LoggerInstance.Log(map[string]interface{}{
			"type":       "websocket_event",
			"request_id": requestID,
			"event":      "connection_closed",
			"client":     r.RemoteAddr,
			"error":      err.Error(),
		})
	}
}

func startKeepAlive(conn *websocket.Conn, requestID string) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		conn.SetPongHandler(func(appData string) error { return nil })

		for range ticker.C {
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}()
}
