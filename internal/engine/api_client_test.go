package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAPIClientCostCalculation tests the cost calculation logic.
func TestAPIClientCostCalculation(t *testing.T) {
	config := DefaultConfig()
	client := NewAPIClient(config)

	tests := []struct {
		name      string
		model     string
		usage     APIUsage
		minCost   float64
		maxCost   float64
	}{
		{
			name:  "sonnet basic usage",
			model: "claude-sonnet-4-20250514",
			usage: APIUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			minCost: 0.01,
			maxCost: 0.02,
		},
		{
			name:  "sonnet with cache",
			model: "claude-sonnet-4-20250514",
			usage: APIUsage{
				InputTokens:      1000,
				OutputTokens:     500,
				CacheReadTokens:  500,
				CacheWriteTokens: 200,
			},
			minCost: 0.01,
			maxCost: 0.02,
		},
		{
			name:  "opus expensive model",
			model: "claude-opus-4-20250514",
			usage: APIUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			minCost: 0.05,
			maxCost: 0.07,
		},
		{
			name:  "unknown model uses default pricing",
			model: "unknown-model",
			usage: APIUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			minCost: 0.01,
			maxCost: 0.02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := client.calculateCost(tt.model, tt.usage)
			if cost < tt.minCost || cost > tt.maxCost {
				t.Errorf("Cost %f not in expected range [%f, %f]", cost, tt.minCost, tt.maxCost)
			}
		})
	}
}

// TestAPIClientStream tests the streaming API with a mock server.
func TestAPIClientStream(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Header.Get("x-api-key") == "" {
			t.Error("Missing API key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("Missing version header")
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send mock SSE events
		flusher, _ := w.(http.Flusher)

		// Message start
		writeSSEEvent(w, "message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    "msg_test123",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-sonnet-4-20250514",
				"usage": map[string]int{
					"input_tokens": 10,
				},
			},
		})
		flusher.Flush()

		// Text content
		writeSSEEvent(w, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{
				"type": "text_delta",
				"text": "Hello",
			},
		})
		flusher.Flush()

		// Message delta with usage
		writeSSEEvent(w, "message_delta", map[string]interface{}{
			"type": "message_delta",
			"usage": map[string]int{
				"output_tokens": 5,
			},
		})
		flusher.Flush()

		// Message stop
		writeSSEEvent(w, "message_stop", map[string]interface{}{
			"type": "message_stop",
		})
		flusher.Flush()
	}))
	defer server.Close()

	// Create client with mock server URL
	config := DefaultConfig()
	config.APIKey = "test-key"
	client := NewAPIClient(config)
	client.baseURL = server.URL

	// Make streaming request
	req := APIRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []APIMessage{
			{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	respCh, err := client.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	// Collect responses
	var responses []StreamResponse
	for resp := range respCh {
		responses = append(responses, resp)
	}

	// Verify responses
	if len(responses) == 0 {
		t.Fatal("No responses received")
	}

	// Check for text response
	hasText := false
	for _, resp := range responses {
		if resp.Type == "text" && resp.Text == "Hello" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("Expected text response 'Hello'")
	}

	// Check for done
	hasDone := false
	for _, resp := range responses {
		if resp.Type == "done" {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("Expected done response")
	}
}

// TestAPIClientErrorHandling tests error handling.
func TestAPIClientErrorHandling(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"type": "authentication_error", "message": "Invalid API key"}}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.APIKey = "invalid-key"
	client := NewAPIClient(config)
	client.baseURL = server.URL

	req := APIRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []APIMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}}},
	}

	_, err := client.Stream(context.Background(), req)
	if err == nil {
		t.Error("Expected error for unauthorized request")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Errorf("Expected APIError, got %T", err)
		return
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", apiErr.StatusCode)
	}
}

// TestAPIClientNonStreaming tests non-streaming requests.
func TestAPIClientNonStreaming(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify stream=false
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if stream, ok := req["stream"].(bool); ok && stream {
			t.Error("Expected stream=false")
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_test123",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": [{"type": "text", "text": "Hello, world!"}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.APIKey = "test-key"
	client := NewAPIClient(config)
	client.baseURL = server.URL

	req := APIRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []APIMessage{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Hello"}}}},
	}

	resp, err := client.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.ID != "msg_test123" {
		t.Errorf("Expected ID 'msg_test123', got '%s'", resp.ID)
	}
	if len(resp.Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello, world!" {
		t.Errorf("Expected text 'Hello, world!', got '%s'", resp.Content[0].Text)
	}
}

// TestAPIError tests APIError type.
func TestAPIError(t *testing.T) {
	err := &APIError{
		StatusCode: 429,
		Type:       "rate_limit_error",
		Message:    "Too many requests",
	}

	if !err.IsRetryable() {
		t.Error("Rate limit error should be retryable")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "429") {
		t.Error("Error message should contain status code")
	}
	if !strings.Contains(errMsg, "rate_limit_error") {
		t.Error("Error message should contain error type")
	}
}

// TestBuildMessagesForAPI tests message building.
func TestBuildMessagesForAPI(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
		{Role: RoleUser, Content: "How are you?"},
	}

	apiMessages := BuildMessagesForAPI(messages)

	if len(apiMessages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(apiMessages))
	}

	if apiMessages[0].Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", apiMessages[0].Role)
	}
	if len(apiMessages[0].Content) != 1 || apiMessages[0].Content[0].Text != "Hello" {
		t.Error("First message content incorrect")
	}
}

// TestBuildToolsForAPI tests tool building.
func TestBuildToolsForAPI(t *testing.T) {
	tools := []ToolDef{
		{
			Name:        "bash",
			Description: "Execute bash commands",
			InputSchema: map[string]interface{}{"type": "object"},
		},
	}

	apiTools := BuildToolsForAPI(tools)

	if len(apiTools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(apiTools))
	}

	if apiTools[0].Name != "bash" {
		t.Errorf("Expected name 'bash', got '%s'", apiTools[0].Name)
	}
}

// Helper function to write SSE events
func writeSSEEvent(w http.ResponseWriter, eventType string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	w.Write([]byte("event: " + eventType + "\n"))
	w.Write([]byte("data: " + string(jsonData) + "\n\n"))
}