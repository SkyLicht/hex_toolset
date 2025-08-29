package websocket

import (
	"net/http"
	"sync"
	"time"

	"hex_toolset/pkg/logger"

	"github.com/gorilla/websocket"
)

// Hub manages active clients and broadcasts messages
// Exported for reuse by managers.
type Hub struct {
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	mu         sync.RWMutex
	closed     bool
}

// NewHub constructs a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte, 1024),
		register:   make(chan *client, 128),
		unregister: make(chan *client, 128),
	}
}

// Run starts the hub event loop
func (h *Hub) Run(logg *logger.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logg.Errorf("hub panic recovered: %v", r)
		}
	}()
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
			logg.Infof("client registered: %p (total=%d)", c, len(h.clients))
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			logg.Infof("client unregistered: %p (total=%d)", c, len(h.clients))
		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// slow client, drop
					close(c.send)
					delete(h.clients, c)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Shutdown closes all client channels and stops the hub
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for c := range h.clients {
		close(c.send)
		delete(h.clients, c)
	}
	close(h.broadcast)
	close(h.register)
	close(h.unregister)
}

// Broadcast sends a message to all clients via the hub.
func (h *Hub) Broadcast(msg []byte) {
	h.broadcast <- msg
}

// client represents a websocket client
// unexported; managed via Hub and WSHandler
type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	log  *logger.Logger
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 // small; we don't expect client -> server traffic
)

func (c *client) readPump() {
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("client read panic recovered: %v", r)
		}
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(int64(maxMessageSize))
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { _ = c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Errorf("unexpected ws close: %v", err)
			}
			break
		}
	}
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("client write panic recovered: %v", r)
		}
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.log.Errorf("next writer error: %v", err)
				return
			}
			if _, err := w.Write(msg); err != nil {
				c.log.Errorf("write error: %v", err)
				_ = w.Close()
				return
			}
			// batch queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write([]byte("\n")); err != nil {
					c.log.Errorf("write joiner error: %v", err)
					break
				}
				if _, err := w.Write(<-c.send); err != nil {
					c.log.Errorf("write batch error: %v", err)
					break
				}
			}
			if err := w.Close(); err != nil {
				c.log.Errorf("writer close error: %v", err)
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.log.Errorf("ping error: %v", err)
				return
			}
		}
	}
}

// WSHandler upgrades and registers clients with the Hub
func WSHandler(h *Hub, logg *logger.Logger) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logg.Errorf("ws handler panic recovered: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logg.Errorf("upgrade error: %v", err)
			return
		}
		cl := &client{hub: h, conn: conn, send: make(chan []byte, 256), log: logg}
		h.register <- cl
		go cl.writePump()
		cl.readPump()
	}
}

// RecoverMiddleware wraps HTTP handlers with panic recovery and logging
func RecoverMiddleware(next http.Handler, logg *logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logg.Errorf("http panic recovered: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
