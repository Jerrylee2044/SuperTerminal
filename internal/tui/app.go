// Package tui provides the Bubble Tea terminal UI for SuperTerminal.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"

	"superterminal/internal/engine"
)

// Styles
var (
	// Colors
	colorPrimary    = lipgloss.Color("#7C3AED")  // Purple
	colorSecondary  = lipgloss.Color("#10B981")  // Green
	colorAccent     = lipgloss.Color("#F59E0B")  // Orange
	colorError      = lipgloss.Color("#EF4444")  // Red
	colorMuted      = lipgloss.Color("#6B7280")  // Gray
	colorBackground = lipgloss.Color("#1F2937")  // Dark gray

	// Text styles
	styleTitle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Padding(0, 1)

	styleUserMessage = lipgloss.NewStyle().
		Foreground(colorSecondary).
		Padding(0, 1)

	styleAssistantMessage = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Padding(0, 1)

	styleSystemMessage = lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	styleMuted = lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	styleError = lipgloss.NewStyle().
		Foreground(colorError).
		Padding(0, 1)

	styleInputBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(colorBackground).
		Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	// Spinner colors
	spinnerStyle = lipgloss.NewStyle().
		Foreground(colorPrimary)
)

// Model represents the TUI state.
type Model struct {
	// Core
	engine   *engine.Engine
	eventBus *engine.EventBus
	eventCh  chan engine.Event

	// UI State
	width        int
	height       int
	input        textinput.Model
	messages     []MessageView
	spinner      spinner.Model
	status       string
	cost         engine.CostInfo
	currentTool  *ToolView
	tasks        []TaskView
	helpVisible  bool
	errorMsg     string
	isThinking   bool

	// Input state
	inputFocused bool

	// Command history
	history      []string
	historyIdx   int
	historySaved string // Saved current input when browsing history

	// Multiline input
	multiline    bool
	multilineBuf []string

	// Autocomplete
	suggestions  []string
	suggestIdx   int
	commands     []string // Available commands

	// Progress indicator
	progress     float64
	progressText string

	// Confirmation dialog
	confirmVisible bool
	confirmMessage string
	confirmCallback func(bool)
}

// MessageView represents a message for display.
type MessageView struct {
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	Time        time.Time `json:"time"`
	IsStreaming bool      `json:"is_streaming"`
}

// ToolView represents a tool execution for display.
type ToolView struct {
	Name      string `json:"name"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	IsRunning bool   `json:"is_running"`
}

// TaskView represents a background task for display.
type TaskView struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// NewModel creates a new TUI Model.
func NewModel(e *engine.Engine) Model {
	// Setup input
	ti := textinput.New()
	ti.Placeholder = "Type a message or /help for commands..."
	ti.Focus()
	ti.CharLimit = 5000
	ti.Width = 80

	// Setup spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	// Available commands for autocomplete
	commands := []string{
		"/help", "/exit", "/clear", "/reset", "/sessions", "/load",
		"/save", "/export", "/search", "/mcp", "/mcp list",
		"/mcp tools", "/mcp resources", "/mcp prompts",
		"/mcp connect", "/mcp disconnect",
		"/permission", "/config", "/version", "/cost",
	}

	return Model{
		engine:       e,
		eventBus:     e.GetEventBus(),
		input:        ti,
		spinner:      s,
		status:       "Ready",
		inputFocused: true,
		messages:     []MessageView{},
		tasks:        []TaskView{},
		isThinking:   false,
		history:      []string{},
		historyIdx:   -1,
		multilineBuf: []string{},
		commands:     commands,
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	// Subscribe to events
	m.eventCh = m.eventBus.SubscribeAll()
	
	return tea.Batch(
		m.spinner.Tick,
		m.waitForEvent(),
	)
}

// Update handles messages and events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch typedMsg := msg.(type) {
	// Window resize
	case tea.WindowSizeMsg:
		m.width = typedMsg.Width
		m.height = typedMsg.Height
		m.input.Width = typedMsg.Width - 4
		return m, nil

	// Key events
	case tea.KeyMsg:
		// Handle confirmation dialog first
		if m.confirmVisible {
			switch typedMsg.String() {
			case "y", "Y":
				m.confirmVisible = false
				if m.confirmCallback != nil {
					m.confirmCallback(true)
				}
				return m, nil
			case "n", "N", "esc":
				m.confirmVisible = false
				if m.confirmCallback != nil {
					m.confirmCallback(false)
				}
				return m, nil
			case "enter":
				m.confirmVisible = false
				if m.confirmCallback != nil {
					m.confirmCallback(true)
				}
				return m, nil
			}
			return m, nil
		}

		switch typedMsg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.currentTool != nil && m.currentTool.IsRunning {
				// Cancel current operation
				m.status = "Cancelling..."
				return m, nil
			}
			if m.multiline {
				// Exit multiline mode
				m.multiline = false
				m.multilineBuf = []string{}
				m.status = "Ready"
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.multiline {
				// Add line to multiline buffer
				m.multilineBuf = append(m.multilineBuf, m.input.Value())
				m.input.SetValue("")
				m.status = fmt.Sprintf("Multiline (%d lines)", len(m.multilineBuf))
				return m, nil
			}
			if m.input.Value() != "" {
				// Send input to engine
				input := m.input.Value()
				m.input.SetValue("")
				
				// Add to history
				m.addToHistory(input)
				
				// Add to messages
				m.messages = append(m.messages, MessageView{
					Role:    "user",
					Content: input,
					Time:    time.Now(),
				})
				
				// Process in engine
				go m.engine.ProcessInput(input, engine.SourceTUI)
				
				m.status = "Thinking..."
				m.isThinking = true
				m.clearSuggestions()
				return m, m.spinner.Tick
			}

		case tea.KeyUp:
			// History navigation - go back
			if len(m.history) > 0 {
				if m.historyIdx == -1 {
					m.historySaved = m.input.Value()
					m.historyIdx = len(m.history) - 1
				} else if m.historyIdx > 0 {
					m.historyIdx--
				}
				m.input.SetValue(m.history[m.historyIdx])
			}
			return m, nil

		case tea.KeyDown:
			// History navigation - go forward
			if m.historyIdx >= 0 {
				if m.historyIdx < len(m.history)-1 {
					m.historyIdx++
					m.input.SetValue(m.history[m.historyIdx])
				} else {
					m.historyIdx = -1
					m.input.SetValue(m.historySaved)
				}
			}
			return m, nil

		case tea.KeyTab:
			// Autocomplete
			m.updateSuggestions()
			if len(m.suggestions) > 0 {
				m.suggestIdx = (m.suggestIdx + 1) % len(m.suggestions)
				m.input.SetValue(m.suggestions[m.suggestIdx])
			}
			return m, nil

		case tea.KeyShiftTab:
			// Reverse autocomplete
			if len(m.suggestions) > 0 {
				m.suggestIdx--
				if m.suggestIdx < 0 {
					m.suggestIdx = len(m.suggestions) - 1
				}
				m.input.SetValue(m.suggestions[m.suggestIdx])
			}
			return m, nil

		case tea.KeyCtrlO:
			// Toggle multiline mode (use Ctrl+O to avoid Enter/Ctrl+M conflict)
			m.multiline = !m.multiline
			if m.multiline {
				m.status = "Multiline mode (Ctrl+O to finish, Esc to cancel)"
			} else {
				// Send multiline content
				if len(m.multilineBuf) > 0 {
					content := strings.Join(m.multilineBuf, "\n")
					m.addToHistory(content)
					m.messages = append(m.messages, MessageView{
						Role:    "user",
						Content: content,
						Time:    time.Now(),
					})
					go m.engine.ProcessInput(content, engine.SourceTUI)
					m.status = "Thinking..."
					m.isThinking = true
				}
				m.multilineBuf = []string{}
			}
			return m, nil

		case tea.KeyCtrlH:
			m.helpVisible = !m.helpVisible
			return m, nil

		case tea.KeyCtrlL:
			// Clear screen
			m.messages = []MessageView{}
			m.errorMsg = ""
			m.clearSuggestions()
			return m, nil

		case tea.KeyCtrlP:
			// Toggle progress indicator
			if m.progress > 0 {
				m.progress = 0
				m.progressText = ""
			}
			return m, nil
		}

	// Spinner tick
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	// Engine events
	case engine.Event:
		var shouldExit bool
		m, shouldExit = m.handleEngineEvent(typedMsg)
		if shouldExit {
			return m, tea.Quit
		}

	// Custom event message (from waitForEvent)
	case EventMsg:
		var shouldExit bool
		m, shouldExit = m.handleEngineEvent(typedMsg.Event)
		if shouldExit {
			return m, tea.Quit
		}
		cmds = append(cmds, m.waitForEvent())
	}

	// Update text input for key events only
	if _, ok := msg.(tea.KeyMsg); ok && !m.confirmVisible {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		// Update suggestions on input change
		if m.input.Value() != "" && strings.HasPrefix(m.input.Value(), "/") {
			m.updateSuggestions()
		} else {
			m.clearSuggestions()
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// addToHistory adds input to command history.
func (m *Model) addToHistory(input string) {
	// Don't add empty or duplicate consecutive entries
	if input == "" || (len(m.history) > 0 && m.history[len(m.history)-1] == input) {
		return
	}
	m.history = append(m.history, input)
	// Limit history size
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	m.historyIdx = -1
}

// updateSuggestions updates autocomplete suggestions.
func (m *Model) updateSuggestions() {
	input := m.input.Value()
	if input == "" {
		m.clearSuggestions()
		return
	}

	m.suggestions = []string{}
	for _, cmd := range m.commands {
		if strings.HasPrefix(cmd, input) {
			m.suggestions = append(m.suggestions, cmd)
		}
	}

	// Reset index if suggestions changed
	if m.suggestIdx >= len(m.suggestions) {
		m.suggestIdx = 0
	}
}

// clearSuggestions clears autocomplete suggestions.
func (m *Model) clearSuggestions() {
	m.suggestions = []string{}
	m.suggestIdx = 0
}

// ShowConfirm shows a confirmation dialog.
func (m *Model) ShowConfirm(message string, callback func(bool)) {
	m.confirmVisible = true
	m.confirmMessage = message
	m.confirmCallback = callback
}

// SetProgress sets the progress indicator.
func (m *Model) SetProgress(progress float64, text string) {
	m.progress = progress
	m.progressText = text
}

// handleEngineEvent processes an event from the engine.
// Returns the updated model and a boolean indicating if the TUI should exit.
func (m Model) handleEngineEvent(event engine.Event) (Model, bool) {
	switch event.Type {
	case engine.EventUserInput:
		// User input already added locally

	case engine.EventAPIStream:
		// Streaming response - handle StreamProgress type
		switch data := event.Data.(type) {
		case engine.StreamProgress:
			if data.Type == "text" {
				// Append to last assistant message or create new
				if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" && m.messages[len(m.messages)-1].IsStreaming {
					m.messages[len(m.messages)-1].Content += data.Text
				} else {
					m.messages = append(m.messages, MessageView{
						Role:        "assistant",
						Content:     data.Text,
						Time:        time.Now(),
						IsStreaming: true,
					})
				}
			} else if data.Type == "thinking" {
				// Thinking content - could show differently
				// For now, just continue
			}
		case string:
			// Legacy string format
			m.messages[len(m.messages)-1].Content += data
		}

	case engine.EventAPIComplete:
		// Mark last message as complete
		if len(m.messages) > 0 {
			m.messages[len(m.messages)-1].IsStreaming = false
		}
		m.status = "Ready"
		m.isThinking = false

	case engine.EventAPIError:
		if err, ok := event.Data.(error); ok {
			m.errorMsg = err.Error()
			m.status = "Error"
			m.isThinking = false
		}

	case engine.EventToolStart:
		if info, ok := event.Data.(engine.ToolInfo); ok {
			m.currentTool = &ToolView{
				Name:      info.Name,
				Input:     info.Input,
				IsRunning: true,
			}
			m.status = fmt.Sprintf("Running: %s", info.Name)
		}

	case engine.EventToolOutput:
		if output, ok := event.Data.(string); ok {
			if m.currentTool != nil {
				m.currentTool.Output = output
			}
		}

	case engine.EventToolComplete:
		if m.currentTool != nil {
			m.currentTool.IsRunning = false
		}
		m.status = "Ready"

	case engine.EventCostUpdate:
		if cost, ok := event.Data.(engine.CostInfo); ok {
			m.cost = cost
		}

	case engine.EventStatusChange:
		if status, ok := event.Data.(engine.EngineStatus); ok {
			m.status = string(status)
		}

	case engine.EventError:
		if err, ok := event.Data.(error); ok {
			// Check for exit request
			if err.Error() == "exit requested" {
				return m, true
			}
			m.errorMsg = err.Error()
		}
	}

	return m, false
}

// View renders the TUI.
func (m Model) View() string {
	var b strings.Builder

	// Confirmation dialog (if visible)
	if m.confirmVisible {
		return m.renderConfirmDialog()
	}

	// Title
	b.WriteString(m.renderTitle())
	b.WriteString("\n\n")

	// Progress indicator (if active)
	if m.progress > 0 {
		b.WriteString(m.renderProgress())
		b.WriteString("\n")
	}

	// Messages area (main content)
	b.WriteString(m.renderMessages())
	b.WriteString("\n")

	// Current tool (if running)
	if m.currentTool != nil {
		b.WriteString(m.renderTool())
		b.WriteString("\n")
	}

	// Error (if any)
	if m.errorMsg != "" {
		b.WriteString(styleError.Render("Error: " + m.errorMsg))
		b.WriteString("\n")
	}

	// Status bar
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Autocomplete suggestions (if any)
	if len(m.suggestions) > 0 {
		b.WriteString(m.renderSuggestions())
		b.WriteString("\n")
	}

	// Input box with multiline indicator
	b.WriteString(m.renderInput())

	// Help panel (if visible)
	if m.helpVisible {
		b.WriteString("\n")
		b.WriteString(m.renderHelp())
	}

	return b.String()
}

// renderConfirmDialog renders a confirmation dialog.
func (m Model) renderConfirmDialog() string {
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(50)

	content := fmt.Sprintf("\n  %s\n\n  [Y] Yes  [N] No (or Esc)\n", m.confirmMessage)
	return dialogStyle.Render(content)
}

// renderProgress renders a progress indicator.
func (m Model) renderProgress() string {
	progressStyle := lipgloss.NewStyle().
		Foreground(colorSecondary).
		Padding(0, 1)

	// Create progress bar
	barWidth := 20
	filled := int(m.progress * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	text := fmt.Sprintf("[%s] %.0f%% %s", bar, m.progress*100, m.progressText)
	return progressStyle.Render(text)
}

// renderSuggestions renders autocomplete suggestions.
func (m Model) renderSuggestions() string {
	suggestStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	// Show top 5 suggestions
	var items []string
	for i, s := range m.suggestions {
		if i >= 5 {
			break
		}
		if i == m.suggestIdx {
			// Highlight current selection
			items = append(items, lipgloss.NewStyle().
				Foreground(colorPrimary).
				Render("→ " + s))
		} else {
			items = append(items, "  "+s)
		}
	}

	return suggestStyle.Render(strings.Join(items, "  "))
}

// renderTitle renders the title bar.
func (m Model) renderTitle() string {
	title := "⚡ SuperTerminal"
	if m.isThinking {
		title += " " + m.spinner.View()
	}
	if m.multiline {
		title += " [Multiline]"
	}
	return styleTitle.Render(title)
}

// renderMessages renders the messages area.
func (m Model) renderMessages() string {
	if len(m.messages) == 0 {
		return styleMuted.Render("No messages yet. Type something to start!")
	}

	var b strings.Builder
	
	// Calculate available height for messages
	// Reserve space for: title (3), status (2), input (3), tool/error (2 each)
	reservedLines := 10
	if m.currentTool != nil {
		reservedLines += 3
	}
	if m.errorMsg != "" {
		reservedLines += 3
	}
	if len(m.suggestions) > 0 {
		reservedLines += 2
	}
	
	availableHeight := m.height - reservedLines
	if availableHeight < 5 {
		availableHeight = 5
	}
	
	// Calculate which messages to show based on available height
	startIdx := 0
	
	// Estimate lines per message (rough estimate)
	linesUsed := 0
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		msgLines := (len(msg.Content) / (m.width - 10)) + 2 // Estimate wrapping
		if linesUsed + msgLines > availableHeight {
			startIdx = i + 1
			break
		}
		linesUsed += msgLines
	}

	for i := startIdx; i < len(m.messages); i++ {
		msg := m.messages[i]
		b.WriteString(m.renderMessage(msg))
		b.WriteString("\n")
	}

	return b.String()
}

// renderMessage renders a single message.
func (m Model) renderMessage(msg MessageView) string {
	var style lipgloss.Style
	var prefix string

	switch msg.Role {
	case "user":
		style = styleUserMessage
		prefix = "You: "
	case "assistant":
		style = styleAssistantMessage
		prefix = "Assistant: "
	default:
		style = styleSystemMessage
		prefix = "System: "
	}

	content := msg.Content
	if msg.IsStreaming {
		content += "..." // Cursor indicator for streaming
	}

	// Truncate if too long
	if len(content) > 500 {
		content = content[:500] + "..."
	}

	return style.Render(prefix + content)
}

// renderTool renders the current tool execution.
func (m Model) renderTool() string {
	if m.currentTool == nil {
		return ""
	}

	toolStyle := lipgloss.NewStyle().
		Foreground(colorAccent).
		Padding(0, 1)

	if m.currentTool.IsRunning {
		return toolStyle.Render(fmt.Sprintf("Tool: %s (running)...", m.currentTool.Name))
	}
	return toolStyle.Render(fmt.Sprintf("Tool: %s: %s", m.currentTool.Name, m.currentTool.Output))
}

// renderStatusBar renders the status bar.
func (m Model) renderStatusBar() string {
	var parts []string

	// Status
	parts = append(parts, fmt.Sprintf("Status: %s", m.status))

	// Session ID
	sessionID := m.engine.GetSession().ID
	if len(sessionID) > 8 {
		sessionID = sessionID[:8]
	}
	parts = append(parts, fmt.Sprintf("Session: %s", sessionID))

	// Cost
	if m.cost.TotalCost > 0 {
		parts = append(parts, fmt.Sprintf("Cost: $%.4f", m.cost.TotalCost))
	}

	// Tokens
	if m.cost.InputTokens > 0 || m.cost.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("Tokens: %d in / %d out", m.cost.InputTokens, m.cost.OutputTokens))
	}

	return styleStatusBar.Render(strings.Join(parts, " | "))
}

// renderInput renders the input box.
func (m Model) renderInput() string {
	return styleInputBox.Render(m.input.View())
}

// renderHelp renders the help panel.
func (m Model) renderHelp() string {
	helpText := `
Keyboard Shortcuts:
  Enter        Send message
  ↑/↓          Command history navigation
  Tab          Autocomplete commands
  Ctrl+C/Esc   Cancel operation or exit
  Ctrl+H       Toggle this help panel
  Ctrl+L       Clear screen
  Ctrl+M       Toggle multiline input mode
  Ctrl+P       Toggle progress indicator

Commands:
  /help        Show available commands
  /clear       Clear session
  /model       Set or show model
  /cost        Show cost statistics
  /status      Show engine status
  /sessions    List saved sessions
  /load <id>   Load a session
  /save        Save current session
  /search <q>  Search sessions
  /mcp         MCP status and commands
`
	return styleHelp.Render(helpText)
}

// EventMsg wraps an engine event for Bubble Tea.
type EventMsg struct {
	Event engine.Event
}

// ExitMsg signals the TUI to exit.
type ExitMsg struct{}

// waitForEvent returns a Cmd that waits for the next engine event.
func (m Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		if m.eventCh == nil {
			return nil
		}
		event, ok := <-m.eventCh
		if !ok {
			return nil // Channel closed
		}
		return EventMsg{Event: event}
	}
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}