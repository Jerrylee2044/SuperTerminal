package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewEventBus(t *testing.T) {
	eb := NewEventBus(100)
	if eb == nil {
		t.Fatal("Expected EventBus to be created")
	}
	if eb.GetBufferSize() != 100 {
		t.Errorf("Expected buffer size 100, got %d", eb.GetBufferSize())
	}
}

func TestEventBusSubscribe(t *testing.T) {
	eb := NewEventBus(10)
	ch := eb.Subscribe(EventAPIStream)
	if ch == nil {
		t.Fatal("Expected channel to be created")
	}

	// Publish event
	event := NewEvent(EventAPIStream, "test data", SourceAPI)
	eb.Publish(event)

	// Receive event
	select {
	case received := <-ch:
		if received.Type != EventAPIStream {
			t.Errorf("Expected type %s, got %s", EventAPIStream, received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive event")
	}

	eb.Close()
}

func TestEventBusSubscribeAll(t *testing.T) {
	eb := NewEventBus(10)
	ch := eb.SubscribeAll()
	if ch == nil {
		t.Fatal("Expected channel to be created")
	}

	// Publish multiple events
	eb.Publish(NewEvent(EventAPIStream, "stream", SourceAPI))
	eb.Publish(NewEvent(EventToolStart, "tool", SourceTool))

	// Should receive both
	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
			count++
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Expected to receive all events")
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 events, got %d", count)
	}

	eb.Close()
}

func TestEventBusUnsubscribe(t *testing.T) {
	eb := NewEventBus(10)
	ch := eb.Subscribe(EventAPIStream)
	eb.Unsubscribe(EventAPIStream, ch)

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("Expected channel to be closed")
		}
	default:
		// Channel closed, good
	}
}

func TestNewEvent(t *testing.T) {
	event := NewEvent(EventUserInput, "hello", SourceTUI)
	if event.Type != EventUserInput {
		t.Errorf("Expected type %s, got %s", EventUserInput, event.Type)
	}
	if event.Source != SourceTUI {
		t.Errorf("Expected source %s, got %s", SourceTUI, event.Source)
	}
	if event.Timestamp.IsZero() {
		t.Fatal("Expected timestamp to be set")
	}
}

func TestNewSession(t *testing.T) {
	session := NewSession()
	if session == nil {
		t.Fatal("Expected session to be created")
	}
	if session.ID == "" {
		t.Fatal("Expected session ID to be set")
	}
}

func TestSessionAddMessage(t *testing.T) {
	session := NewSession()
	msg := Message{
		Role:    RoleUser,
		Content: "Hello",
	}
	session.AddMessage(msg)

	if session.MessageCount() != 1 {
		t.Errorf("Expected 1 message, got %d", session.MessageCount())
	}

	msgs := session.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("Expected role %s, got %s", RoleUser, msgs[0].Role)
	}
}

func TestSessionClear(t *testing.T) {
	session := NewSession()
	session.AddMessage(Message{Role: RoleUser, Content: "test"})
	session.Clear()

	if session.MessageCount() != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", session.MessageCount())
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config == nil {
		t.Fatal("Expected config to be created")
	}
	if config.Model == "" {
		t.Fatal("Expected model to be set")
	}
	if config.MaxTokens <= 0 {
		t.Fatal("Expected max tokens to be positive")
	}
}

func TestNewEngine(t *testing.T) {
	config := DefaultConfig()
	e := NewEngine(EngineOptions{
		Config:     config,
		BufferSize: 50,
	})

	if e == nil {
		t.Fatal("Expected engine to be created")
	}
	if e.GetEventBus() == nil {
		t.Fatal("Expected event bus to be set")
	}
	if e.GetSession() == nil {
		t.Fatal("Expected session to be set")
	}
	if e.GetStatus() != StatusIdle {
		t.Errorf("Expected status %s, got %s", StatusIdle, e.GetStatus())
	}

	e.Shutdown()
}

func TestEngineCostTracking(t *testing.T) {
	config := DefaultConfig()
	e := NewEngine(EngineOptions{Config: config})

	// Initial cost should be zero
	cost := e.GetCost()
	if cost.TotalCost != 0 {
		t.Errorf("Expected initial cost 0, got %f", cost.TotalCost)
	}

	e.Shutdown()
}

func TestToolManager(t *testing.T) {
	tm := NewToolManager()
	if tm == nil {
		t.Fatal("Expected tool manager to be created")
	}

	// Register a tool
	tm.RegisterTool("test", &BashTool{})

	tool, ok := tm.GetTool("test")
	if !ok {
		t.Fatal("Expected tool to be found")
	}
	if tool.Name() != "bash" {
		t.Errorf("Expected tool name 'bash', got '%s'", tool.Name())
	}

	// List tools
	tools := tm.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
}

// TestGetHelpText tests help text content.
func TestGetHelpText(t *testing.T) {
	help := GetHelpText()
	
	// Check that help contains expected sections
	checks := []string{
		"/help",
		"/clear",
		"/save",
		"/load",
		"/sessions",
		"/export",
		"/model",
		"/cost",
		"/status",
		"bash",
		"web_search",
		"web_fetch",
	}
	
	for _, check := range checks {
		if !strings.Contains(help, check) {
			t.Errorf("Help text should contain '%s'", check)
		}
	}
}

// TestExportSession tests session export functionality.
func TestExportSession(t *testing.T) {
	// Create a simple engine config
	config := DefaultConfig()
	config.APIKey = "test-key"
	config.DataDir = t.TempDir()
	
	// Create engine
	engine := NewEngine(EngineOptions{
		Config: config,
	})
	
	// Add some messages
	engine.session.AddMessage(Message{
		Role:    RoleUser,
		Content: "Hello",
		Time:    time.Now(),
	})
	engine.session.AddMessage(Message{
		Role:    RoleAssistant,
		Content: "Hi there!",
		Time:    time.Now(),
	})
	
	// Test export formats
	exports := []string{"text", "json", "markdown", "md"}
	for _, format := range exports {
		result := engine.exportSession(format)
		if result == "" {
			t.Errorf("Export format '%s' should produce output", format)
		}
		if !strings.Contains(result, "Hello") {
			t.Errorf("Export format '%s' should contain user message", format)
		}
	}
}

// TestAutoSave tests auto-save functionality.
func TestAutoSave(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	
	// Create config with auto-save enabled
	config := DefaultConfig()
	config.APIKey = "test-key"
	config.DataDir = tempDir
	config.AutoSave = true
	
	// Create engine with persistence
	engine := NewEngine(EngineOptions{
		Config: config,
	})
	
	// Verify auto-save is enabled
	if !engine.AutoSaveEnabled() {
		t.Error("Auto-save should be enabled")
	}
	
	// Add a message
	engine.session.AddMessage(Message{
		Role:    RoleUser,
		Content: "Test message",
		Time:    time.Now(),
	})
	
	// Trigger auto-save
	engine.autoSaveSession("Test Title")
	
	// List sessions to verify save
	sessions, err := engine.ListSessions()
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	
	if len(sessions) == 0 {
		t.Error("Session should be saved")
	}
}