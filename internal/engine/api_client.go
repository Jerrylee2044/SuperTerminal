// Package engine provides the core engine for SuperTerminal.
// api_client.go handles communication with the Anthropic Claude API.
package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// API Types
// ============================================================================

// APIRequest represents a request to the Claude API.
type APIRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []APIMessage `json:"messages"`
	System    string       `json:"system,omitempty"`
	Tools     []APITool    `json:"tools,omitempty"`
	Stream    bool         `json:"stream,omitempty"`
}

// APIMessage represents a message in the conversation.
type APIMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// For tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// For thinking
	Thinking string `json:"thinking,omitempty"`
}

// APITool represents a tool definition.
type APITool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ============================================================================
// Response Types
// ============================================================================

// StreamEventType represents the type of SSE event.
type StreamEventType string

const (
	SSEEventMessageStart     StreamEventType = "message_start"
	SSEEventContentBlockStart StreamEventType = "content_block_start"
	SSEEventContentBlockDelta StreamEventType = "content_block_delta"
	SSEEventContentBlockStop  StreamEventType = "content_block_stop"
	SSEEventMessageDelta      StreamEventType = "message_delta"
	SSEEventMessageStop       StreamEventType = "message_stop"
	SSEEventError             StreamEventType = "error"
	SSEEventPing              StreamEventType = "ping"
)

// StreamEvent represents a SSE stream event.
type StreamEvent struct {
	Type  StreamEventType     `json:"type"`
	Index int                 `json:"index,omitempty"`
	Delta *ContentBlockDelta  `json:"delta,omitempty"`
	Message *MessageStartData `json:"message,omitempty"`
	Error  *ErrorData         `json:"error,omitempty"`
	Usage  *UsageDelta        `json:"usage,omitempty"`
}

// MessageStartData contains message start information.
type MessageStartData struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Role    string    `json:"role"`
	Model   string    `json:"model"`
	Usage   APIUsage  `json:"usage"`
}

// OpenAIStreamChoice represents a choice in OpenAI streaming response.
type OpenAIStreamChoice struct {
	Index        int                     `json:"index"`
	Delta        OpenAIDelta             `json:"delta"`
	FinishReason string                  `json:"finish_reason"`
}

// OpenAIDelta represents the delta in OpenAI streaming response.
type OpenAIDelta struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Role             string `json:"role"`
}

// OpenAIStreamEvent represents an OpenAI-style SSE stream event.
type OpenAIStreamEvent struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int                 `json:"created"`
	Model   string              `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ContentBlockDelta represents a delta in content.
type ContentBlockDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	
	// For thinking delta
	Thinking string `json:"thinking,omitempty"`
	
	// For tool use delta (partial JSON)
	PartialJSON string `json:"partial_json,omitempty"`
}

// UsageDelta represents usage information in a delta.
type UsageDelta struct {
	OutputTokens int64 `json:"output_tokens"`
}

// ErrorData represents an error from the API.
type ErrorData struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// APIUsage represents token usage from the API.
type APIUsage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_creation_input_tokens,omitempty"`
}

// ============================================================================
// API Client
// ============================================================================

// APIClient handles communication with the Claude API.
type APIClient struct {
	config      *Config
	httpClient  *http.Client
	apiKey      string
	baseURL     string
	modelPricing map[string]ModelPricing
	isDashScope bool
	mu          sync.RWMutex
}

// ModelPricing contains pricing information for a model.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheWritePerMillion float64
	CacheReadPerMillion  float64
}

// Default pricing (as of 2024)
var defaultPricing = map[string]ModelPricing{
	"claude-sonnet-4-20250514": {
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheWritePerMillion: 3.75,
		CacheReadPerMillion:  0.30,
	},
	"claude-opus-4-20250514": {
		InputPerMillion:      15.0,
		OutputPerMillion:     75.0,
		CacheWritePerMillion: 18.75,
		CacheReadPerMillion:  1.50,
	},
	"claude-3-5-sonnet-20241022": {
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheWritePerMillion: 3.75,
		CacheReadPerMillion:  0.30,
	},
	"claude-3-5-haiku-20241022": {
		InputPerMillion:      0.80,
		OutputPerMillion:     4.0,
		CacheWritePerMillion: 1.0,
		CacheReadPerMillion:  0.08,
	},
}

// NewAPIClient creates a new API client.
func NewAPIClient(config *Config) *APIClient {
	isDashScope := strings.Contains(config.BaseURL, "dashscope")
	
	client := &APIClient{
		config:       config,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
		apiKey:       config.APIKey,
		baseURL:      config.BaseURL,
		modelPricing: defaultPricing,
		isDashScope:  isDashScope,
	}
	
	// Add DashScope-specific pricing
	if isDashScope {
		client.modelPricing["qwen3.5-plus"] = ModelPricing{
			InputPerMillion:  0.8,
			OutputPerMillion: 4.0,
		}
		client.modelPricing["qwen3.5"] = ModelPricing{
			InputPerMillion:  0.6,
			OutputPerMillion: 3.0,
		}
		client.modelPricing["qwen2.5-coder-plus"] = ModelPricing{
			InputPerMillion:  0.8,
			OutputPerMillion: 4.0,
		}
	}
	
	return client
}

// ============================================================================
// Streaming API
// ============================================================================

// StreamResponse represents a processed response chunk for the engine.
type StreamResponse struct {
	Type        string          `json:"type"`        // "text", "thinking", "tool_use", "tool_result", "error", "done"
	Text        string          `json:"text,omitempty"`
	Thinking    string          `json:"thinking,omitempty"`
	ToolName    string          `json:"tool_name,omitempty"`
	ToolInput   json.RawMessage `json:"tool_input,omitempty"`
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	Usage       *APIUsage       `json:"usage,omitempty"`
	Cost        float64         `json:"cost,omitempty"`
	Error       error           `json:"error,omitempty"`
	MessageID   string          `json:"message_id,omitempty"`
	Model       string          `json:"model,omitempty"`
	StopReason  string          `json:"stop_reason,omitempty"`
}

// Convert request to DashScope (OpenAI-compatible) format
func convertToDashScopeRequest(req APIRequest) map[string]interface{} {
	messages := make([]map[string]interface{}, len(req.Messages))
	
	for i, msg := range req.Messages {
		// Convert content blocks to simple string
		var content string
		if len(msg.Content) > 0 {
			// Join all text content
			var texts []string
			for _, block := range msg.Content {
				if block.Text != "" {
					texts = append(texts, block.Text)
				}
			}
			content = strings.Join(texts, "\n")
		}
		
		messages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": content,
		}
	}
	
	result := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
		"stream":     req.Stream,
	}
	
	// Add system message if present
	if req.System != "" {
		result["system"] = req.System
	}
	
	// Note: Tools are not converted for now - DashScope may handle differently
	
	return result
}

// processOpenAIStreamEvent processes OpenAI/DashScope format streaming events.
func (c *APIClient) processOpenAIStreamEvent(event OpenAIStreamEvent, respCh chan<- StreamResponse, totalUsage *APIUsage, messageID *string, model string) {
	if len(event.Choices) == 0 {
		return
	}
	
	choice := event.Choices[0]
	delta := choice.Delta
	
	// Update message ID from first event
	if *messageID == "" {
		*messageID = event.ID
	}
	
	// Handle usage from first event
	if event.Usage.TotalTokens > 0 && totalUsage.InputTokens == 0 {
		totalUsage.InputTokens = int64(event.Usage.PromptTokens)
		totalUsage.OutputTokens = int64(event.Usage.CompletionTokens)
	}
	
	// Handle reasoning content (thinking)
	if delta.ReasoningContent != "" {
		respCh <- StreamResponse{
			Type:     "thinking",
			Thinking: delta.ReasoningContent,
		}
	}
	
	// Handle content
	if delta.Content != "" {
		respCh <- StreamResponse{
			Type: "text",
			Text: delta.Content,
		}
	}
	
	// Handle finish
	if choice.FinishReason != "" {
		respCh <- StreamResponse{
			Type:      "message_stop",
			MessageID: *messageID,
			StopReason: choice.FinishReason,
			Usage:     totalUsage,
			Cost:      c.calculateCost(model, *totalUsage),
		}
	}
}

// Stream sends a streaming request to the Claude API.
// Returns a channel that yields processed StreamResponse objects.
func (c *APIClient) Stream(ctx context.Context, req APIRequest) (<-chan StreamResponse, error) {
	req.Stream = true
	
	var body []byte
	var err error
	
	if c.isDashScope {
		// Convert to DashScope (OpenAI-compatible) format
		dashReq := convertToDashScopeRequest(req)
		body, err = json.Marshal(dashReq)
	} else {
		body, err = json.Marshal(req)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Determine endpoint based on provider
	endpoint := c.baseURL + "/v1/messages"
	if c.isDashScope {
		endpoint = c.baseURL + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	if c.isDashScope {
		// DashScope uses OpenAI-compatible API with Bearer token
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		// Anthropic API
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		// Add beta headers for features
		httpReq.Header.Set("anthropic-beta", "max-tokens-3-5-sonnet-2024-07-15")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Type:       "http_error",
			Message:    string(errorBody),
		}
	}

	// Create response channel
	respCh := make(chan StreamResponse, 100)

	// Process stream in background
	go c.processSSEStream(ctx, resp.Body, req.Model, respCh)

	return respCh, nil
}

// processSSEStream reads SSE data and converts to StreamResponse.
func (c *APIClient) processSSEStream(ctx context.Context, body io.ReadCloser, model string, respCh chan<- StreamResponse) {
	defer close(respCh)
	defer body.Close()

	reader := bufio.NewReader(body)
	
	// Accumulator for streaming state
	var (
		messageID    string
		currentBlock *ContentBlock
		blockIndex   int
		totalUsage   APIUsage
	)

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			respCh <- StreamResponse{Type: "error", Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				respCh <- StreamResponse{Type: "error", Error: err}
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// SSE format: "data: {...}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Send final response
			respCh <- StreamResponse{
				Type:      "done",
				MessageID: messageID,
				Usage:     &totalUsage,
				Cost:      c.calculateCost(model, totalUsage),
			}
			return
		}

		// Parse event
		// Check format: Anthropic has "type" field, OpenAI/DashScope has "choices" field
		var event StreamEvent
		var openaiEvent OpenAIStreamEvent
		
		// Try to detect format by checking for "choices" (OpenAI) or "type" (Anthropic)
		if strings.Contains(data, `"choices"`) {
			// OpenAI/DashScope format
			if err := json.Unmarshal([]byte(data), &openaiEvent); err != nil {
				continue
			}
			// Process OpenAI format
			c.processOpenAIStreamEvent(openaiEvent, respCh, &totalUsage, &messageID, model)
			continue
		}
		
		// Anthropic format
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Skip malformed events
			continue
		}

		// Handle event types
		switch event.Type {
		case SSEEventMessageStart:
			// Message started - contains initial usage
			if event.Message != nil {
				messageID = event.Message.ID
				totalUsage.InputTokens = event.Message.Usage.InputTokens
				totalUsage.CacheReadTokens = event.Message.Usage.CacheReadTokens
				totalUsage.CacheWriteTokens = event.Message.Usage.CacheWriteTokens
				
				respCh <- StreamResponse{
					Type:      "message_start",
					MessageID: messageID,
					Model:     event.Message.Model,
					Usage:     &totalUsage,
					Cost:      c.calculateCost(model, totalUsage),
				}
			}

		case SSEEventContentBlockStart:
			// New content block starting
			blockIndex = event.Index
			// We'll determine type from delta events
			currentBlock = &ContentBlock{}

		case SSEEventContentBlockDelta:
			// Delta to current block
			if event.Delta == nil {
				continue
			}

			switch event.Delta.Type {
			case "text_delta":
				// Text content
				respCh <- StreamResponse{
					Type: "text",
					Text: event.Delta.Text,
				}

			case "thinking_delta":
				// Thinking content
				respCh <- StreamResponse{
					Type:     "thinking",
					Thinking: event.Delta.Thinking,
				}

			case "input_json_delta":
				// Partial JSON for tool input - accumulate
				if currentBlock != nil {
					// We need to accumulate partial JSON
					// For now, send as raw partial
					respCh <- StreamResponse{
						Type:      "tool_input_delta",
						ToolUseID: fmt.Sprintf("tool_%d", blockIndex),
						Text:      event.Delta.PartialJSON, // Partial JSON string
					}
				}
			}

		case SSEEventContentBlockStop:
			// Content block finished
			if currentBlock != nil {
				currentBlock = nil
			}

		case SSEEventMessageDelta:
			// Message delta - usually contains output tokens
			if event.Usage != nil {
				totalUsage.OutputTokens = event.Usage.OutputTokens
				respCh <- StreamResponse{
					Type: "usage_update",
					Usage: &APIUsage{
						OutputTokens: event.Usage.OutputTokens,
					},
					Cost: c.calculateCost(model, totalUsage),
				}
			}

		case SSEEventMessageStop:
			// Message complete
			respCh <- StreamResponse{
				Type:      "done",
				MessageID: messageID,
				Usage:     &totalUsage,
				Cost:      c.calculateCost(model, totalUsage),
			}

		case SSEEventError:
			// Error from API
			if event.Error != nil {
				respCh <- StreamResponse{
					Type:  "error",
					Error: &APIError{
						Type:    event.Error.Type,
						Message: event.Error.Message,
					},
				}
			}

		case SSEEventPing:
			// Ignore ping events
		}
	}
}

// ============================================================================
// Non-Streaming API
// ============================================================================

// APIResponse represents a complete API response.
type APIResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	Usage        APIUsage       `json:"usage"`
}

// Send sends a non-streaming request and returns the full response.
func (c *APIClient) Send(ctx context.Context, req APIRequest) (*APIResponse, error) {
	req.Stream = false

	var body []byte
	var err error
	
	if c.isDashScope {
		// Convert to DashScope (OpenAI-compatible) format
		dashReq := convertToDashScopeRequest(req)
		body, err = json.Marshal(dashReq)
	} else {
		body, err = json.Marshal(req)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Determine endpoint based on provider
	endpoint := c.baseURL + "/v1/messages"
	if c.isDashScope {
		endpoint = c.baseURL + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	
	// Set headers based on provider
	if c.isDashScope {
		// DashScope uses OpenAI-compatible API with Bearer token
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		// Anthropic API
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Type:       "http_error",
			Message:    string(errorBody),
		}
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &apiResp, nil
}

// ============================================================================
// Cost Calculation
// ============================================================================

// calculateCost computes the cost based on token usage.
func (c *APIClient) calculateCost(model string, usage APIUsage) float64 {
	pricing, ok := c.modelPricing[model]
	if !ok {
		// Default pricing for unknown models
		pricing = ModelPricing{
			InputPerMillion:      3.0,
			OutputPerMillion:     15.0,
			CacheWritePerMillion: 3.75,
			CacheReadPerMillion:  0.30,
		}
	}

	cost := 0.0
	
	// Input tokens cost
	if usage.InputTokens > 0 {
		cost += float64(usage.InputTokens) * pricing.InputPerMillion / 1_000_000
	}
	
	// Output tokens cost
	if usage.OutputTokens > 0 {
		cost += float64(usage.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	}
	
	// Cache read tokens cost (much cheaper)
	if usage.CacheReadTokens > 0 {
		cost += float64(usage.CacheReadTokens) * pricing.CacheReadPerMillion / 1_000_000
	}
	
	// Cache write tokens cost
	if usage.CacheWriteTokens > 0 {
		cost += float64(usage.CacheWriteTokens) * pricing.CacheWritePerMillion / 1_000_000
	}

	return cost
}

// ============================================================================
// Error Types
// ============================================================================

// APIError represents an error from the API.
type APIError struct {
	StatusCode int    `json:"status_code,omitempty"`
	Type       string `json:"type"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("API error (%d): %s - %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("API error: %s - %s", e.Type, e.Message)
}

// IsRetryable returns true if the error might succeed on retry.
func (e *APIError) IsRetryable() bool {
	// Rate limit errors are retryable
	if e.StatusCode == 429 {
		return true
	}
	// Server errors might be retryable
	if e.StatusCode >= 500 && e.StatusCode < 600 {
		return true
	}
	// Overloaded error
	if e.Type == "overloaded" {
		return true
	}
	return false
}

// ============================================================================
// Configuration
// ============================================================================

// SetAPIKey updates the API key.
func (c *APIClient) SetAPIKey(key string) {
	c.mu.Lock()
	c.apiKey = key
	c.mu.Unlock()
}

// SetBaseURL updates the base URL.
func (c *APIClient) SetBaseURL(url string) {
	c.mu.Lock()
	c.baseURL = url
	c.mu.Unlock()
}

// SetModelPricing updates pricing for a model.
func (c *APIClient) SetModelPricing(model string, pricing ModelPricing) {
	c.mu.Lock()
	c.modelPricing[model] = pricing
	c.mu.Unlock()
}

// ============================================================================
// Helper Functions
// ============================================================================

// BuildMessagesForAPI converts session messages to API format.
func BuildMessagesForAPI(messages []Message) []APIMessage {
	result := []APIMessage{}
	
	for _, msg := range messages {
		apiMsg := APIMessage{
			Role: string(msg.Role),
		}
		
		// Convert content to blocks
		if msg.ToolName != "" && msg.ToolResult != "" {
			// Tool result message
			apiMsg.Content = []ContentBlock{
				{
					Type:       "tool_result",
					ToolUseID:  msg.ToolUseID,
					Content:    json.RawMessage(fmt.Sprintf(`"%s"`, msg.ToolResult)),
					IsError:    false,
				},
			}
		} else {
			// Regular text message
			apiMsg.Content = []ContentBlock{
				{
					Type: "text",
					Text: msg.Content,
				},
			}
		}
		
		result = append(result, apiMsg)
	}
	
	return result
}

// BuildToolsForAPI converts tool definitions to API format.
func BuildToolsForAPI(tools []ToolDef) []APITool {
	result := []APITool{}
	
	for _, tool := range tools {
		result = append(result, APITool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	
	return result
}