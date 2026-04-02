// Package persistence provides session persistence for SuperTerminal.
package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SessionData represents a stored session.
type SessionData struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Title        string    `json:"title,omitempty"`
	MessageCount int       `json:"message_count"`
	Messages     []Message `json:"messages"`
	Config       Config    `json:"config,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
}

// Message represents a stored message.
type Message struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Thinking  string                 `json:"thinking,omitempty"`
	Time      time.Time              `json:"time"`
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	ToolResult string                `json:"tool_result,omitempty"`
	ToolCalls  []ToolCall            `json:"tool_calls,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCall represents a tool call in a message.
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

// Config represents session configuration.
type Config struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
}

// SessionManager manages session persistence.
type SessionManager struct {
	dataDir     string
	maxSessions int
	autoSave    bool
	mu          sync.RWMutex
}

// SessionManagerOptions configures the session manager.
type SessionManagerOptions struct {
	DataDir     string // Directory to store sessions
	MaxSessions int    // Maximum number of sessions to keep
	AutoSave    bool   // Auto-save on changes
}

// NewSessionManager creates a new session manager.
func NewSessionManager(opts SessionManagerOptions) *SessionManager {
	if opts.DataDir == "" {
		home, _ := os.UserHomeDir()
		opts.DataDir = filepath.Join(home, ".superterminal", "sessions")
	}
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 100
	}

	sm := &SessionManager{
		dataDir:     opts.DataDir,
		maxSessions: opts.MaxSessions,
		autoSave:    opts.AutoSave,
	}

	// Ensure directory exists
	os.MkdirAll(sm.dataDir, 0755)

	return sm
}

// Save saves a session to disk.
func (sm *SessionManager) Save(session *SessionData) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update timestamp
	session.UpdatedAt = time.Now()

	// Create filename
	filename := sm.sessionFile(session.ID)

	// Marshal to JSON
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first
	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	// Rename to final file (atomic)
	return os.Rename(tmpFile, filename)
}

// Load loads a session from disk.
func (sm *SessionManager) Load(id string) (*SessionData, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	filename := sm.sessionFile(id)
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// Delete deletes a session.
func (sm *SessionManager) Delete(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	filename := sm.sessionFile(id)
	return os.Remove(filename)
}

// List lists all stored sessions.
func (sm *SessionManager) List() ([]SessionMeta, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	files, err := os.ReadDir(sm.dataDir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionMeta
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(file.Name(), ".json")
		info, err := file.Info()
		if err != nil {
			continue
		}

		// Try to read session for more info
		session, err := sm.Load(id)
		if err != nil {
			// Use file info as fallback
			sessions = append(sessions, SessionMeta{
				ID:        id,
				UpdatedAt: info.ModTime(),
			})
			continue
		}

		sessions = append(sessions, SessionMeta{
			ID:           session.ID,
			CreatedAt:    session.CreatedAt,
			UpdatedAt:    session.UpdatedAt,
			Title:        session.Title,
			MessageCount: session.MessageCount,
			Tags:         session.Tags,
		})
	}

	// Sort by update time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// SessionMeta contains session metadata for listing.
type SessionMeta struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Title        string    `json:"title,omitempty"`
	MessageCount int       `json:"message_count"`
	Tags         []string  `json:"tags,omitempty"`
}

// GetLatest returns the most recent session.
func (sm *SessionManager) GetLatest() (*SessionData, error) {
	sessions, err := sm.List()
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	return sm.Load(sessions[0].ID)
}

// Exists checks if a session exists.
func (sm *SessionManager) Exists(id string) bool {
	filename := sm.sessionFile(id)
	_, err := os.Stat(filename)
	return err == nil
}

// Prune removes old sessions to stay within max limit.
func (sm *SessionManager) Prune() error {
	sessions, err := sm.List()
	if err != nil {
		return err
	}

	if len(sessions) <= sm.maxSessions {
		return nil
	}

	// Remove oldest sessions
	for i := sm.maxSessions; i < len(sessions); i++ {
		if err := sm.Delete(sessions[i].ID); err != nil {
			// Log error but continue
			continue
		}
	}

	return nil
}

// Export exports a session to a specific file.
func (sm *SessionManager) Export(id string, path string) error {
	session, err := sm.Load(id)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Import imports a session from a file.
func (sm *SessionManager) Import(path string) (*SessionData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	// Generate new ID to avoid conflicts
	if sm.Exists(session.ID) {
		session.ID = generateID()
	}

	if err := sm.Save(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

// Search searches sessions by content.
func (sm *SessionManager) Search(query string) ([]SessionMeta, error) {
	sessions, err := sm.List()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var matches []SessionMeta

	for _, meta := range sessions {
		session, err := sm.Load(meta.ID)
		if err != nil {
			continue
		}

		// Check title
		if strings.Contains(strings.ToLower(session.Title), query) {
			matches = append(matches, meta)
			continue
		}

		// Check messages
		for _, msg := range session.Messages {
			if strings.Contains(strings.ToLower(msg.Content), query) {
				matches = append(matches, meta)
				break
			}
		}
	}

	return matches, nil
}

// SetTitle sets a session's title.
func (sm *SessionManager) SetTitle(id string, title string) error {
	session, err := sm.Load(id)
	if err != nil {
		return err
	}

	session.Title = title
	return sm.Save(session)
}

// AddTag adds a tag to a session.
func (sm *SessionManager) AddTag(id string, tag string) error {
	session, err := sm.Load(id)
	if err != nil {
		return err
	}

	for _, t := range session.Tags {
		if t == tag {
			return nil // Already tagged
		}
	}

	session.Tags = append(session.Tags, tag)
	return sm.Save(session)
}

// RemoveTag removes a tag from a session.
func (sm *SessionManager) RemoveTag(id string, tag string) error {
	session, err := sm.Load(id)
	if err != nil {
		return err
	}

	for i, t := range session.Tags {
		if t == tag {
			session.Tags = append(session.Tags[:i], session.Tags[i+1:]...)
			break
		}
	}

	return sm.Save(session)
}

// sessionFile returns the file path for a session.
func (sm *SessionManager) sessionFile(id string) string {
	return filepath.Join(sm.dataDir, id+".json")
}

// generateID generates a unique session ID.
func generateID() string {
	return time.Now().Format("20060102-150405") + "-" + randomString(6)
}

// randomString generates a random string of given length.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, n)
	timestamp := time.Now().UnixNano()
	for i := range result {
		// Use timestamp and position for pseudo-randomness
		result[i] = letters[(int(timestamp)+i*7)%len(letters)]
	}
	return string(result)
}

// === Auto-save functionality ===

// AutoSaver handles automatic session saving.
type AutoSaver struct {
	manager   *SessionManager
	session   *SessionData
	interval  time.Duration
	stopCh    chan struct{}
	mu        sync.Mutex
}

// NewAutoSaver creates an auto-saver.
func NewAutoSaver(manager *SessionManager, session *SessionData, interval time.Duration) *AutoSaver {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &AutoSaver{
		manager:  manager,
		session:  session,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the auto-save loop.
func (as *AutoSaver) Start() {
	go func() {
		ticker := time.NewTicker(as.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				as.mu.Lock()
				as.manager.Save(as.session)
				as.mu.Unlock()
			case <-as.stopCh:
				return
			}
		}
	}()
}

// Stop stops the auto-save loop.
func (as *AutoSaver) Stop() {
	close(as.stopCh)
	// Final save
	as.manager.Save(as.session)
}

// Update updates the session data.
func (as *AutoSaver) Update(session *SessionData) {
	as.mu.Lock()
	as.session = session
	as.mu.Unlock()
}

// SearchResult contains a search match with context.
type SearchResult struct {
	SessionID    string    `json:"session_id"`
	SessionTitle string    `json:"session_title"`
	MessageID    string    `json:"message_id"`
	Role         string    `json:"role"`
	MatchSnippet string    `json:"match_snippet"` // Context around the match
	Time         time.Time `json:"time"`
}

// SearchWithMatches searches sessions and returns matching message snippets.
func (sm *SessionManager) SearchWithMatches(query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 20
	}

	sessions, err := sm.List()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for _, meta := range sessions {
		session, err := sm.Load(meta.ID)
		if err != nil {
			continue
		}

		// Search in messages
		for _, msg := range session.Messages {
			contentLower := strings.ToLower(msg.Content)
			if strings.Contains(contentLower, query) {
				// Extract context around the match
				snippet := extractMatchSnippet(msg.Content, query, 100)
				
				results = append(results, SearchResult{
					SessionID:    session.ID,
					SessionTitle: session.Title,
					MessageID:    msg.ID,
					Role:         msg.Role,
					MatchSnippet: snippet,
					Time:         msg.Time,
				})

				if len(results) >= maxResults {
					return results, nil
				}
			}
		}
	}

	// Sort results by time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time.After(results[j].Time)
	})

	return results, nil
}

// extractMatchSnippet extracts text around a match.
func extractMatchSnippet(content, query string, contextLen int) string {
	contentLower := strings.ToLower(content)
	queryLower := strings.ToLower(query)
	
	idx := strings.Index(contentLower, queryLower)
	if idx == -1 {
		if len(content) <= contextLen*2 {
			return content
		}
		return content[:contextLen*2] + "..."
	}

	// Calculate start and end positions
	start := idx - contextLen
	if start < 0 {
		start = 0
	}
	
	end := idx + len(query) + contextLen
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	
	// Add ellipsis if truncated
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}

	return snippet
}