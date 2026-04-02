package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

// MockTool is a mock tool for testing.
type MockTool struct {
	name        string
	description string
	schema      map[string]interface{}
	result      string
	err         error
}

func (t *MockTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        t.name,
		Description: t.description,
		InputSchema: t.schema,
	}
}

func (t *MockTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.err != nil {
		return ToolResult{}, t.err
	}
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: t.result}},
	}, nil
}

// MockResource is a mock resource for testing.
type MockResource struct {
	uri         string
	name        string
	description string
	mimeType    string
	content     string
	err         error
}

func (r *MockResource) Definition() ResourceDefinition {
	return ResourceDefinition{
		URI:         r.uri,
		Name:        r.name,
		Description: r.description,
		MimeType:    r.mimeType,
	}
}

func (r *MockResource) Read(ctx context.Context) ([]ContentBlock, error) {
	if r.err != nil {
		return nil, r.err
	}
	return []ContentBlock{{Type: "text", Text: r.content}}, nil
}

// MockPrompt is a mock prompt for testing.
type MockPrompt struct {
	name        string
	description string
	content     string
	err         error
}

func (p *MockPrompt) Definition() PromptDefinition {
	return PromptDefinition{
		Name:        p.name,
		Description: p.description,
	}
}

func (p *MockPrompt) Get(ctx context.Context, args json.RawMessage) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.content, nil
}

// TestServerInitialize tests initialize request.
func TestServerInitialize(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	
	result, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Errorf("Expected InitializeResult, got %T", resp.Result)
	}
	
	if result.ProtocolVersion != MCPVersion {
		t.Errorf("Expected protocol version %s, got %s", MCPVersion, result.ProtocolVersion)
	}
	
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", result.ServerInfo.Name)
	}
}

// TestServerToolRegistration tests tool registration.
func TestServerToolRegistration(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	tool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
		schema: map[string]interface{}{
			"type": "object",
		},
		result: "tool executed",
	}
	
	server.RegisterTool(tool)
	
	// Test tools/list
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	
	result, ok := resp.Result.(ListToolsResult)
	if !ok {
		t.Errorf("Expected ListToolsResult, got %T", resp.Result)
	}
	
	if len(result.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(result.Tools))
	}
	
	if result.Tools[0].Name != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", result.Tools[0].Name)
	}
}

// TestServerToolCall tests tool execution.
func TestServerToolCall(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	tool := &MockTool{
		name:        "echo",
		description: "Echo input",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{"type": "string"},
			},
		},
		result: "echoed",
	}
	
	server.RegisterTool(tool)
	
	// Test tools/call
	params := CallToolParams{
		Name:      "echo",
		Arguments: json.RawMessage(`{"message":"hello"}`),
	}
	paramsJSON, _ := json.Marshal(params)
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  paramsJSON,
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	
	result, ok := resp.Result.(ToolResult)
	if !ok {
		t.Errorf("Expected ToolResult, got %T", resp.Result)
	}
	
	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(result.Content))
	}
	
	if result.Content[0].Text != "echoed" {
		t.Errorf("Expected text 'echoed', got '%s'", result.Content[0].Text)
	}
}

// TestServerToolNotFound tests tool not found error.
func TestServerToolNotFound(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	params := CallToolParams{Name: "nonexistent"}
	paramsJSON, _ := json.Marshal(params)
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  paramsJSON,
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error == nil {
		t.Error("Expected error for nonexistent tool")
	}
	
	if resp.Error.Code != ErrInvalidParams {
		t.Errorf("Expected error code %d, got %d", ErrInvalidParams, resp.Error.Code)
	}
}

// TestServerResource tests resource handling.
func TestServerResource(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	resource := &MockResource{
		uri:         "test://resource",
		name:        "Test Resource",
		description: "A test resource",
		content:     "resource content",
	}
	
	server.RegisterResource(resource)
	
	// Test resources/list
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "resources/list",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	
	result, ok := resp.Result.(ListResourcesResult)
	if !ok {
		t.Errorf("Expected ListResourcesResult, got %T", resp.Result)
	}
	
	if len(result.Resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(result.Resources))
	}
}

// TestServerPrompt tests prompt handling.
func TestServerPrompt(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	prompt := &MockPrompt{
		name:        "test_prompt",
		description: "A test prompt",
		content:     "This is a test prompt content",
	}
	
	server.RegisterPrompt(prompt)
	
	// Test prompts/list
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "prompts/list",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	
	result, ok := resp.Result.(ListPromptsResult)
	if !ok {
		t.Errorf("Expected ListPromptsResult, got %T", resp.Result)
	}
	
	if len(result.Prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(result.Prompts))
	}
}

// TestServerPing tests ping request.
func TestServerPing(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "ping",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
}

// TestServerMethodNotFound tests method not found error.
func TestServerMethodNotFound(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      8,
		Method:  "unknown_method",
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error == nil {
		t.Error("Expected error for unknown method")
	}
	
	if resp.Error.Code != ErrMethodNotFound {
		t.Errorf("Expected error code %d, got %d", ErrMethodNotFound, resp.Error.Code)
	}
}

// TestServerInvalidParams tests invalid params error.
func TestServerInvalidParams(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      9,
		Method:  "tools/call",
		Params:  json.RawMessage(`invalid json`),
	}
	
	resp := server.HandleRequest(context.Background(), req)
	
	if resp.Error == nil {
		t.Error("Expected error for invalid params")
	}
	
	if resp.Error.Code != ErrInvalidParams {
		t.Errorf("Expected error code %d, got %d", ErrInvalidParams, resp.Error.Code)
	}
}