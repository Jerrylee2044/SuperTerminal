// Package webui provides the HTTP/WebSocket server for SuperTerminal.
package webui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	
	"superterminal/internal/engine"
)

// Server provides HTTP and WebSocket endpoints for the Web UI.
type Server struct {
	engine     *engine.Engine
	eventBus   *engine.EventBus
	port       int
	upgrader   websocket.Upgrader
	clients    map[*websocket.Conn]bool
	clientsMu  sync.RWMutex
	mu         sync.Mutex // Protects WebSocket writes
	eventCh    chan engine.Event
	staticPath string
}

// ServerOptions configures the Web UI server.
type ServerOptions struct {
	Port       int
	StaticPath string
}

// NewServer creates a new Web UI server.
func NewServer(e *engine.Engine, opts ServerOptions) *Server {
	if opts.Port <= 0 {
		opts.Port = 8080
	}
	if opts.StaticPath == "" {
		opts.StaticPath = "web"
	}
	
	return &Server{
		engine:     e,
		eventBus:   e.GetEventBus(),
		port:       opts.Port,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		clients:    make(map[*websocket.Conn]bool),
		staticPath: opts.StaticPath,
	}
}

// Start begins the HTTP/WebSocket server.
func (s *Server) Start() error {
	// Subscribe to all events
	s.eventCh = s.eventBus.SubscribeAll()
	go s.broadcastEvents()

	// Setup routes
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/api/input", s.handleInput)
	http.HandleFunc("/api/messages", s.handleMessages)
	http.HandleFunc("/api/cost", s.handleCost)
	http.HandleFunc("/api/status", s.handleStatus)
	http.HandleFunc("/api/config", s.handleConfig)
	http.HandleFunc("/api/tools", s.handleTools)
	http.HandleFunc("/api/clear", s.handleClear)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Web UI starting on http://localhost%s", addr)
	
	return http.ListenAndServe(addr, nil)
}

// handleIndex serves the main HTML page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Try to serve from static path
	indexPath := s.staticPath + "/index.html"
	
	// Check if file exists
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}
	
	// Fallback to embedded HTML
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fallbackHTML))
}

// handleWebSocket handles WebSocket connections.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Register client
	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	log.Printf("WebSocket client connected")

	// Send initial state
	s.sendInitialState(conn)

	// Handle incoming messages
	go s.handleClientMessages(conn)
}

// sendInitialState sends the current state to a new WebSocket client.
func (s *Server) sendInitialState(conn *websocket.Conn) {
	state := map[string]interface{}{
		"type":    "initial_state",
		"session": s.engine.GetSession().GetInfo(),
		"cost":    s.engine.GetCost(),
		"status":  s.engine.GetStatus(),
		"messages": s.engine.GetSession().GetMessages(),
	}
	conn.WriteJSON(state)
}

// handleClientMessages reads messages from a WebSocket client.
func (s *Server) handleClientMessages(conn *websocket.Conn) {
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		conn.Close()
	}()

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		// Handle different message types
		msgType, _ := msg["type"].(string)
		
		switch msgType {
		case "input":
			// New format: { type: "input", content: "text" }
			if content, ok := msg["content"].(string); ok {
				s.engine.ProcessInput(content, engine.SourceWeb)
				conn.WriteJSON(map[string]interface{}{
					"type":    "input_received",
					"data":    map[string]string{"content": content},
				})
			}
			
		case "command":
			if cmd, ok := msg["content"].(string); ok {
				s.engine.ProcessInput(cmd, engine.SourceWeb)
			}
			
		case "cancel":
			s.engine.GetEventBus().Publish(engine.NewEvent(engine.EventError, "cancel_requested", engine.SourceWeb))
			conn.WriteJSON(map[string]interface{}{"type": "cancelled"})
			
		case "ping":
			conn.WriteJSON(map[string]interface{}{"type": "pong"})
			
		case "get_status":
			conn.WriteJSON(map[string]interface{}{
				"type": "status",
				"data": map[string]string{"status": string(s.engine.GetStatus())},
			})
			
		case "get_cost":
			conn.WriteJSON(map[string]interface{}{
				"type": "cost",
				"data": s.engine.GetCost(),
			})
			
		case "get_session":
			session := s.engine.GetSession()
			conn.WriteJSON(map[string]interface{}{
				"type": "session",
				"data": map[string]interface{}{
					"id":            session.ID,
					"message_count": len(session.Messages),
					"info":          session.GetInfo(),
				},
			})
			
		case "load_session":
			if data, ok := msg["data"].(map[string]interface{}); ok {
				if id, ok := data["id"].(string); ok {
					if err := s.engine.LoadSession(id); err != nil {
						conn.WriteJSON(map[string]interface{}{
							"type":  "error",
							"error": err.Error(),
						})
					} else {
						conn.WriteJSON(map[string]interface{}{
							"type": "session_loaded",
							"data": map[string]string{"id": id},
						})
					}
				}
			}
			
		case "save_session":
			session := s.engine.GetSession()
			if err := s.engine.SaveSession(session.ID); err != nil {
				conn.WriteJSON(map[string]interface{}{
					"type":  "error",
					"error": err.Error(),
				})
			} else {
				conn.WriteJSON(map[string]interface{}{
					"type": "session_saved",
					"data": map[string]string{"id": session.ID},
				})
			}
			
		case "new_session":
			// Clear current session
			s.engine.GetSession().Clear()
			conn.WriteJSON(map[string]interface{}{
				"type": "session_loaded",
				"data": map[string]string{"id": "new"},
			})
			
		case "list_sessions":
			sessions, err := s.engine.ListSessions()
			if err != nil {
				conn.WriteJSON(map[string]interface{}{
					"type":  "error",
					"error": err.Error(),
				})
			} else {
				conn.WriteJSON(map[string]interface{}{
					"type": "session_list",
					"data": sessions,
				})
			}
			
		case "update_settings":
			if data, ok := msg["data"].(map[string]interface{}); ok {
				config := s.engine.GetConfig()
				if model, ok := data["model"].(string); ok {
					config.Model = model
				}
				if maxTokens, ok := data["max_tokens"].(float64); ok {
					config.MaxTokens = int(maxTokens)
				}
				conn.WriteJSON(map[string]interface{}{
					"type": "settings_updated",
				})
			}
			
		case "permission_response":
			// Handle permission response from user
			// For now, auto-approve in engine
			conn.WriteJSON(map[string]interface{}{
				"type": "permission_handled",
			})
			
		default:
			// Unknown command type
			conn.WriteJSON(map[string]interface{}{
				"type":  "error",
				"error": "Unknown command type: " + msgType,
			})
		}
	}
}

// broadcastEvents sends engine events to all WebSocket clients.
func (s *Server) broadcastEvents() {
	for event := range s.eventCh {
		s.clientsMu.RLock()
		
		// Create a slice to avoid holding lock while writing
		connections := make([]*websocket.Conn, 0, len(s.clients))
		for conn := range s.clients {
			connections = append(connections, conn)
		}
		s.clientsMu.RUnlock()
		
		// Broadcast to all connections
		for _, conn := range connections {
			s.mu.Lock()
			err := conn.WriteJSON(map[string]interface{}{
				"type": event.Type,
				"data": event.Data,
				"source": event.Source,
			})
			s.mu.Unlock()
			if err != nil {
				// Connection error, will be cleaned up
				continue
			}
		}
	}
}

// handleInput handles POST /api/input.
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	s.engine.ProcessInput(req.Text, engine.SourceWeb)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// InputRequest represents an input request.
type InputRequest struct {
	Text string `json:"text"`
}

// handleMessages handles GET /api/messages.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.engine.GetSession().GetMessages())
}

// handleCost handles GET /api/cost.
func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.engine.GetCost())
}

// handleStatus handles GET /api/status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":      s.engine.GetStatus(),
		"session":     s.engine.GetSession().GetInfo(),
		"cost":        s.engine.GetCost(),
	}

	name, toolUseID, ok := s.engine.GetCurrentTool()
	if ok {
		status["current_tool"] = map[string]string{
			"name":       name,
			"tool_use_id": toolUseID,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleConfig handles GET /api/config.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	config := s.engine.GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleTools handles GET /api/tools.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.engine.GetToolManager().GetToolDefinitions())
}

// handleClear handles POST /api/clear.
func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	s.engine.GetSession().Clear()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Stop shuts down the server.
func (s *Server) Stop() {
	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clientsMu.Unlock()
}

// fallbackHTML is a minimal embedded HTML for when web/index.html is not found.
const fallbackHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>SuperTerminal</title>
    <style>
        body { font-family: sans-serif; background: #1a1a2e; color: #e0e0e0; padding: 40px; }
        h1 { color: #7c3aed; }
        p { color: #6b7280; }
        a { color: #7c3aed; }
    </style>
</head>
<body>
    <h1>SuperTerminal Web UI</h1>
    <p>Web interface file not found. Please ensure web/index.html exists.</p>
    <p>WebSocket endpoint: <a href="/ws">/ws</a></p>
    <p>API endpoints: /api/input, /api/messages, /api/cost, /api/status</p>
</body>
</html>`