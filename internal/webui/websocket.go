// Package webui provides WebSocket support for real-time communication.
package webui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	
	"superterminal/internal/engine"
)

// WebSocketServer handles WebSocket connections.
type WebSocketServer struct {
	engine     *engine.Engine
	clients    map[*websocket.Conn]*WSClient
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	broadcast  chan []byte
	mu         sync.RWMutex
	upgrader   websocket.Upgrader
}

// WSClient represents a WebSocket client.
type WSClient struct {
	conn      *websocket.Conn
	send      chan []byte
	id        string
	createdAt time.Time
}

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
	Time    time.Time       `json:"time"`
}

// WSCommand represents a command from client.
type WSCommand struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	ID      string `json:"id,omitempty"`
}

// WSResponse represents a response to client.
type WSResponse struct {
	Type    string      `json:"type"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Time    time.Time   `json:"time"`
}

// NewWebSocketServer creates a new WebSocket server.
func NewWebSocketServer(e *engine.Engine) *WebSocketServer {
	return &WebSocketServer{
		engine:     e,
		clients:    make(map[*websocket.Conn]*WSClient),
		register:   make(chan *websocket.Conn, 10),
		unregister: make(chan *websocket.Conn, 10),
		broadcast:  make(chan []byte, 100),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}
}

// Start starts the WebSocket server loop.
func (ws *WebSocketServer) Start() {
	// Subscribe to engine events via channel
	eventBus := ws.engine.GetEventBus()
	eventChan := eventBus.SubscribeAll()
	
	// Start event handler
	go ws.handleEventChannel(eventChan)

	// Start client management loop
	go ws.run()
}

// handleEventChannel handles events from the engine event channel.
func (ws *WebSocketServer) handleEventChannel(eventChan chan engine.Event) {
	for event := range eventChan {
		msg := WSMessage{
			Type: string(event.Type),
			Data: json.RawMessage(marshalJSON(event.Data)),
			Time: time.Now(),
		}
		ws.broadcast <- marshalJSON(msg)
	}
}

// run handles client registration/unregistration and broadcasting.
func (ws *WebSocketServer) run() {
	for {
		select {
		case conn := <-ws.register:
			ws.mu.Lock()
			client := &WSClient{
				conn:      conn,
				send:      make(chan []byte, 100),
				id:        generateClientID(),
				createdAt: time.Now(),
			}
			ws.clients[conn] = client
			ws.mu.Unlock()
			
			// Start client write loop
			go ws.writePump(client)
			
			// Send welcome message
			welcome := WSResponse{
				Type:    "connected",
				Success: true,
				Data:    map[string]string{"client_id": client.id},
				Time:    time.Now(),
			}
			client.send <- marshalJSON(welcome)

		case conn := <-ws.unregister:
			ws.mu.Lock()
			if client, ok := ws.clients[conn]; ok {
				close(client.send)
				delete(ws.clients, conn)
				conn.Close()
			}
			ws.mu.Unlock()

		case msg := <-ws.broadcast:
			ws.mu.RLock()
			for _, client := range ws.clients {
				select {
				case client.send <- msg:
				default:
					// Client buffer full, skip
				}
			}
			ws.mu.RUnlock()
		}
	}
}

// HandleWebSocket handles WebSocket upgrade and message processing.
func (ws *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Register client
	ws.register <- conn

	// Read messages from client
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			ws.unregister <- conn
			break
		}

		// Process message
		ws.processMessage(conn, msg)
	}
}

// processMessage processes a message from client.
func (ws *WebSocketServer) processMessage(conn *websocket.Conn, msg []byte) {
	var cmd WSCommand
	if err := json.Unmarshal(msg, &cmd); err != nil {
		ws.sendError(conn, "Invalid message format")
		return
	}

	switch cmd.Type {
	case "input":
		// User input message
		if cmd.Content != "" {
			ws.engine.ProcessInput(cmd.Content, engine.SourceWeb)
			ws.sendSuccess(conn, "input_received", map[string]string{"content": cmd.Content})
		}

	case "command":
		// Slash command - process as input with / prefix
		if cmd.Content != "" {
			ws.engine.ProcessInput(cmd.Content, engine.SourceWeb)
			ws.sendSuccess(conn, "command_executed", map[string]string{"command": cmd.Content})
		}

	case "cancel":
		// Cancel current operation - send interrupt signal
		ws.engine.GetEventBus().Publish(engine.NewEvent(engine.EventError, "cancel_requested", engine.SourceWeb))
		ws.sendSuccess(conn, "cancelled", nil)

	case "ping":
		// Ping/pong for heartbeat
		ws.sendSuccess(conn, "pong", nil)

	case "get_status":
		// Get engine status
		status := ws.engine.GetStatus()
		ws.sendSuccess(conn, "status", map[string]string{"status": string(status)})

	case "get_cost":
		// Get cost info
		cost := ws.engine.GetCostForDisplay()
		ws.sendSuccess(conn, "cost", cost)

	case "get_session":
		// Get session info
		session := ws.engine.GetSession()
		ws.sendSuccess(conn, "session", map[string]int{"message_count": len(session.Messages)})

	case "load_session":
		// Load a session
		if cmd.ID != "" {
			if err := ws.engine.LoadSession(cmd.ID); err != nil {
				ws.sendError(conn, "Failed to load session: "+err.Error())
			} else {
				ws.sendSuccess(conn, "session_loaded", map[string]string{"id": cmd.ID})
			}
		}

	case "save_session":
		// Save current session
		session := ws.engine.GetSession()
		if err := ws.engine.SaveSession(session.ID); err != nil {
			ws.sendError(conn, "Failed to save session: "+err.Error())
		} else {
			ws.sendSuccess(conn, "session_saved", map[string]string{"id": session.ID})
		}

	default:
		ws.sendError(conn, "Unknown command type: "+cmd.Type)
	}
}

// handleEvent handles events from the engine and broadcasts to clients.
func (ws *WebSocketServer) handleEvent(event engine.Event) {
	msg := WSMessage{
		Type: string(event.Type),
		Data: json.RawMessage(marshalJSON(event.Data)),
		Time: time.Now(),
	}
	ws.broadcast <- marshalJSON(msg)
}

// writePump writes messages to client connection.
func (ws *WebSocketServer) writePump(client *WSClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				// Channel closed
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			// Send ping for heartbeat
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendSuccess sends a success response to client.
func (ws *WebSocketServer) sendSuccess(conn *websocket.Conn, typ string, data interface{}) {
	ws.mu.RLock()
	client, ok := ws.clients[conn]
	ws.mu.RUnlock()

	if !ok {
		return
	}

	resp := WSResponse{
		Type:    typ,
		Success: true,
		Data:    data,
		Time:    time.Now(),
	}
	client.send <- marshalJSON(resp)
}

// sendError sends an error response to client.
func (ws *WebSocketServer) sendError(conn *websocket.Conn, errMsg string) {
	ws.mu.RLock()
	client, ok := ws.clients[conn]
	ws.mu.RUnlock()

	if !ok {
		return
	}

	resp := WSResponse{
		Type:    "error",
		Success: false,
		Error:   errMsg,
		Time:    time.Now(),
	}
	client.send <- marshalJSON(resp)
}

// Stop stops the WebSocket server.
func (ws *WebSocketServer) Stop() {
	ws.mu.Lock()
	for _, client := range ws.clients {
		close(client.send)
		client.conn.Close()
	}
	ws.clients = make(map[*websocket.Conn]*WSClient)
	ws.mu.Unlock()
}

// GetClientCount returns the number of connected clients.
func (ws *WebSocketServer) GetClientCount() int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return len(ws.clients)
}

// generateClientID generates a unique client ID.
func generateClientID() string {
	return fmt.Sprintf("client-%d", time.Now().UnixNano())
}

// marshalJSON marshals to JSON, returning empty slice on error.
func marshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte{}
	}
	return data
}