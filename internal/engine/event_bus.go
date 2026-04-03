// Package engine provides the core engine for SuperTerminal.
// It handles API communication, tool execution, session management,
// and event distribution to multiple UI frontends.
package engine

import (
	"sync"
	"time"
)

// EventType represents the type of event in the system.
type EventType string

const (
	// User interaction events
	EventUserInput    EventType = "user_input"     // User sent a message
	EventUserCommand  EventType = "user_command"   // User executed a slash command

	// API events
	EventAPIStart     EventType = "api_start"      // API request started
	EventAPIStream    EventType = "api_stream"     // Streaming response chunk
	EventAPIComplete  EventType = "api_complete"   // API request completed
	EventAPIError     EventType = "api_error"      // API request failed

	// Tool events
	EventToolStart    EventType = "tool_start"     // Tool execution started
	EventToolProgress EventType = "tool_progress"  // Tool execution progress
	EventToolOutput   EventType = "tool_output"    // Tool produced output
	EventToolComplete EventType = "tool_complete"  // Tool execution completed
	EventToolError    EventType = "tool_error"     // Tool execution failed

	// System events
	EventExit        EventType = "exit"            // Exit requested
	EventCostUpdate   EventType = "cost_update"    // Cost tracking update
	EventSessionSave  EventType = "session_save"   // Session saved
	EventSessionLoad  EventType = "session_load"   // Session load request
	EventSessionList  EventType = "session_list"   // Session list result
	EventSessionLoaded EventType = "session_loaded" // Session loaded from disk
	EventSearchResult EventType = "search_result"  // Search results
	EventPermissionRequest EventType = "permission_request" // Tool permission request
	EventConfigChange EventType = "config_change"  // Configuration changed
	EventError        EventType = "error"          // General error
	EventStatusChange EventType = "status_change"  // Engine status changed
)

// Source indicates where the event originated.
type Source string

const (
	SourceTUI    Source = "tui"    // Bubble Tea terminal UI
	SourceWeb    Source = "web"    // Web UI
	SourceEngine Source = "engine" // Engine itself
	SourceTool   Source = "tool"   // Tool execution
	SourceAPI    Source = "api"    // API client
)

// Event represents a single event in the system.
type Event struct {
	Type      EventType    `json:"type"`
	Data      interface{}  `json:"data"`
	Timestamp time.Time    `json:"timestamp"`
	Source    Source       `json:"source"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewEvent creates a new event with the current timestamp.
func NewEvent(typ EventType, data interface{}, source Source) Event {
	return Event{
		Type:      typ,
		Data:      data,
		Timestamp: time.Now(),
		Source:    source,
	}
}

// EventBus provides a publish-subscribe mechanism for event distribution.
// It allows multiple UI frontends (TUI, Web) to receive the same events
// from the engine core.
type EventBus struct {
	subscribers map[EventType][]chan Event
	allChannels []chan Event // Channels that receive all events
	mu          sync.RWMutex
	bufferSize  int
}

// NewEventBus creates a new EventBus with the specified buffer size.
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &EventBus{
		subscribers: make(map[EventType][]chan Event),
		allChannels: make([]chan Event, 0),
		bufferSize:  bufferSize,
	}
}

// Subscribe creates a channel that receives events of the specified type.
// Returns a channel that should be read continuously to prevent blocking.
func (eb *EventBus) Subscribe(eventType EventType) chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, eb.bufferSize)
	eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
	return ch
}

// SubscribeAll creates a channel that receives all events.
// Useful for logging, debugging, or UI updates.
func (eb *EventBus) SubscribeAll() chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, eb.bufferSize)
	eb.allChannels = append(eb.allChannels, ch)
	return ch
}

// GetBufferSize returns the configured buffer size.
func (eb *EventBus) GetBufferSize() int {
	return eb.bufferSize
}

// Publish sends an event to all subscribers of that event type.
// Non-blocking: if a channel is full, the event is dropped for that subscriber.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Send to type-specific subscribers
	if chans, ok := eb.subscribers[event.Type]; ok {
		for _, ch := range chans {
			select {
			case ch <- event:
			default:
				// Channel full, drop event for this subscriber
				// This prevents blocking the publisher
			}
		}
	}

	// Send to all-event subscribers
	for _, ch := range eb.allChannels {
		select {
		case ch <- event:
		default:
			// Channel full, drop event
		}
	}
}

// PublishSync sends an event and waits for all subscribers to receive it.
// Use sparingly, only when event delivery is critical.
func (eb *EventBus) PublishSync(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if chans, ok := eb.subscribers[event.Type]; ok {
		for _, ch := range chans {
			ch <- event // Blocks until received
		}
	}

	for _, ch := range eb.allChannels {
		ch <- event
	}
}

// Unsubscribe removes a channel from the subscribers list.
// Always call this when a subscriber is shutting down.
func (eb *EventBus) Unsubscribe(eventType EventType, ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if chans, ok := eb.subscribers[eventType]; ok {
		for i, c := range chans {
			if c == ch {
				eb.subscribers[eventType] = append(chans[:i], chans[i+1:]...)
				close(ch)
				break
			}
		}
	}
}

// UnsubscribeAll removes a channel from the all-events list.
func (eb *EventBus) UnsubscribeAll(ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for i, c := range eb.allChannels {
		if c == ch {
			eb.allChannels = append(eb.allChannels[:i], eb.allChannels[i+1:]...)
			close(ch)
			break
		}
	}
}

// Close shuts down the event bus, closing all subscriber channels.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType, chans := range eb.subscribers {
		for _, ch := range chans {
			close(ch)
		}
		eb.subscribers[eventType] = nil
	}

	for _, ch := range eb.allChannels {
		close(ch)
	}
	eb.allChannels = nil
}