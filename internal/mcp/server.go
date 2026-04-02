// Package mcp provides Model Context Protocol (MCP) support for SuperTerminal.
// MCP is a standard protocol for connecting AI models to external tools and data sources.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Protocol version
const MCPVersion = "2024-11-05"

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// ServerInfo contains server metadata.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities describes server capabilities.
type Capabilities struct {
	Tools     *ToolCapabilities     `json:"tools,omitempty"`
	Resources *ResourceCapabilities `json:"resources,omitempty"`
	Prompts   *PromptCapabilities   `json:"prompts,omitempty"`
}

// ToolCapabilities describes tool capabilities.
type ToolCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourceCapabilities describes resource capabilities.
type ResourceCapabilities struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptCapabilities describes prompt capabilities.
type PromptCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolDefinition defines an MCP tool.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolResult contains tool execution results.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents content in a result.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// ResourceDefinition defines an MCP resource.
type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// PromptDefinition defines an MCP prompt.
type PromptDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument defines a prompt argument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// InitializeResult contains initialization result.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// ListToolsResult contains tools list result.
type ListToolsResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// ListResourcesResult contains resources list result.
type ListResourcesResult struct {
	Resources []ResourceDefinition `json:"resources"`
}

// ListPromptsResult contains prompts list result.
type ListPromptsResult struct {
	Prompts []PromptDefinition `json:"prompts"`
}

// CallToolParams contains tool call parameters.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ReadResourceParams contains resource read parameters.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// GetPromptParams contains prompt get parameters.
type GetPromptParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolHandler is the interface for MCP tools.
type ToolHandler interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ResourceHandler is the interface for MCP resources.
type ResourceHandler interface {
	Definition() ResourceDefinition
	Read(ctx context.Context) ([]ContentBlock, error)
}

// PromptHandler is the interface for MCP prompts.
type PromptHandler interface {
	Definition() PromptDefinition
	Get(ctx context.Context, args json.RawMessage) (string, error)
}

// Server implements an MCP server.
type Server struct {
	name         string
	version      string
	tools        map[string]ToolHandler
	resources    map[string]ResourceHandler
	prompts      map[string]PromptHandler
	toolList     []ToolDefinition
	resourceList []ResourceDefinition
	promptList   []PromptDefinition
	mu           sync.RWMutex
}

// NewServer creates a new MCP server.
func NewServer(name, version string) *Server {
	return &Server{
		name:      name,
		version:   version,
		tools:     make(map[string]ToolHandler),
		resources: make(map[string]ResourceHandler),
		prompts:   make(map[string]PromptHandler),
	}
}

// RegisterTool adds a tool to the server.
func (s *Server) RegisterTool(tool ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	def := tool.Definition()
	s.tools[def.Name] = tool
	s.toolList = append(s.toolList, def)
}

// RegisterResource adds a resource to the server.
func (s *Server) RegisterResource(resource ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	def := resource.Definition()
	s.resources[def.URI] = resource
	s.resourceList = append(s.resourceList, def)
}

// RegisterPrompt adds a prompt to the server.
func (s *Server) RegisterPrompt(prompt PromptHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	def := prompt.Definition()
	s.prompts[def.Name] = prompt
	s.promptList = append(s.promptList, def)
}

// HandleRequest processes a JSON-RPC request.
func (s *Server) HandleRequest(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(ctx, req)
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptsGet(ctx, req)
	case "ping":
		return s.handlePing(req)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: InitializeResult{
			ProtocolVersion: MCPVersion,
			Capabilities: Capabilities{
				Tools:     &ToolCapabilities{},
				Resources: &ResourceCapabilities{},
				Prompts:   &PromptCapabilities{},
			},
			ServerInfo: ServerInfo{
				Name:    s.name,
				Version: s.version,
			},
		},
	}
}

func (s *Server) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListToolsResult{Tools: s.toolList},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: "Invalid params: " + err.Error(),
			},
		}
	}
	
	s.mu.RLock()
	tool, ok := s.tools[params.Name]
	s.mu.RUnlock()
	
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: fmt.Sprintf("Tool not found: %s", params.Name),
			},
		}
	}
	
	result, err := tool.Execute(ctx, params.Arguments)
	if err != nil {
		result = ToolResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleResourcesList(req JSONRPCRequest) JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListResourcesResult{Resources: s.resourceList},
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	var params ReadResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: "Invalid params: " + err.Error(),
			},
		}
	}
	
	s.mu.RLock()
	resource, ok := s.resources[params.URI]
	s.mu.RUnlock()
	
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: fmt.Sprintf("Resource not found: %s", params.URI),
			},
		}
	}
	
	content, err := resource.Read(ctx)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInternal,
				Message: err.Error(),
			},
		}
	}
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"contents": content},
	}
}

func (s *Server) handlePromptsList(req JSONRPCRequest) JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListPromptsResult{Prompts: s.promptList},
	}
}

func (s *Server) handlePromptsGet(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	var params GetPromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: "Invalid params: " + err.Error(),
			},
		}
	}
	
	s.mu.RLock()
	prompt, ok := s.prompts[params.Name]
	s.mu.RUnlock()
	
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInvalidParams,
				Message: fmt.Sprintf("Prompt not found: %s", params.Name),
			},
		}
	}
	
	content, err := prompt.Get(ctx, params.Arguments)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrInternal,
				Message: err.Error(),
			},
		}
	}
	
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"description": prompt.Definition().Description,
			"messages": []map[string]interface{}{
				{"role": "user", "content": map[string]string{"type": "text", "text": content}},
			},
		},
	}
}

func (s *Server) handlePing(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{},
	}
}

// GetToolDefinitions returns all tool definitions.
func (s *Server) GetToolDefinitions() []ToolDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolList
}

// GetTool returns a tool by name.
func (s *Server) GetTool(name string) (ToolHandler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tool, ok := s.tools[name]
	return tool, ok
}