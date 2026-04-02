// Package mcp provides HTTP transport for MCP server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// HTTPHandler provides HTTP/SSE transport for MCP.
type HTTPHandler struct {
	server *Server
}

// NewHTTPHandler creates a new HTTP handler for MCP.
func NewHTTPHandler(server *Server) *HTTPHandler {
	return &HTTPHandler{server: server}
}

// ServeHTTP handles HTTP requests for MCP.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle different transport modes
	if r.Header.Get("Accept") == "text/event-stream" {
		// SSE transport
		h.handleSSE(w, r)
		return
	}
	
	// Standard HTTP JSON-RPC
	h.handleJSONRPC(w, r)
}

// handleJSONRPC handles standard JSON-RPC over HTTP.
func (h *HTTPHandler) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`))
		return
	}
	
	resp := h.server.HandleRequest(context.Background(), req)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSSE handles SSE transport for MCP.
func (h *HTTPHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	
	// Send endpoint event for SSE transport
	endpoint := r.URL.Path
	w.Write([]byte("event: endpoint\n"))
	w.Write([]byte("data: " + endpoint + "\n\n"))
	flusher.Flush()
	
	// Keep connection alive
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Wait for client messages (would need to implement message handling)
			// For now, just send periodic pings
			w.Write([]byte("event: ping\n"))
			w.Write([]byte("data: {}\n\n"))
			flusher.Flush()
			
			// Wait a bit
			select {
			case <-ctx.Done():
				return
			case <-r.Context().Done():
				return
			}
		}
	}
}

// StartMCPServer starts an MCP server on the given port.
func StartMCPServer(server *Server, port int) error {
	handler := NewHTTPHandler(server)
	
	addr := fmt.Sprintf(":%d", port)
	log.Printf("MCP server starting on http://localhost%s/mcp", addr)
	
	http.Handle("/mcp", handler)
	http.HandleFunc("/mcp/sse", handler.ServeHTTP)
	
	return http.ListenAndServe(addr, nil)
}