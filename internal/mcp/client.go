// Package mcp provides MCP client for connecting to external MCP servers.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ClientConfig configures an MCP client connection.
type ClientConfig struct {
	Name    string            `json:"name"`    // Server name for identification
	Type    string            `json:"type"`    // stdio, sse, http
	Command string            `json:"command"` // For stdio type
	Args    []string          `json:"args"`    // For stdio type
	Env     map[string]string `json:"env"`     // Environment variables
	URL     string            `json:"url"`     // For sse/http type
	Headers map[string]string `json:"headers"` // HTTP headers
}

// Client connects to an external MCP server.
type Client struct {
	config     ClientConfig
	serverInfo ServerInfo
	tools      []ToolDefinition
	resources  []ResourceDefinition
	prompts    []PromptDefinition

	// For stdio transport
	cmd    *exec.Cmd
	stdin  io.Writer
	stdout io.Reader
	stderr io.Reader

	// For HTTP/SSE transport
	httpClient *http.Client

	// Connection state
	connected bool
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc

	// Request tracking
	requestID int
	idMu      sync.Mutex
}

// NewClient creates a new MCP client.
func NewClient(config ClientConfig) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Connect establishes connection to the MCP server.
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.config.Type {
	case "stdio":
		return c.connectStdio()
	case "sse":
		return c.connectSSE()
	case "http":
		return c.connectHTTP()
	default:
		return fmt.Errorf("unsupported transport type: %s", c.config.Type)
	}
}

// connectStdio starts a subprocess and communicates via stdin/stdout.
func (c *Client) connectStdio() error {
	cmd := exec.CommandContext(c.ctx, c.config.Command, c.config.Args...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range c.config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Get pipes
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdinPipe
	c.stdout = stdoutPipe
	c.stderr = stderrPipe

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Send initialize request
	initResult, err := c.sendInitialize()
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("initialize failed: %w", err)
	}

	c.serverInfo = initResult.ServerInfo
	c.connected = true

	// Load tools, resources, prompts
	c.loadCapabilities()

	return nil
}

// connectHTTP establishes HTTP connection.
func (c *Client) connectHTTP() error {
	// Send initialize via HTTP
	initResult, err := c.httpInitialize()
	if err != nil {
		return fmt.Errorf("HTTP initialize failed: %w", err)
	}

	c.serverInfo = initResult.ServerInfo
	c.connected = true
	c.loadCapabilitiesHTTP()

	return nil
}

// connectSSE establishes SSE connection.
func (c *Client) connectSSE() error {
	// SSE uses HTTP for requests, SSE for server-to-client events
	return c.connectHTTP()
}

// sendInitialize sends initialize request via stdio.
func (c *Client) sendInitialize() (*InitializeResult, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{},"resources":{},"prompts":{}}}`),
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	var result InitializeResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	return &result, nil
}

// httpInitialize sends initialize request via HTTP.
func (c *Client) httpInitialize() (*InitializeResult, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{},"resources":{},"prompts":{}}}`),
	}

	resp, err := c.httpSendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	var result InitializeResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	return &result, nil
}

// loadCapabilities loads tools, resources, and prompts via stdio.
func (c *Client) loadCapabilities() {
	// Load tools
	tools, err := c.listTools()
	if err == nil {
		c.tools = tools
	}

	// Load resources
	resources, err := c.listResources()
	if err == nil {
		c.resources = resources
	}

	// Load prompts
	prompts, err := c.listPrompts()
	if err == nil {
		c.prompts = prompts
	}
}

// loadCapabilitiesHTTP loads via HTTP.
func (c *Client) loadCapabilitiesHTTP() {
	tools, err := c.httpListTools()
	if err == nil {
		c.tools = tools
	}

	resources, err := c.httpListResources()
	if err == nil {
		c.resources = resources
	}

	prompts, err := c.httpListPrompts()
	if err == nil {
		c.prompts = prompts
	}
}

// nextID generates a unique request ID.
func (c *Client) nextID() int {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	c.requestID++
	return c.requestID
}

// sendRequest sends a JSON-RPC request via stdio.
func (c *Client) sendRequest(req JSONRPCRequest) (*JSONRPCResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Write request (with newline for stdio protocol)
	if _, err := c.stdin.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(c.stdout)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read response: no data")
	}

	respBytes := scanner.Bytes()
	var resp JSONRPCResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// httpSendRequest sends a JSON-RPC request via HTTP.
func (c *Client) httpSendRequest(req JSONRPCRequest) (*JSONRPCResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(c.ctx, "POST", c.config.URL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.config.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", httpResp.StatusCode)
	}

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse HTTP response: %w", err)
	}

	return &resp, nil
}

// listTools retrieves tool list via stdio.
func (c *Client) listTools() ([]ToolDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/list",
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	var result ListToolsResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, err
	}

	return result.Tools, nil
}

// httpListTools retrieves tool list via HTTP.
func (c *Client) httpListTools() ([]ToolDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/list",
	}

	resp, err := c.httpSendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	var result ListToolsResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &result)

	return result.Tools, nil
}

// listResources retrieves resource list via stdio.
func (c *Client) listResources() ([]ResourceDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "resources/list",
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("resources/list error: %s", resp.Error.Message)
	}

	var result ListResourcesResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, err
	}

	return result.Resources, nil
}

// httpListResources retrieves resource list via HTTP.
func (c *Client) httpListResources() ([]ResourceDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "resources/list",
	}

	resp, err := c.httpSendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("resources/list error: %s", resp.Error.Message)
	}

	var result ListResourcesResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &result)

	return result.Resources, nil
}

// listPrompts retrieves prompt list via stdio.
func (c *Client) listPrompts() ([]PromptDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "prompts/list",
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("prompts/list error: %s", resp.Error.Message)
	}

	var result ListPromptsResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, err
	}

	return result.Prompts, nil
}

// httpListPrompts retrieves prompt list via HTTP.
func (c *Client) httpListPrompts() ([]PromptDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "prompts/list",
	}

	resp, err := c.httpSendRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("prompts/list error: %s", resp.Error.Message)
	}

	var result ListPromptsResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &result)

	return result.Prompts, nil
}

// CallTool calls a tool on the MCP server.
func (c *Client) CallTool(name string, args json.RawMessage) (*ToolResult, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()

	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}
	paramsBytes, _ := json.Marshal(params)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/call",
		Params:  paramsBytes,
	}

	var resp *JSONRPCResponse
	var err error

	if c.config.Type == "stdio" {
		resp, err = c.sendRequest(req)
	} else {
		resp, err = c.httpSendRequest(req)
	}

	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tool call error: %s", resp.Error.Message)
	}

	var result ToolResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &result)

	return &result, nil
}

// ReadResource reads a resource from the MCP server.
func (c *Client) ReadResource(uri string) ([]ContentBlock, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()

	params := ReadResourceParams{URI: uri}
	paramsBytes, _ := json.Marshal(params)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "resources/read",
		Params:  paramsBytes,
	}

	var resp *JSONRPCResponse
	var err error

	if c.config.Type == "stdio" {
		resp, err = c.sendRequest(req)
	} else {
		resp, err = c.httpSendRequest(req)
	}

	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("resource read error: %s", resp.Error.Message)
	}

	// Parse contents from the result
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid resource result format")
	}

	contents, ok := resultMap["contents"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no contents in resource result")
	}

	var blocks []ContentBlock
	for _, content := range contents {
		contentBytes, _ := json.Marshal(content)
		var block ContentBlock
		json.Unmarshal(contentBytes, &block)
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// GetPrompt gets a prompt from the MCP server.
func (c *Client) GetPrompt(name string, args json.RawMessage) (string, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return "", fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()

	params := GetPromptParams{
		Name:      name,
		Arguments: args,
	}
	paramsBytes, _ := json.Marshal(params)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "prompts/get",
		Params:  paramsBytes,
	}

	var resp *JSONRPCResponse
	var err error

	if c.config.Type == "stdio" {
		resp, err = c.sendRequest(req)
	} else {
		resp, err = c.httpSendRequest(req)
	}

	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("prompt get error: %s", resp.Error.Message)
	}

	// Extract text from the messages
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid prompt result format")
	}

	messages, ok := resultMap["messages"].([]interface{})
	if !ok {
		return "", fmt.Errorf("no messages in prompt result")
	}

	var result strings.Builder
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := msgMap["content"].(map[string]interface{})
		if !ok {
			continue
		}
		text, ok := content["text"].(string)
		if ok {
			result.WriteString(text)
		}
	}

	return result.String(), nil
}

// GetTools returns available tools.
func (c *Client) GetTools() []ToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

// GetResources returns available resources.
func (c *Client) GetResources() []ResourceDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resources
}

// GetPrompts returns available prompts.
func (c *Client) GetPrompts() []PromptDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.prompts
}

// GetServerInfo returns server info.
func (c *Client) GetServerInfo() ServerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverInfo
}

// IsConnected returns connection status.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close closes the connection.
func (c *Client) Close() error {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		// Send shutdown notification
		req := JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "shutdown",
		}
		reqBytes, _ := json.Marshal(req)
		c.stdin.Write(append(reqBytes, '\n'))

		// Wait for process to exit
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()

		select {
		case <-time.After(5 * time.Second):
			c.cmd.Process.Kill()
		case <-done:
		}
	}

	c.connected = false
	return nil
}

// ClientManager manages multiple MCP clients.
type ClientManager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewClientManager creates a new client manager.
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*Client),
	}
}

// AddClient adds a new MCP client.
func (cm *ClientManager) AddClient(name string, config ClientConfig) (*Client, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.clients[name]; exists {
		return nil, fmt.Errorf("client %s already exists", name)
	}

	client := NewClient(config)
	if err := client.Connect(); err != nil {
		return nil, err
	}

	cm.clients[name] = client
	return client, nil
}

// GetClient gets a client by name.
func (cm *ClientManager) GetClient(name string) (*Client, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	client, ok := cm.clients[name]
	return client, ok
}

// RemoveClient removes and closes a client.
func (cm *ClientManager) RemoveClient(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	client, ok := cm.clients[name]
	if !ok {
		return fmt.Errorf("client %s not found", name)
	}

	client.Close()
	delete(cm.clients, name)
	return nil
}

// ListClients returns all client names.
func (cm *ClientManager) ListClients() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	names := make([]string, 0, len(cm.clients))
	for name := range cm.clients {
		names = append(names, name)
	}
	return names
}

// GetAllTools returns tools from all clients.
func (cm *ClientManager) GetAllTools() map[string][]ToolDefinition {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string][]ToolDefinition)
	for name, client := range cm.clients {
		result[name] = client.GetTools()
	}
	return result
}

// CloseAll closes all clients.
func (cm *ClientManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, client := range cm.clients {
		client.Close()
	}
	cm.clients = make(map[string]*Client)
}