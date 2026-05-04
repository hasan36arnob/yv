package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// WebSocket Message Types
const (
	MsgTaskCreated    = "task_created"
	MsgTaskUpdated    = "task_updated"
	MsgTaskDeleted    = "task_deleted"
	MsgUserJoined     = "user_joined"
	MsgUserLeft       = "user_left"
	MsgWorkspaceUpdate = "workspace_update"
)

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	UserID  uint        `json:"user_id"`
	Time    int64       `json:"timestamp"`
}

// Client represents a WebSocket connection
type Client struct {
	ID        uint
	WorkspaceID string
	Conn      *websocket.Conn
	Send      chan []byte
	Hub       *Hub
}

// Hub manages WebSocket connections
type Hub struct {
	clients    map[*Client]bool
	workspaces map[string]map[*Client]bool // workspaceID -> clients
	register   chan *Client
	unregister chan *Client
	broadcast  chan WebSocketMessage
	mu         sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		workspaces: make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan WebSocketMessage, 256),
	}
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			
			// Add to workspace
			if _, exists := h.workspaces[client.WorkspaceID]; !exists {
				h.workspaces[client.WorkspaceID] = make(map[*Client]bool)
			}
			h.workspaces[client.WorkspaceID][client] = true
			
			// Notify workspace
			h.broadcastToWorkspace(client.WorkspaceID, MsgUserJoined, map[string]interface{}{
				"user_id": client.ID,
				"online":  len(h.workspaces[client.WorkspaceID]),
			})
			h.mu.Unlock()
			
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				
				// Remove from workspace
				if clients, exists := h.workspaces[client.WorkspaceID]; exists {
					delete(clients, client)
					if len(clients) == 0 {
						delete(h.workspaces, client.WorkspaceID)
					} else {
						// Notify remaining users
						h.broadcastToWorkspace(client.WorkspaceID, MsgUserLeft, map[string]interface{}{
							"user_id": client.ID,
							"online":  len(clients),
						})
					}
				}
				close(client.Send)
			}
			h.mu.Unlock()
			
		case message := <-h.broadcast:
			h.mu.RLock()
			// Broadcast to specific workspace
			if clients, exists := h.workspaces[message.Payload.(map[string]interface{})["workspace_id"].(string)]; exists {
				for client := range clients {
					select {
					case client.Send <- h.encodeMessage(message):
					default:
						close(client.Send)
						delete(h.clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// broadcastToWorkspace sends message to all clients in a workspace
func (h *Hub) broadcastToWorkspace(workspaceID string, msgType string, payload interface{}) {
	h.broadcast <- WebSocketMessage{
		Type:    msgType,
		Payload: payload,
		Time:    time.Now().Unix(),
	}
}

func (h *Hub) encodeMessage(msg WebSocketMessage) []byte {
	data, _ := json.Marshal(msg)
	return data
}

// handleWebSocket manages WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// Authenticate
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
		return
	}

	tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
	claims, err := validateToken(tokenString)
	if err != nil {
		http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	// Get workspace ID from query
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		conn.Close()
		http.Error(w, `{"error":"Workspace ID required"}`, http.StatusBadRequest)
		return
	}

	// Verify workspace membership
	var membership TeamMember
	if err := db.Where("workspace_id = ? AND user_id = ? AND is_active = ?", 
		workspaceID, claims.UserID, true).First(&membership).Error; err != nil {
		conn.Close()
		http.Error(w, `{"error":"Access denied to workspace"}`, http.StatusUnauthorized)
		return
	}

	// Create client
	client := &Client{
		ID:          claims.UserID,
		WorkspaceID: workspaceID,
		Conn:        conn,
		Send:        make(chan []byte, 256),
		Hub:         wsHub,
	}

	// Register
	wsHub.register <- client

	// Send welcome message
	 welcomeMsg := WebSocketMessage{
		Type: "connected",
		Payload: map[string]interface{}{
			"user_id":    claims.UserID,
			"workspace":  workspaceID,
			"role":       membership.Role,
			"connected":  true,
		},
		Time: time.Now().Unix(),
	}
	conn.WriteJSON(welcomeMsg)

	// Start pumps
	go client.readPump()
	go client.writePump()
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	for {
		var message WebSocketMessage
		err := c.Conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle ping/pong for keepalive
		if message.Type == "ping" {
			c.Conn.WriteJSON(WebSocketMessage{Type: "pong", Time: time.Now().Unix()})
			continue
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			// Send ping to keep connection alive
			c.Conn.WriteJSON(WebSocketMessage{Type: "ping", Time: time.Now().Unix()})
		}
	}
}
