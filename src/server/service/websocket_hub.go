package service

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/casapps/wthr/src/server/model"
	"github.com/gorilla/websocket"
)

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	// "notification", "ping", "pong"
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	// "user-{userID}" or "admin-{adminID}"
	ID       string
	Conn     *websocket.Conn
	Hub      *WebSocketHub
	Send     chan []byte
	UserID   *int
	AdminID  *int
	LastPing time.Time
}

// WebSocketHub manages all WebSocket connections and broadcasts
type WebSocketHub struct {
	// Client management
	clients    map[string]*WebSocketClient
	clientsMux sync.RWMutex

	// Channels for client registration/unregistration
	register   chan *WebSocketClient
	unregister chan *WebSocketClient

	// Broadcast channels
	broadcast chan *WebSocketMessage

	// Shutdown
	done chan struct{}
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[string]*WebSocketClient),
		register:   make(chan *WebSocketClient, 10),
		unregister: make(chan *WebSocketClient, 10),
		broadcast:  make(chan *WebSocketMessage, 100),
		done:       make(chan struct{}),
	}
}

// Run starts the WebSocket hub (run in goroutine)
func (h *WebSocketHub) Run() {
	// Ping ticker (every 30 seconds)
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Cleanup ticker (every 5 minutes)
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case client := <-h.register:
			h.clientsMux.Lock()
			h.clients[client.ID] = client
			h.clientsMux.Unlock()
			log.Printf("WebSocket client registered: %s (total: %d)", client.ID, len(h.clients))

		case client := <-h.unregister:
			h.clientsMux.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
				log.Printf("WebSocket client unregistered: %s (total: %d)", client.ID, len(h.clients))
			}
			h.clientsMux.Unlock()

		case message := <-h.broadcast:
			h.broadcastToAll(message)

		case <-pingTicker.C:
			h.pingClients()

		case <-cleanupTicker.C:
			h.cleanupStaleConnections()

		case <-h.done:
			log.Println("WebSocket hub shutting down")
			return
		}
	}
}

// Stop stops the WebSocket hub
func (h *WebSocketHub) Stop() {
	close(h.done)

	// Close all client connections
	h.clientsMux.Lock()
	for _, client := range h.clients {
		if client.Conn != nil {
			client.Conn.Close()
		}
	}
	h.clientsMux.Unlock()
}

// RegisterClient registers a new WebSocket client
func (h *WebSocketHub) RegisterClient(client *WebSocketClient) {
	h.register <- client
}

// UnregisterClient unregisters a WebSocket client
func (h *WebSocketHub) UnregisterClient(client *WebSocketClient) {
	h.unregister <- client
}

// BroadcastToUser broadcasts a notification to a specific user
func (h *WebSocketHub) BroadcastToUser(userID int, notification *models.Notification) {
	clientID := ClientIDForUser(userID)

	h.clientsMux.RLock()
	client, ok := h.clients[clientID]
	h.clientsMux.RUnlock()

	if ok {
		message := &WebSocketMessage{
			Type: "notification",
			Data: notification,
		}
		h.sendToClient(client, message)
	}
}

// BroadcastToAdmin broadcasts a notification to a specific admin
func (h *WebSocketHub) BroadcastToAdmin(adminID int, notification *models.Notification) {
	clientID := ClientIDForAdmin(adminID)

	h.clientsMux.RLock()
	client, ok := h.clients[clientID]
	h.clientsMux.RUnlock()

	if ok {
		message := &WebSocketMessage{
			Type: "notification",
			Data: notification,
		}
		h.sendToClient(client, message)
	}
}

// BroadcastToAllUsers broadcasts a message to all user clients
func (h *WebSocketHub) BroadcastToAllUsers(message *WebSocketMessage) {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	for id, client := range h.clients {
		if client.UserID != nil {
			go h.sendToClient(client, message)
			log.Printf("Broadcast to user client: %s", id)
		}
	}
}

// BroadcastToAllAdmins broadcasts a message to all admin clients
func (h *WebSocketHub) BroadcastToAllAdmins(message *WebSocketMessage) {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	for id, client := range h.clients {
		if client.AdminID != nil {
			go h.sendToClient(client, message)
			log.Printf("Broadcast to admin client: %s", id)
		}
	}
}

// broadcastToAll broadcasts a message to all connected clients
func (h *WebSocketHub) broadcastToAll(message *WebSocketMessage) {
	h.clientsMux.RLock()
	clients := make([]*WebSocketClient, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.clientsMux.RUnlock()

	for _, client := range clients {
		h.sendToClient(client, message)
	}
}

// sendToClient sends a message to a specific client
func (h *WebSocketHub) sendToClient(client *WebSocketClient, message *WebSocketMessage) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling WebSocket message: %v", err)
		return
	}

	select {
	case client.Send <- data:
		// Message sent successfully
	default:
		// Client send buffer is full, close connection
		h.UnregisterClient(client)
		client.Conn.Close()
	}
}

// pingClients sends ping messages to all clients
func (h *WebSocketHub) pingClients() {
	h.clientsMux.RLock()
	clients := make([]*WebSocketClient, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.clientsMux.RUnlock()

	pingMessage := &WebSocketMessage{
		Type: "ping",
		Data: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}

	for _, client := range clients {
		h.sendToClient(client, pingMessage)
	}
}

// cleanupStaleConnections removes clients that haven't responded to pings
func (h *WebSocketHub) cleanupStaleConnections() {
	h.clientsMux.Lock()
	defer h.clientsMux.Unlock()

	now := time.Now()
	staleClients := make([]*WebSocketClient, 0)

	for _, client := range h.clients {
		// If no ping in 2 minutes, consider stale
		if now.Sub(client.LastPing) > 2*time.Minute {
			staleClients = append(staleClients, client)
		}
	}

	for _, client := range staleClients {
		delete(h.clients, client.ID)
		close(client.Send)
		client.Conn.Close()
		log.Printf("Removed stale WebSocket connection: %s", client.ID)
	}
}

// GetConnectedCount returns the number of connected clients
func (h *WebSocketHub) GetConnectedCount() int {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()
	return len(h.clients)
}

// GetConnectedUserCount returns the number of connected user clients
func (h *WebSocketHub) GetConnectedUserCount() int {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	count := 0
	for _, client := range h.clients {
		if client.UserID != nil {
			count++
		}
	}
	return count
}

// GetConnectedAdminCount returns the number of connected admin clients
func (h *WebSocketHub) GetConnectedAdminCount() int {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	count := 0
	for _, client := range h.clients {
		if client.AdminID != nil {
			count++
		}
	}
	return count
}

// IsUserConnected checks if a specific user is connected
func (h *WebSocketHub) IsUserConnected(userID int) bool {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	clientID := ClientIDForUser(userID)
	_, ok := h.clients[clientID]
	return ok
}

// IsAdminConnected checks if a specific admin is connected
func (h *WebSocketHub) IsAdminConnected(adminID int) bool {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	clientID := ClientIDForAdmin(adminID)
	_, ok := h.clients[clientID]
	return ok
}

// ClientIDForUser generates a client ID for a user
func ClientIDForUser(userID int) string {
	return "user-" + strconv.Itoa(userID)
}

// ClientIDForAdmin generates a client ID for an admin
func ClientIDForAdmin(adminID int) string {
	return "admin-" + strconv.Itoa(adminID)
}

// ReadPump handles reading messages from the WebSocket connection
func (c *WebSocketClient) ReadPump() {
	defer func() {
		c.Hub.UnregisterClient(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.LastPing = time.Now()
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages (e.g., pong responses)
		var wsMessage WebSocketMessage
		if err := json.Unmarshal(message, &wsMessage); err == nil {
			if wsMessage.Type == "pong" {
				c.LastPing = time.Now()
			}
		}
	}
}

// WritePump handles writing messages to the WebSocket connection
func (c *WebSocketClient) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Coalesce queued messages
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
