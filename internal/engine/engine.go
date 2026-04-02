package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"superterminal/internal/logger"
	"superterminal/internal/mcp"
	"superterminal/internal/persistence"
	"superterminal/internal/security"
)

// EngineStatus represents the current state of the engine.
type EngineStatus string

const (
	StatusIdle       EngineStatus = "idle"       // Waiting for user input
	StatusThinking   EngineStatus = "thinking"   // Processing API request
	StatusToolExec   EngineStatus = "tool_exec"  // Executing a tool
	StatusError      EngineStatus = "error"      // Error state
	StatusShuttingDown EngineStatus = "shutting_down" // Engine is shutting down
)

// Engine is the core of SuperTerminal.
// It manages sessions, handles API communication, executes tools,
// and distributes events to UI frontends via the EventBus.
type Engine struct {
	// Core components
	eventBus    *EventBus
	session     *Session
	apiClient   *APIClient
	toolManager *ToolManager
	config      *Config

	// New components
	logger           *logger.Logger
	permManager      *security.PermissionManager
	sessionManager   *persistence.SessionManager
	secureStorage    *security.SecureStorage
	mcpManager       *mcp.ClientManager // MCP client manager

	// State
	status     EngineStatus
	statusMu   sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc

	// Cost tracking
	totalCost      float64
	inputTokens    int64
	outputTokens   int64
	cacheReadTokens int64
	cacheWriteTokens int64
	costMu         sync.RWMutex

	// Current operation
	currentToolUseID string
	currentToolName  string
	toolMu           sync.RWMutex
}

// EngineOptions provides configuration for creating a new Engine.
type EngineOptions struct {
	Config       *Config
	BufferSize   int
	DataDir      string // Directory for session storage
	EnableLog    bool   // Enable logging
	LogFile      string // Log file path
}

// NewEngine creates a new Engine instance.
func NewEngine(opts EngineOptions) *Engine {
	if opts.Config == nil {
		opts.Config = DefaultConfig()
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize logger
	var log *logger.Logger
	if opts.EnableLog {
		log = logger.NewLogger(logger.Options{
			Level: logger.LevelInfo,
			File:  opts.LogFile,
		})
	} else {
		log = logger.NewLogger(logger.Options{Level: logger.LevelInfo})
	}

	// Initialize permission manager
	permFile := ""
	if opts.DataDir != "" {
		permFile = opts.DataDir + "/permissions.json"
	}
	permManager := security.NewPermissionManager(permFile)

	// Initialize session manager
	sessionManager := persistence.NewSessionManager(persistence.SessionManagerOptions{
		DataDir:     opts.DataDir,
		MaxSessions: 100,
		AutoSave:    false,
	})

	// Initialize secure storage
	secureStorage, _ := security.NewSecureStorage(security.StorageOptions{
		PreferEnv: true,
	})

	e := &Engine{
		eventBus:       NewEventBus(opts.BufferSize),
		session:        NewSession(),
		apiClient:      NewAPIClient(opts.Config),
		toolManager:    NewToolManager(),
		config:         opts.Config,
		logger:         log,
		permManager:    permManager,
		sessionManager: sessionManager,
		secureStorage:  secureStorage,
		mcpManager:     mcp.NewClientManager(),
		status:         StatusIdle,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Register built-in tools
	e.registerBuiltinTools()

	// Connect configured MCP servers
	e.connectMCPServers()

	return e
}

// GetEventBus returns the EventBus for UI subscription.
func (e *Engine) GetEventBus() *EventBus {
	return e.eventBus
}

// GetSession returns the current session.
func (e *Engine) GetSession() *Session {
	return e.session
}

// GetStatus returns the current engine status.
func (e *Engine) GetStatus() EngineStatus {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	return e.status
}

// setStatus updates the engine status and publishes an event.
func (e *Engine) setStatus(status EngineStatus) {
	e.statusMu.Lock()
	e.status = status
	e.statusMu.Unlock()

	e.eventBus.Publish(NewEvent(EventStatusChange, status, SourceEngine))
}

// GetCost returns the current cost statistics.
func (e *Engine) GetCost() CostInfo {
	e.costMu.RLock()
	defer e.costMu.RUnlock()

	return CostInfo{
		TotalCost:        e.totalCost,
		InputTokens:      e.inputTokens,
		OutputTokens:     e.outputTokens,
		CacheReadTokens:  e.cacheReadTokens,
		CacheWriteTokens: e.cacheWriteTokens,
	}
}

// CostInfo contains cost tracking information.
type CostInfo struct {
	TotalCost        float64 `json:"total_cost"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
}

// CostInfoForDisplay contains formatted cost info for UI display.
type CostInfoForDisplay struct {
	TotalCost    float64 `json:"total_cost"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// GetCostForDisplay returns cost info formatted for display.
func (e *Engine) GetCostForDisplay() CostInfoForDisplay {
	cost := e.GetCost()
	return CostInfoForDisplay{
		TotalCost:    cost.TotalCost,
		InputTokens:  cost.InputTokens,
		OutputTokens: cost.OutputTokens,
	}
}

// updateCost adds to the cost counters and publishes an event.
func (e *Engine) updateCost(cost float64, input, output, cacheRead, cacheWrite int64) {
	e.costMu.Lock()
	e.totalCost += cost
	e.inputTokens += input
	e.outputTokens += output
	e.cacheReadTokens += cacheRead
	e.cacheWriteTokens += cacheWrite
	e.costMu.Unlock()

	e.eventBus.Publish(NewEvent(EventCostUpdate, e.GetCost(), SourceEngine))
}

// ProcessInput handles user input from any UI source.
// This is the main entry point for user interactions.
func (e *Engine) ProcessInput(input string, source Source) error {
	// Check for slash commands
	if len(input) > 0 && input[0] == '/' {
		return e.processCommand(input, source)
	}

	// Regular message - send to API
	return e.sendMessage(input, source)
}

// processCommand handles slash commands like /help, /clear, /model.
func (e *Engine) processCommand(cmd string, source Source) error {
	e.eventBus.Publish(NewEvent(EventUserCommand, cmd, source))

	// Parse command
	parts := parseCommand(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	commandName := parts[0]
	args := parts[1:]

	switch commandName {
	case "/help":
		e.eventBus.Publish(NewEvent(EventToolOutput, GetHelpText(), SourceEngine))
	case "/clear":
		e.session.Clear()
		e.eventBus.Publish(NewEvent(EventToolOutput, "Session cleared.", SourceEngine))
	case "/model":
		if len(args) > 0 {
			e.config.Model = args[0]
			e.eventBus.Publish(NewEvent(EventToolOutput, 
				fmt.Sprintf("Model set to: %s", args[0]), SourceEngine))
		} else {
			e.eventBus.Publish(NewEvent(EventToolOutput, 
				fmt.Sprintf("Current model: %s", e.config.Model), SourceEngine))
		}
	case "/cost":
		cost := e.GetCost()
		e.eventBus.Publish(NewEvent(EventToolOutput, 
			fmt.Sprintf("Total cost: $%.4f\nInput: %d tokens\nOutput: %d tokens",
				cost.TotalCost, cost.InputTokens, cost.OutputTokens), SourceEngine))
	case "/status":
		e.eventBus.Publish(NewEvent(EventToolOutput,
			fmt.Sprintf("Status: %s\nSession ID: %s\nMessages: %d",
				e.GetStatus(), e.session.ID, e.session.MessageCount()), SourceEngine))
	case "/save":
		title := ""
		if len(args) > 0 {
			title = strings.Join(args, " ")
		}
		e.autoSaveSession(title)
		e.eventBus.Publish(NewEvent(EventToolOutput, 
			fmt.Sprintf("Session saved: %s", e.session.ID), SourceEngine))
	case "/load":
		if len(args) == 0 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /load <session-id>"), SourceEngine))
			return nil
		}
		sessionID := args[0]
		if err := e.LoadSession(sessionID); err != nil {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("failed to load session: %w", err), SourceEngine))
			return nil
		}
		e.eventBus.Publish(NewEvent(EventToolOutput, 
			fmt.Sprintf("Session loaded: %s (%d messages)", sessionID, e.session.MessageCount()), SourceEngine))
	case "/sessions":
		sessions, err := e.ListSessions()
		if err != nil {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("failed to list sessions: %w", err), SourceEngine))
			return nil
		}
		e.eventBus.Publish(NewEvent(EventSessionList, sessions, SourceEngine))
		if len(sessions) == 0 {
			e.eventBus.Publish(NewEvent(EventToolOutput, "No saved sessions.", SourceEngine))
		} else {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Saved Sessions (%d):\n\n", len(sessions)))
			for i, s := range sessions {
				title := s.Title
				if title == "" {
					title = "Untitled"
				}
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, s.ID))
				sb.WriteString(fmt.Sprintf("   Title: %s\n", title))
				sb.WriteString(fmt.Sprintf("   Messages: %d\n", s.MessageCount))
				sb.WriteString(fmt.Sprintf("   Updated: %s\n", s.UpdatedAt.Format("2006-01-02 15:04")))
				sb.WriteString("\n")
			}
			e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
		}
	case "/export":
		format := "text"
		if len(args) > 0 {
			format = args[0]
		}
		exported := e.exportSession(format)
		e.eventBus.Publish(NewEvent(EventToolOutput, exported, SourceEngine))
	case "/search":
		if len(args) == 0 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /search <query>"), SourceEngine))
			return nil
		}
		query := strings.Join(args, " ")
		results, err := e.SearchSessions(query, 20)
		if err != nil {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("search failed: %w", err), SourceEngine))
			return nil
		}
		e.eventBus.Publish(NewEvent(EventSearchResult, results, SourceEngine))
		if len(results) == 0 {
			e.eventBus.Publish(NewEvent(EventToolOutput, 
				fmt.Sprintf("No results found for: %s", query), SourceEngine))
		} else {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Search Results for '%s' (%d matches):\n\n", query, len(results)))
			for i, r := range results {
				title := r.SessionTitle
				if title == "" {
					title = "Untitled"
				}
				sb.WriteString(fmt.Sprintf("%d. Session: %s (%s)\n", i+1, r.SessionID, title))
				sb.WriteString(fmt.Sprintf("   Role: %s\n", r.Role))
				sb.WriteString(fmt.Sprintf("   Time: %s\n", r.Time.Format("2006-01-02 15:04")))
				sb.WriteString(fmt.Sprintf("   Match: %s\n", r.MatchSnippet))
				sb.WriteString("\n")
			}
			sb.WriteString("Use /load <session-id> to load a session.")
			e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
		}
	case "/mcp":
		e.handleMCPCommand(args)
	default:
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("unknown command: %s. Type /help for available commands.", commandName), SourceEngine))
	}

	return nil
}

// sendMessage sends a user message to the Claude API.
func (e *Engine) sendMessage(input string, source Source) error {
	e.setStatus(StatusThinking)

	// Add user message to session
	userMsg := Message{
		Role:    RoleUser,
		Content: input,
		Time:    time.Now(),
	}
	e.session.AddMessage(userMsg)

	// Publish user input event
	e.eventBus.Publish(NewEvent(EventUserInput, userMsg, source))

	// Send to API in background
	go e.processAPIRequest()

	return nil
}

// processAPIRequest handles the full API request/response cycle.
func (e *Engine) processAPIRequest() {
	defer e.setStatus(StatusIdle)

	// Build API request
	req := APIRequest{
		Model:     e.config.Model,
		MaxTokens: e.config.MaxTokens,
		Messages:  e.session.GetMessagesForAPI(),
		Tools:     BuildToolsForAPI(e.toolManager.GetToolDefinitions()),
	}

	// Stream response from API
	respChan, err := e.apiClient.Stream(e.ctx, req)
	if err != nil {
		e.eventBus.Publish(NewEvent(EventAPIError, err, SourceAPI))
		return
	}

	// Process streaming response
	var (
		assistantContent  string
		assistantThinking string
		toolCalls         []ToolCall
		messageID         string
	)

	for resp := range respChan {
		switch resp.Type {
		case "message_start":
			// Message started
			messageID = resp.MessageID
			e.eventBus.Publish(NewEvent(EventAPIStream, StreamProgress{
				Type:   "message_start",
				Model:  resp.Model,
			}, SourceAPI))

		case "text":
			// Text content
			assistantContent += resp.Text
			e.eventBus.Publish(NewEvent(EventAPIStream, StreamProgress{
				Type: "text",
				Text: resp.Text,
			}, SourceAPI))

		case "thinking":
			// Thinking content
			assistantThinking += resp.Thinking
			e.eventBus.Publish(NewEvent(EventAPIStream, StreamProgress{
				Type:     "thinking",
				Thinking: resp.Thinking,
			}, SourceAPI))

		case "tool_use":
			// Tool use complete
			if resp.ToolName != "" {
				toolCalls = append(toolCalls, ToolCall{
					ID:    resp.ToolUseID,
					Name:  resp.ToolName,
					Input: resp.ToolInput,
				})
			}

		case "usage_update":
			// Update cost tracking
			if resp.Usage != nil {
				e.updateCost(resp.Cost,
					resp.Usage.InputTokens,
					resp.Usage.OutputTokens,
					resp.Usage.CacheReadTokens,
					resp.Usage.CacheWriteTokens,
				)
			}

		case "error":
			// Error from API
			e.eventBus.Publish(NewEvent(EventAPIError, resp.Error, SourceAPI))
			return

		case "done":
			// Message complete - save to session
			if assistantContent != "" || len(toolCalls) > 0 {
				assistantMsg := Message{
					Role:      RoleAssistant,
					Content:   assistantContent,
					Thinking:  assistantThinking,
					ToolCalls: toolCalls,
					Time:      time.Now(),
				}
				e.session.AddMessage(assistantMsg)
			}

			// Update final usage
			if resp.Usage != nil {
				e.updateCost(resp.Cost,
					resp.Usage.InputTokens,
					resp.Usage.OutputTokens,
					resp.Usage.CacheReadTokens,
					resp.Usage.CacheWriteTokens,
				)
			}

			e.eventBus.Publish(NewEvent(EventAPIComplete, StreamProgress{
				Type:      "done",
				MessageID: messageID,
			}, SourceAPI))

			// Auto-save session if enabled
			if e.config.AutoSave && e.session.MessageCount() > 0 {
				e.autoSaveSession("")
			}

			// Execute tool calls if any
			if len(toolCalls) > 0 {
				go e.executeToolCalls(toolCalls)
			}
		}
	}
}

// StreamProgress represents streaming progress for UI updates.
type StreamProgress struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Model     string          `json:"model,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
}

// ToolCall represents a tool call from the assistant.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// executeToolCalls executes a list of tool calls.
func (e *Engine) executeToolCalls(toolCalls []ToolCall) {
	for _, tc := range toolCalls {
		e.executeTool(tc.Name, string(tc.Input), tc.ID)
	}
}

// executeTool runs a tool and publishes events for its execution.
func (e *Engine) executeTool(name, input, toolUseID string) {
	e.setStatus(StatusToolExec)

	// Store current tool info
	e.toolMu.Lock()
	e.currentToolName = name
	e.currentToolUseID = toolUseID
	e.toolMu.Unlock()

	// Publish tool start event
	e.eventBus.Publish(NewEvent(EventToolStart, ToolInfo{
		Name:      name,
		Input:     input,
		ToolUseID: toolUseID,
	}, SourceTool))

	// Check tool permission
	if e.permManager != nil {
		permResult := e.permManager.CheckToolPermission(name, input)
		if !permResult.Allowed {
			// Tool is denied
			e.eventBus.Publish(NewEvent(EventToolError, 
				fmt.Errorf("tool '%s' denied: %s", name, permResult.Message), SourceTool))
			e.setStatus(StatusThinking)
			return
		}
		if permResult.AskUser {
			// Need user confirmation - publish permission request event
			e.eventBus.Publish(NewEvent(EventPermissionRequest, PermissionRequest{
				ToolName:  name,
				Input:     input,
				ToolUseID: toolUseID,
				Message:   permResult.Message,
			}, SourceTool))
			// For now, auto-approve if user confirmation is needed
			// In future, wait for user response via EventBus
			e.logger.Info("Tool permission requested", 
				logger.Field{Key: "tool", Value: name},
				logger.Field{Key: "auto_approved", Value: true})
		}
	}

	// Get tool
	tool, ok := e.toolManager.GetTool(name)
	if !ok {
		e.eventBus.Publish(NewEvent(EventToolError, 
			fmt.Errorf("unknown tool: %s", name), SourceTool))
		e.setStatus(StatusThinking)
		return
	}

	// Execute tool
	result, err := tool.Execute(e.ctx, input)

	// Publish result
	if err != nil {
		e.eventBus.Publish(NewEvent(EventToolError, err, SourceTool))
	} else {
		e.eventBus.Publish(NewEvent(EventToolOutput, result, SourceTool))
	}

	e.eventBus.Publish(NewEvent(EventToolComplete, ToolInfo{
		Name:      name,
		ToolUseID: toolUseID,
	}, SourceTool))

	// Clear current tool
	e.toolMu.Lock()
	e.currentToolName = ""
	e.currentToolUseID = ""
	e.toolMu.Unlock()

	e.setStatus(StatusThinking)
}

// GetCurrentTool returns info about the currently executing tool.
func (e *Engine) GetCurrentTool() (name, toolUseID string, ok bool) {
	e.toolMu.RLock()
	defer e.toolMu.RUnlock()
	if e.currentToolName != "" {
		return e.currentToolName, e.currentToolUseID, true
	}
	return "", "", false
}

// GetConfig returns the engine configuration.
func (e *Engine) GetConfig() *Config {
	return e.config
}

// GetToolManager returns the tool manager.
func (e *Engine) GetToolManager() *ToolManager {
	return e.toolManager
}

// registerBuiltinTools registers all built-in tools.
func (e *Engine) registerBuiltinTools() {
	e.toolManager.RegisterTool("bash", &BashTool{})
	e.toolManager.RegisterTool("read", &FileReadTool{})
	e.toolManager.RegisterTool("write", &FileWriteTool{})
	e.toolManager.RegisterTool("edit", &FileEditTool{})
	e.toolManager.RegisterTool("glob", &GlobTool{})
	e.toolManager.RegisterTool("grep", &GrepTool{})
	e.toolManager.RegisterTool("web_search", &WebSearchTool{})
	e.toolManager.RegisterTool("web_fetch", &WebFetchTool{})
}

// Shutdown gracefully shuts down the engine.
func (e *Engine) Shutdown() {
	e.setStatus(StatusShuttingDown)
	e.cancel()
	e.eventBus.Close()
}

// parseCommand splits a command string into parts.
func parseCommand(cmd string) []string {
	// Simple split for now, can be enhanced for quoted arguments
	result := []string{}
	current := ""
	inQuote := false

	for _, ch := range cmd {
		if ch == '"' {
			inQuote = !inQuote
		} else if ch == ' ' && !inQuote {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// GetHelpText returns the help text for available commands.
func GetHelpText() string {
	return `SuperTerminal - Available Commands:

Session Commands:
  /help              Show this help message
  /clear             Clear the current session
  /status            Show engine status
  /save [title]      Save current session (with optional title)
  /load <id>         Load a saved session
  /sessions          List all saved sessions
  /search <query>    Search all sessions for content
  /export [format]   Export session (text, json, markdown)

Model Commands:
  /model [name]      Set or show the current model
  /cost              Show cost statistics

MCP Commands:
  /mcp               Show MCP status and commands
  /mcp list          List connected MCP servers
  /mcp tools [name]  List MCP tools (all or from server)
  /mcp resources     List MCP resources
  /mcp prompts       List MCP prompts
  /mcp read <server> <uri>   Read a resource
  /mcp connect <name> <type> <cmd|url>  Connect to server
  /mcp disconnect <name>     Disconnect from server

Exit:
  /exit              Exit SuperTerminal

Built-in Tools:
  - bash: Execute shell commands
  - read: Read file contents
  - write: Write file contents
  - edit: Edit file contents
  - glob: Find files matching pattern
  - grep: Search file contents
  - web_search: Search the web
  - web_fetch: Fetch web content

Press Ctrl+C to cancel current operation.
`
}

// === New Component Accessors ===

// GetLogger returns the logger instance.
func (e *Engine) GetLogger() *logger.Logger {
	return e.logger
}

// GetPermissionManager returns the permission manager.
func (e *Engine) GetPermissionManager() *security.PermissionManager {
	return e.permManager
}

// GetSessionManager returns the session persistence manager.
func (e *Engine) GetSessionManager() *persistence.SessionManager {
	return e.sessionManager
}

// GetSecureStorage returns the secure storage instance.
func (e *Engine) GetSecureStorage() *security.SecureStorage {
	return e.secureStorage
}

// CheckToolPermission checks if a tool action is permitted.
func (e *Engine) CheckToolPermission(toolName, input string) security.PermissionResult {
	return e.permManager.CheckToolPermission(toolName, input)
}

// SaveSession saves the current session to disk.
func (e *Engine) SaveSession(id string) error {
	if e.sessionManager == nil {
		return nil
	}

	sessionData := &persistence.SessionData{
		ID:           id,
		CreatedAt:    e.session.CreatedAt,
		UpdatedAt:    time.Now(),
		MessageCount: len(e.session.Messages),
		Config:       persistence.Config{
			Model:     e.config.Model,
			MaxTokens: e.config.MaxTokens,
		},
	}

	// Convert messages
	for _, msg := range e.session.Messages {
		sessionData.Messages = append(sessionData.Messages, persistence.Message{
			ID:        msg.ID,
			Role:      string(msg.Role),
			Content:   msg.Content,
			Thinking:  msg.Thinking,
			Time:      msg.Time,
			ToolName:  msg.ToolName,
			ToolUseID: msg.ToolUseID,
			ToolResult: msg.ToolResult,
		})
	}

	return e.sessionManager.Save(sessionData)
}

// LoadSession loads a session from disk.
func (e *Engine) LoadSession(id string) error {
	if e.sessionManager == nil {
		return nil
	}

	sessionData, err := e.sessionManager.Load(id)
	if err != nil {
		return err
	}

	// Restore session
	e.session = NewSession()
	e.session.CreatedAt = sessionData.CreatedAt

	for _, msg := range sessionData.Messages {
		e.session.Messages = append(e.session.Messages, Message{
			ID:        msg.ID,
			Role:      MessageRole(msg.Role),
			Content:   msg.Content,
			Thinking:  msg.Thinking,
			Time:      msg.Time,
			ToolName:  msg.ToolName,
			ToolUseID: msg.ToolUseID,
			ToolResult: msg.ToolResult,
		})
	}

	// Restore config
	if sessionData.Config.Model != "" {
		e.config.Model = sessionData.Config.Model
	}
	if sessionData.Config.MaxTokens > 0 {
		e.config.MaxTokens = sessionData.Config.MaxTokens
	}

	// Publish session loaded event
	e.eventBus.Publish(NewEvent(EventSessionLoaded, id, SourceEngine))
	return nil
}

// ListSessions returns a list of saved sessions.
func (e *Engine) ListSessions() ([]persistence.SessionMeta, error) {
	if e.sessionManager == nil {
		return nil, nil
	}
	return e.sessionManager.List()
}

// GetAPIKey securely retrieves the API key.
func (e *Engine) GetAPIKey(keyType string) string {
	if e.secureStorage == nil {
		// Fallback to config
		return e.config.APIKey
	}
	secretType := security.SecretType(keyType)
	value, _ := e.secureStorage.GetSecret(secretType)
	return value
}

// SetAPIKey securely stores an API key.
func (e *Engine) SetAPIKey(keyType, value string) error {
	if e.secureStorage == nil {
		e.config.APIKey = value
		return nil
	}
	secretType := security.SecretType(keyType)
	return e.secureStorage.SetSecret(secretType, value, security.StorageEnv)
}

// autoSaveSession saves the current session with optional title.
func (e *Engine) autoSaveSession(title string) {
	if e.sessionManager == nil {
		return
	}

	sessionData := &persistence.SessionData{
		ID:           e.session.ID,
		CreatedAt:    e.session.CreatedAt,
		UpdatedAt:    time.Now(),
		Title:        title,
		MessageCount: len(e.session.Messages),
		Config: persistence.Config{
			Model:     e.config.Model,
			MaxTokens: e.config.MaxTokens,
		},
	}

	// Convert messages
	for _, msg := range e.session.Messages {
		sessionData.Messages = append(sessionData.Messages, persistence.Message{
			ID:         msg.ID,
			Role:       string(msg.Role),
			Content:    msg.Content,
			Thinking:   msg.Thinking,
			Time:       msg.Time,
			ToolName:   msg.ToolName,
			ToolUseID:  msg.ToolUseID,
			ToolResult: msg.ToolResult,
		})
	}

	if err := e.sessionManager.Save(sessionData); err == nil {
		e.eventBus.Publish(NewEvent(EventSessionSave, e.session.ID, SourceEngine))
	}
}

// exportSession exports the current session in the specified format.
func (e *Engine) exportSession(format string) string {
	switch format {
	case "json":
		data := map[string]interface{}{
			"session_id":  e.session.ID,
			"created_at":  e.session.CreatedAt,
			"message_count": len(e.session.Messages),
			"model":       e.config.Model,
			"messages":    e.session.Messages,
		}
		jsonData, _ := json.MarshalIndent(data, "", "  ")
		return string(jsonData)
	
	case "markdown", "md":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Session %s\n\n", e.session.ID))
		sb.WriteString(fmt.Sprintf("**Created:** %s\n", e.session.CreatedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("**Model:** %s\n", e.config.Model))
		sb.WriteString(fmt.Sprintf("**Messages:** %d\n\n---\n\n", len(e.session.Messages)))
		
		for _, msg := range e.session.Messages {
			if msg.Role == RoleUser {
				sb.WriteString(fmt.Sprintf("## User\n\n%s\n\n", msg.Content))
			} else if msg.Role == RoleAssistant {
				sb.WriteString(fmt.Sprintf("## Assistant\n\n%s\n\n", msg.Content))
				if msg.Thinking != "" {
					sb.WriteString(fmt.Sprintf("> Thinking: %s\n\n", msg.Thinking))
				}
			} else if msg.Role == RoleTool {
				sb.WriteString(fmt.Sprintf("### Tool: %s\n\n```\n%s\n```\n\n", msg.ToolName, msg.ToolResult))
			}
		}
		return sb.String()
	
	default: // text format
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Session: %s\n", e.session.ID))
		sb.WriteString(fmt.Sprintf("Created: %s\n", e.session.CreatedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("Model: %s\n", e.config.Model))
		sb.WriteString(fmt.Sprintf("Messages: %d\n\n", len(e.session.Messages)))
		
		for _, msg := range e.session.Messages {
			sb.WriteString(fmt.Sprintf("[%s] ", msg.Time.Format("15:04")))
			if msg.Role == RoleUser {
				sb.WriteString("USER: ")
			} else if msg.Role == RoleAssistant {
				sb.WriteString("ASSISTANT: ")
			} else if msg.Role == RoleTool {
				sb.WriteString(fmt.Sprintf("TOOL(%s): ", msg.ToolName))
			}
			sb.WriteString(msg.Content)
			if msg.Thinking != "" {
				sb.WriteString(fmt.Sprintf("\n  Thinking: %s", msg.Thinking))
			}
			if msg.ToolResult != "" {
				sb.WriteString(fmt.Sprintf("\n  Result: %s", msg.ToolResult))
			}
			sb.WriteString("\n\n")
		}
		return sb.String()
	}
}

// AutoSaveEnabled returns whether auto-save is enabled.
func (e *Engine) AutoSaveEnabled() bool {
	return e.config.AutoSave
}

// SearchSessions searches all sessions for a query.
func (e *Engine) SearchSessions(query string, maxResults int) ([]persistence.SearchResult, error) {
	if e.sessionManager == nil {
		return nil, nil
	}
	return e.sessionManager.SearchWithMatches(query, maxResults)
}

// === MCP Methods ===

// connectMCPServers connects to configured MCP servers.
func (e *Engine) connectMCPServers() {
	if e.mcpManager == nil || len(e.config.MCPServers) == 0 {
		return
	}

	for _, serverConfig := range e.config.MCPServers {
		config := mcp.ClientConfig{
			Name:    serverConfig.Name,
			Type:    serverConfig.Type,
			Command: serverConfig.Command,
			Args:    serverConfig.Args,
			Env:     serverConfig.Env,
			URL:     serverConfig.URL,
		}

		client, err := e.mcpManager.AddClient(serverConfig.Name, config)
		if err != nil {
			e.logger.Error("Failed to connect MCP server", 
				logger.Field{Key: "server", Value: serverConfig.Name},
				logger.Field{Key: "error", Value: err.Error()})
			continue
		}

		e.logger.Info("Connected MCP server",
			logger.Field{Key: "server", Value: serverConfig.Name},
			logger.Field{Key: "tools", Value: len(client.GetTools())})

		// Register MCP tools
		e.registerMCPTools(serverConfig.Name, client)
	}
}

// registerMCPTools registers tools from an MCP client.
func (e *Engine) registerMCPTools(serverName string, client *mcp.Client) {
	tools := client.GetTools()
	for _, toolDef := range tools {
		// Create MCP tool wrapper
		mcpTool := &MCPTool{
			serverName: serverName,
			client:     client,
			definition: toolDef,
		}
		
		// Register with prefixed name to avoid conflicts
		toolName := fmt.Sprintf("%s_%s", serverName, toolDef.Name)
		e.toolManager.RegisterTool(toolName, mcpTool)
		
		e.logger.Info("Registered MCP tool",
			logger.Field{Key: "tool", Value: toolName},
			logger.Field{Key: "server", Value: serverName})
	}
}

// GetMCPManager returns the MCP client manager.
func (e *Engine) GetMCPManager() *mcp.ClientManager {
	return e.mcpManager
}

// ListMCPServers returns connected MCP server names.
func (e *Engine) ListMCPServers() []string {
	if e.mcpManager == nil {
		return nil
	}
	return e.mcpManager.ListClients()
}

// GetMCPTools returns all tools from MCP servers.
func (e *Engine) GetMCPTools() map[string][]mcp.ToolDefinition {
	if e.mcpManager == nil {
		return nil
	}
	return e.mcpManager.GetAllTools()
}

// MCPTool wraps an MCP tool for use in the engine.
type MCPTool struct {
	serverName string
	client     *mcp.Client
	definition mcp.ToolDefinition
}

// Name returns the tool name.
func (t *MCPTool) Name() string {
	return fmt.Sprintf("%s_%s", t.serverName, t.definition.Name)
}

// Description returns the tool description.
func (t *MCPTool) Description() string {
	return t.definition.Description
}

// InputSchema returns the input schema.
func (t *MCPTool) InputSchema() map[string]interface{} {
	return t.definition.InputSchema
}

// Execute runs the MCP tool.
func (t *MCPTool) Execute(ctx context.Context, input string) (string, error) {
	result, err := t.client.CallTool(t.definition.Name, json.RawMessage(input))
	if err != nil {
		return "", err
	}

	// Extract text from content blocks
	var output strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			output.WriteString(block.Text)
		} else if block.Type == "resource" && block.MimeType != "" {
			output.WriteString(fmt.Sprintf("[%s: %s]", block.MimeType, block.Data))
		}
	}

	if result.IsError {
		return output.String(), fmt.Errorf("tool execution error")
	}

	return output.String(), nil
}

// handleMCPCommand handles MCP-related commands.
func (e *Engine) handleMCPCommand(args []string) {
	if len(args) == 0 {
		// Show MCP status
		e.showMCPStatus()
		return
	}

	subCommand := args[0]
	subArgs := args[1:]

	switch subCommand {
	case "list", "servers":
		e.listMCPServers()
	case "tools":
		if len(subArgs) == 0 {
			e.listAllMCPTools()
		} else {
			e.listServerTools(subArgs[0])
		}
	case "resources":
		if len(subArgs) == 0 {
			e.listAllMCPResources()
		} else {
			e.listServerResources(subArgs[0])
		}
	case "prompts":
		if len(subArgs) == 0 {
			e.listAllMCPPrompts()
		} else {
			e.listServerPrompts(subArgs[0])
		}
	case "read":
		if len(subArgs) < 2 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /mcp read <server> <uri>"), SourceEngine))
			return
		}
		e.readMCPResource(subArgs[0], subArgs[1])
	case "prompt":
		if len(subArgs) < 1 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /mcp prompt <server> <name> [args]"), SourceEngine))
			return
		}
		promptName := subArgs[0]
		promptArgs := ""
		if len(subArgs) > 1 {
			promptArgs = strings.Join(subArgs[1:], " ")
		}
		e.getMCPPrompt(promptName, promptArgs)
	case "connect":
		if len(subArgs) < 2 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /mcp connect <name> <type> [command|url]"), SourceEngine))
			return
		}
		e.connectMCPServer(subArgs[0], subArgs[1], subArgs[2:])
	case "disconnect":
		if len(subArgs) == 0 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("usage: /mcp disconnect <server>"), SourceEngine))
			return
		}
		e.disconnectMCPServer(subArgs[0])
	default:
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("unknown MCP subcommand: %s. Use /mcp for help.", subCommand), SourceEngine))
	}
}

// showMCPStatus shows overall MCP status.
func (e *Engine) showMCPStatus() {
	servers := e.ListMCPServers()
	
	var sb strings.Builder
	sb.WriteString("MCP Status:\n\n")
	sb.WriteString("Connected Servers: ")
	if len(servers) == 0 {
		sb.WriteString("None\n")
	} else {
		sb.WriteString(fmt.Sprintf("%d\n", len(servers)))
		for _, name := range servers {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}
	
	sb.WriteString("\nCommands:\n")
	sb.WriteString("  /mcp list          List connected servers\n")
	sb.WriteString("  /mcp tools [name]  List tools (all or from server)\n")
	sb.WriteString("  /mcp resources     List available resources\n")
	sb.WriteString("  /mcp prompts       List available prompts\n")
	sb.WriteString("  /mcp read <s> <u>  Read a resource\n")
	sb.WriteString("  /mcp prompt <n>    Get a prompt\n")
	sb.WriteString("  /mcp connect       Connect to a new server\n")
	sb.WriteString("  /mcp disconnect    Disconnect from a server\n")
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listMCPServers lists connected MCP servers.
func (e *Engine) listMCPServers() {
	servers := e.ListMCPServers()
	
	if len(servers) == 0 {
		e.eventBus.Publish(NewEvent(EventToolOutput, "No MCP servers connected.\n", SourceEngine))
		return
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Connected MCP Servers (%d):\n\n", len(servers)))
	
	for _, name := range servers {
		client, ok := e.mcpManager.GetClient(name)
		if ok {
			info := client.GetServerInfo()
			sb.WriteString(fmt.Sprintf("Server: %s\n", name))
			sb.WriteString(fmt.Sprintf("  Name: %s\n", info.Name))
			sb.WriteString(fmt.Sprintf("  Version: %s\n", info.Version))
			sb.WriteString(fmt.Sprintf("  Tools: %d\n", len(client.GetTools())))
			sb.WriteString(fmt.Sprintf("  Resources: %d\n", len(client.GetResources())))
			sb.WriteString(fmt.Sprintf("  Prompts: %d\n", len(client.GetPrompts())))
			sb.WriteString("\n")
		}
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listAllMCPTools lists all MCP tools.
func (e *Engine) listAllMCPTools() {
	tools := e.GetMCPTools()
	
	if len(tools) == 0 {
		e.eventBus.Publish(NewEvent(EventToolOutput, "No MCP tools available.\n", SourceEngine))
		return
	}
	
	var sb strings.Builder
	sb.WriteString("MCP Tools:\n\n")
	
	for serverName, toolList := range tools {
		sb.WriteString(fmt.Sprintf("Server: %s (%d tools)\n", serverName, len(toolList)))
		for _, tool := range toolList {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", tool.Name, tool.Description))
		}
		sb.WriteString("\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listServerTools lists tools from a specific server.
func (e *Engine) listServerTools(serverName string) {
	client, ok := e.mcpManager.GetClient(serverName)
	if !ok {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("server '%s' not found", serverName), SourceEngine))
		return
	}
	
	tools := client.GetTools()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Tools from %s (%d):\n\n", serverName, len(tools)))
	
	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("Tool: %s\n", tool.Name))
		sb.WriteString(fmt.Sprintf("  Description: %s\n", tool.Description))
		if len(tool.InputSchema) > 0 {
			sb.WriteString("  Has input schema\n")
		}
		sb.WriteString("\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listAllMCPResources lists all MCP resources.
func (e *Engine) listAllMCPResources() {
	servers := e.ListMCPServers()
	
	var sb strings.Builder
	sb.WriteString("MCP Resources:\n\n")
	
	totalResources := 0
	for _, name := range servers {
		client, ok := e.mcpManager.GetClient(name)
		if ok {
			resources := client.GetResources()
			totalResources += len(resources)
			sb.WriteString(fmt.Sprintf("Server: %s (%d resources)\n", name, len(resources)))
			for _, res := range resources {
				sb.WriteString(fmt.Sprintf("  - %s (%s)\n", res.URI, res.Name))
				if res.Description != "" {
					sb.WriteString(fmt.Sprintf("    %s\n", res.Description))
				}
			}
			sb.WriteString("\n")
		}
	}
	
	if totalResources == 0 {
		sb.WriteString("No resources available.\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listServerResources lists resources from a specific server.
func (e *Engine) listServerResources(serverName string) {
	client, ok := e.mcpManager.GetClient(serverName)
	if !ok {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("server '%s' not found", serverName), SourceEngine))
		return
	}
	
	resources := client.GetResources()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Resources from %s (%d):\n\n", serverName, len(resources)))
	
	for _, res := range resources {
		sb.WriteString(fmt.Sprintf("URI: %s\n", res.URI))
		sb.WriteString(fmt.Sprintf("  Name: %s\n", res.Name))
		if res.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", res.Description))
		}
		if res.MimeType != "" {
			sb.WriteString(fmt.Sprintf("  Type: %s\n", res.MimeType))
		}
		sb.WriteString("\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listAllMCPPrompts lists all MCP prompts.
func (e *Engine) listAllMCPPrompts() {
	servers := e.ListMCPServers()
	
	var sb strings.Builder
	sb.WriteString("MCP Prompts:\n\n")
	
	totalPrompts := 0
	for _, name := range servers {
		client, ok := e.mcpManager.GetClient(name)
		if ok {
			prompts := client.GetPrompts()
			totalPrompts += len(prompts)
			sb.WriteString(fmt.Sprintf("Server: %s (%d prompts)\n", name, len(prompts)))
			for _, prompt := range prompts {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", prompt.Name, prompt.Description))
			}
			sb.WriteString("\n")
		}
	}
	
	if totalPrompts == 0 {
		sb.WriteString("No prompts available.\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// listServerPrompts lists prompts from a specific server.
func (e *Engine) listServerPrompts(serverName string) {
	client, ok := e.mcpManager.GetClient(serverName)
	if !ok {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("server '%s' not found", serverName), SourceEngine))
		return
	}
	
	prompts := client.GetPrompts()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Prompts from %s (%d):\n\n", serverName, len(prompts)))
	
	for _, prompt := range prompts {
		sb.WriteString(fmt.Sprintf("Prompt: %s\n", prompt.Name))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", prompt.Description))
		}
		if len(prompt.Arguments) > 0 {
			sb.WriteString("  Arguments:\n")
			for _, arg := range prompt.Arguments {
				sb.WriteString(fmt.Sprintf("    - %s", arg.Name))
				if arg.Required {
					sb.WriteString(" (required)")
				}
				if arg.Description != "" {
					sb.WriteString(fmt.Sprintf(": %s", arg.Description))
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// readMCPResource reads a resource from an MCP server.
func (e *Engine) readMCPResource(serverName, uri string) {
	client, ok := e.mcpManager.GetClient(serverName)
	if !ok {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("server '%s' not found", serverName), SourceEngine))
		return
	}
	
	blocks, err := client.ReadResource(uri)
	if err != nil {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("failed to read resource: %w", err), SourceEngine))
		return
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Resource: %s\n\n", uri))
	
	for _, block := range blocks {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		} else if block.MimeType != "" {
			sb.WriteString(fmt.Sprintf("[%s data: %d bytes]\n", block.MimeType, len(block.Data)))
		}
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// getMCPPrompt gets a prompt from an MCP server.
func (e *Engine) getMCPPrompt(promptName, argsStr string) {
	// Find which server has this prompt
	servers := e.ListMCPServers()
	var foundClient *mcp.Client
	var foundServer string
	
	for _, name := range servers {
		client, ok := e.mcpManager.GetClient(name)
		if ok {
			for _, prompt := range client.GetPrompts() {
				if prompt.Name == promptName {
					foundClient = client
					foundServer = name
					break
				}
			}
		}
		if foundClient != nil {
			break
		}
	}
	
	if foundClient == nil {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("prompt '%s' not found", promptName), SourceEngine))
		return
	}
	
	var args json.RawMessage
	if argsStr != "" {
		// Try to parse as JSON
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			// Treat as simple string argument
			args = json.RawMessage(fmt.Sprintf(`{"arg":"%s"}`, argsStr))
		}
	} else {
		args = json.RawMessage("{}")
	}
	
	content, err := foundClient.GetPrompt(promptName, args)
	if err != nil {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("failed to get prompt: %w", err), SourceEngine))
		return
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Prompt: %s (from %s)\n\n", promptName, foundServer))
	sb.WriteString(content)
	
	e.eventBus.Publish(NewEvent(EventToolOutput, sb.String(), SourceEngine))
}

// connectMCPServer connects to a new MCP server dynamically.
func (e *Engine) connectMCPServer(name, typ string, params []string) {
	config := mcp.ClientConfig{
		Name: name,
		Type: typ,
	}
	
	switch typ {
	case "stdio":
		if len(params) == 0 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("stdio requires command"), SourceEngine))
			return
		}
		config.Command = params[0]
		config.Args = params[1:]
	case "http", "sse":
		if len(params) == 0 {
			e.eventBus.Publish(NewEvent(EventError,
				fmt.Errorf("%s requires URL", typ), SourceEngine))
			return
		}
		config.URL = params[0]
	default:
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("unsupported type: %s", typ), SourceEngine))
		return
	}
	
	client, err := e.mcpManager.AddClient(name, config)
	if err != nil {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("failed to connect: %w", err), SourceEngine))
		return
	}
	
	// Register tools
	e.registerMCPTools(name, client)
	
	e.eventBus.Publish(NewEvent(EventToolOutput,
		fmt.Sprintf("Connected to MCP server '%s' with %d tools.\n", name, len(client.GetTools())),
		SourceEngine))
}

// disconnectMCPServer disconnects from an MCP server.
func (e *Engine) disconnectMCPServer(serverName string) {
	if err := e.mcpManager.RemoveClient(serverName); err != nil {
		e.eventBus.Publish(NewEvent(EventError,
			fmt.Errorf("failed to disconnect: %w", err), SourceEngine))
		return
	}
	
	e.eventBus.Publish(NewEvent(EventToolOutput,
		fmt.Sprintf("Disconnected from MCP server '%s'.\n", serverName),
		SourceEngine))
}