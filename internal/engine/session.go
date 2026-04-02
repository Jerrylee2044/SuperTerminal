package engine

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// MessageRole represents the role of a message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// Message represents a single message in the conversation.
type Message struct {
	ID        string            `json:"id"`
	Role      MessageRole       `json:"role"`
	Content   string            `json:"content"`
	Thinking  string            `json:"thinking,omitempty"`
	Time      time.Time         `json:"time"`
	ToolName  string            `json:"tool_name,omitempty"`
	ToolUseID string            `json:"tool_use_id,omitempty"`
	ToolResult string           `json:"tool_result,omitempty"`
	ToolCalls []ToolCall        `json:"tool_calls,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Session represents a conversation session.
type Session struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Messages  []Message
	mu        sync.RWMutex
}

// NewSession creates a new session with a unique ID.
func NewSession() *Session {
	return &Session{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{},
	}
}

// AddMessage adds a message to the session.
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Time.IsZero() {
		msg.Time = time.Now()
	}

	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetMessages returns all messages in the session.
func (s *Session) GetMessages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Message, len(s.Messages))
	copy(result, s.Messages)
	return result
}

// GetMessagesForAPI formats messages for the Claude API.
func (s *Session) GetMessagesForAPI() []APIMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []APIMessage{}
	for _, msg := range s.Messages {
		// Skip empty messages
		if msg.Content == "" && len(msg.ToolCalls) == 0 && msg.ToolResult == "" {
			continue
		}

		apiMsg := APIMessage{
			Role: string(msg.Role),
		}

		// Handle different message types
		if msg.ToolResult != "" {
			// Tool result message
			apiMsg.Content = []ContentBlock{
				{
					Type:       "tool_result",
					ToolUseID:  msg.ToolUseID,
					Text:       msg.ToolResult,
				},
			}
		} else if len(msg.ToolCalls) > 0 {
			// Assistant message with tool calls
			blocks := []ContentBlock{}
			if msg.Content != "" {
				blocks = append(blocks, ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, ContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: []byte(tc.Input),
				})
			}
			apiMsg.Content = blocks
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

// MessageCount returns the number of messages in the session.
func (s *Session) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Messages)
}

// Clear removes all messages from the session.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = []Message{}
	s.UpdatedAt = time.Now()
}

// GetInfo returns session summary information.
func (s *Session) GetInfo() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SessionInfo{
		ID:          s.ID,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		MessageCount: len(s.Messages),
	}
}

// SessionInfo contains summary session information.
type SessionInfo struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

// GetLastMessage returns the most recent message.
func (s *Session) GetLastMessage() *Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Messages) == 0 {
		return nil
	}
	return &s.Messages[len(s.Messages)-1]
}

// GetMessagesSince returns messages after a specific time.
func (s *Session) GetMessagesSince(t time.Time) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []Message{}
	for _, msg := range s.Messages {
		if msg.Time.After(t) {
			result = append(result, msg)
		}
	}
	return result
}