package mcp

import (
	"encoding/json"
	"testing"
)

// TestClientConfig tests client configuration.
func TestClientConfig(t *testing.T) {
	config := ClientConfig{
		Name:    "test-server",
		Type:    "stdio",
		Command: "test-command",
		Args:    []string{"--arg1", "--arg2"},
		Env:     map[string]string{"TEST": "value"},
	}

	if config.Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", config.Name)
	}

	if config.Type != "stdio" {
		t.Errorf("Expected type 'stdio', got '%s'", config.Type)
	}

	if len(config.Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(config.Args))
	}
}

// TestNewClient tests client creation.
func TestNewClient(t *testing.T) {
	config := ClientConfig{
		Name: "test",
		Type: "stdio",
	}

	client := NewClient(config)
	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.config.Name != "test" {
		t.Errorf("Expected config name 'test', got '%s'", client.config.Name)
	}

	if client.connected {
		t.Error("Client should not be connected initially")
	}
}

// TestNextID tests ID generation.
func TestNextID(t *testing.T) {
	config := ClientConfig{Name: "test"}
	client := NewClient(config)

	id1 := client.nextID()
	id2 := client.nextID()

	if id1 >= id2 {
		t.Errorf("IDs should increment: id1=%d, id2=%d", id1, id2)
	}

	if id1 != 1 {
		t.Errorf("First ID should be 1, got %d", id1)
	}
}

// TestClientManager tests client manager.
func TestClientManager(t *testing.T) {
	cm := NewClientManager()
	if cm == nil {
		t.Fatal("Expected manager to be created")
	}

	// List should be empty initially
	names := cm.ListClients()
	if len(names) != 0 {
		t.Errorf("Expected empty list, got %d", len(names))
	}
}

// TestClientManagerAddRemove tests adding and removing clients.
func TestClientManagerAddRemove(t *testing.T) {
	cm := NewClientManager()

	// Adding a client with invalid config should fail
	// (since we can't actually connect to a non-existent server)
	_, err := cm.AddClient("test", ClientConfig{
		Name:    "test",
		Type:    "stdio",
		Command: "nonexistent-command",
	})
	if err == nil {
		t.Error("Expected error for non-existent command")
	}

	// Get non-existent client
	_, ok := cm.GetClient("nonexistent")
	if ok {
		t.Error("Should not find non-existent client")
	}

	// Remove non-existent client
	err = cm.RemoveClient("nonexistent")
	if err == nil {
		t.Error("Expected error for removing non-existent client")
	}
}

// TestJSONRPCRequest tests JSON-RPC request marshaling.
func TestJSONRPCRequest(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		t.Errorf("Failed to marshal request: %v", err)
	}

	// Check that required fields are present
	var unmarshaled JSONRPCRequest
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Errorf("Failed to unmarshal request: %v", err)
	}

	if unmarshaled.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got '%s'", unmarshaled.JSONRPC)
	}

	if unmarshaled.Method != "test" {
		t.Errorf("Expected method 'test', got '%s'", unmarshaled.Method)
	}
}

// TestJSONRPCResponse tests JSON-RPC response parsing.
func TestJSONRPCResponse(t *testing.T) {
	// Success response
	successJSON := `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
	var successResp JSONRPCResponse
	if err := json.Unmarshal([]byte(successJSON), &successResp); err != nil {
		t.Errorf("Failed to parse success response: %v", err)
	}

	if successResp.Error != nil {
		t.Error("Success response should not have error")
	}

	// Error response
	errorJSON := `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found"}}`
	var errorResp JSONRPCResponse
	if err := json.Unmarshal([]byte(errorJSON), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Error == nil {
		t.Error("Error response should have error")
	}

	if errorResp.Error.Code != -32601 {
		t.Errorf("Expected error code -32601, got %d", errorResp.Error.Code)
	}
}

// TestToolResult tests tool result parsing.
func TestToolResult(t *testing.T) {
	resultJSON := `{"content":[{"type":"text","text":"Hello World"}],"isError":false}`
	var result ToolResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		t.Errorf("Failed to parse tool result: %v", err)
	}

	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(result.Content))
	}

	if result.Content[0].Type != "text" {
		t.Errorf("Expected content type 'text', got '%s'", result.Content[0].Type)
	}

	if result.Content[0].Text != "Hello World" {
		t.Errorf("Expected text 'Hello World', got '%s'", result.Content[0].Text)
	}
}

// TestCallToolParams tests tool call parameters.
func TestCallToolParams(t *testing.T) {
	params := CallToolParams{
		Name:      "test_tool",
		Arguments: json.RawMessage(`{"input":"test"}`),
	}

	bytes, err := json.Marshal(params)
	if err != nil {
		t.Errorf("Failed to marshal params: %v", err)
	}

	var unmarshaled CallToolParams
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Errorf("Failed to unmarshal params: %v", err)
	}

	if unmarshaled.Name != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", unmarshaled.Name)
	}
}

// TestReadResourceParams tests resource read parameters.
func TestReadResourceParams(t *testing.T) {
	params := ReadResourceParams{URI: "file:///test.txt"}

	bytes, err := json.Marshal(params)
	if err != nil {
		t.Errorf("Failed to marshal params: %v", err)
	}

	var unmarshaled ReadResourceParams
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Errorf("Failed to unmarshal params: %v", err)
	}

	if unmarshaled.URI != "file:///test.txt" {
		t.Errorf("Expected URI 'file:///test.txt', got '%s'", unmarshaled.URI)
	}
}

// TestGetPromptParams tests prompt get parameters.
func TestGetPromptParams(t *testing.T) {
	params := GetPromptParams{
		Name:      "test_prompt",
		Arguments: json.RawMessage(`{"topic":"testing"}`),
	}

	bytes, err := json.Marshal(params)
	if err != nil {
		t.Errorf("Failed to marshal params: %v", err)
	}

	var unmarshaled GetPromptParams
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Errorf("Failed to unmarshal params: %v", err)
	}

	if unmarshaled.Name != "test_prompt" {
		t.Errorf("Expected name 'test_prompt', got '%s'", unmarshaled.Name)
	}
}

// TestContentBlock tests content block parsing.
func TestContentBlock(t *testing.T) {
	textBlock := ContentBlock{
		Type: "text",
		Text: "Sample text",
	}

	bytes, _ := json.Marshal(textBlock)
	var unmarshaled ContentBlock
	json.Unmarshal(bytes, &unmarshaled)

	if unmarshaled.Type != "text" {
		t.Errorf("Expected type 'text', got '%s'", unmarshaled.Type)
	}

	// Data block
	dataBlock := ContentBlock{
		Type:     "resource",
		Data:     "base64data",
		MimeType: "application/octet-stream",
	}

	bytes, _ = json.Marshal(dataBlock)
	json.Unmarshal(bytes, &unmarshaled)

	if unmarshaled.MimeType != "application/octet-stream" {
		t.Errorf("Expected mimeType 'application/octet-stream', got '%s'", unmarshaled.MimeType)
	}
}

// TestIsConnected tests connection status.
func TestIsConnected(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	if client.IsConnected() {
		t.Error("Client should not be connected initially")
	}

	// Manually set connected for testing
	client.mu.Lock()
	client.connected = true
	client.mu.Unlock()

	if !client.IsConnected() {
		t.Error("Client should be connected after setting")
	}
}

// TestGetServerInfo tests server info retrieval.
func TestGetServerInfo(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	// Set server info for testing
	client.mu.Lock()
	client.serverInfo = ServerInfo{Name: "test-server", Version: "1.0.0"}
	client.mu.Unlock()

	info := client.GetServerInfo()
	if info.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", info.Version)
	}
}

// TestGetTools tests tools retrieval.
func TestGetTools(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	// Set tools for testing
	client.mu.Lock()
	client.tools = []ToolDefinition{
		{Name: "tool1", Description: "First tool"},
		{Name: "tool2", Description: "Second tool"},
	}
	client.mu.Unlock()

	tools := client.GetTools()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	if tools[0].Name != "tool1" {
		t.Errorf("Expected first tool 'tool1', got '%s'", tools[0].Name)
	}
}

// TestGetResources tests resources retrieval.
func TestGetResources(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	// Set resources for testing
	client.mu.Lock()
	client.resources = []ResourceDefinition{
		{URI: "file:///test.txt", Name: "Test File"},
	}
	client.mu.Unlock()

	resources := client.GetResources()
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}

	if resources[0].URI != "file:///test.txt" {
		t.Errorf("Expected URI 'file:///test.txt', got '%s'", resources[0].URI)
	}
}

// TestGetPrompts tests prompts retrieval.
func TestGetPrompts(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	// Set prompts for testing
	client.mu.Lock()
	client.prompts = []PromptDefinition{
		{Name: "prompt1", Description: "First prompt"},
	}
	client.mu.Unlock()

	prompts := client.GetPrompts()
	if len(prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(prompts))
	}

	if prompts[0].Name != "prompt1" {
		t.Errorf("Expected prompt 'prompt1', got '%s'", prompts[0].Name)
	}
}

// TestClose tests client close.
func TestClose(t *testing.T) {
	client := NewClient(ClientConfig{Name: "test"})

	// Set connected
	client.mu.Lock()
	client.connected = true
	client.mu.Unlock()

	// Close should work even without actual connection
	err := client.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after close")
	}
}

// TestClientManagerGetAllTools tests getting tools from all clients.
func TestClientManagerGetAllTools(t *testing.T) {
	cm := NewClientManager()

	// Empty manager should return empty map
	tools := cm.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("Expected empty tools map, got %d", len(tools))
	}
}

// TestClientManagerCloseAll tests closing all clients.
func TestClientManagerCloseAll(t *testing.T) {
	cm := NewClientManager()

	// Should work even with no clients
	cm.CloseAll()

	// List should still be empty
	names := cm.ListClients()
	if len(names) != 0 {
		t.Errorf("Expected empty list after close, got %d", len(names))
	}
}