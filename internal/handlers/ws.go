package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WebSocketHub manages WebSocket connections and broadcasts events.
type WebSocketHub struct {
	broker  *broker.Broker
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewWebSocketHub creates a new WebSocket hub.
func NewWebSocketHub(b *broker.Broker, logger *slog.Logger) *WebSocketHub {
	return &WebSocketHub{
		broker:  b,
		clients: make(map[*websocket.Conn]bool),
		logger:  logger,
	}
}

// Start subscribes to Redis pub/sub and broadcasts to connected clients.
func (h *WebSocketHub) Start(ctx context.Context) {
	go h.subscribe(ctx)
}

func (h *WebSocketHub) subscribe(ctx context.Context) {
	sub := h.broker.Subscribe(ctx)
	defer sub.Close()

	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.broadcast([]byte(msg.Payload))
		}
	}
}

func (h *WebSocketHub) broadcast(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			h.logger.Debug("websocket write error", "error", err)
			conn.Close()
			go h.removeClient(conn)
		}
	}
}

func (h *WebSocketHub) addClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
	h.logger.Debug("websocket client connected", "total", len(h.clients))
}

func (h *WebSocketHub) removeClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
	h.logger.Debug("websocket client disconnected", "total", len(h.clients))
}

// HandleWS handles WebSocket upgrade requests.
func (h *WebSocketHub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade error", "error", err)
		return
	}

	h.addClient(conn)

	// Keep connection alive by reading pings
	go func() {
		defer func() {
			h.removeClient(conn)
			conn.Close()
		}()

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
