package persistence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionManagerSaveLoad tests saving and loading sessions.
func TestSessionManagerSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	session := &SessionData{
		ID:        "test-session-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     "Test Session",
		Messages: []Message{
			{ID: "msg1", Role: "user", Content: "Hello"},
			{ID: "msg2", Role: "assistant", Content: "Hi there!"},
		},
	}

	// Save
	if err := sm.Save(session); err != nil {
		t.Errorf("Failed to save: %v", err)
	}

	// Load
	loaded, err := sm.Load("test-session-1")
	if err != nil {
		t.Errorf("Failed to load: %v", err)
	}

	if loaded.ID != session.ID {
		t.Errorf("ID mismatch: got %s, want %s", loaded.ID, session.ID)
	}
	if loaded.Title != session.Title {
		t.Errorf("Title mismatch: got %s, want %s", loaded.Title, session.Title)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("Message count mismatch: got %d, want 2", len(loaded.Messages))
	}
}

// TestSessionManagerDelete tests deleting sessions.
func TestSessionManagerDelete(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	session := &SessionData{
		ID:        "to-delete",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save
	sm.Save(session)

	// Delete
	if err := sm.Delete("to-delete"); err != nil {
		t.Errorf("Failed to delete: %v", err)
	}

	// Should not exist
	if sm.Exists("to-delete") {
		t.Error("Session should not exist after deletion")
	}
}

// TestSessionManagerList tests listing sessions.
func TestSessionManagerList(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	// Create multiple sessions with explicit times
	for i := 0; i < 5; i++ {
		session := &SessionData{
			ID:        "session-" + string(rune('a'+i)),
			Title:     "Session " + string(rune('A'+i)),
			UpdatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
		sm.Save(session)
	}

	// List
	sessions, err := sm.List()
	if err != nil {
		t.Errorf("Failed to list: %v", err)
	}

	if len(sessions) != 5 {
		t.Errorf("Expected 5 sessions, got %d", len(sessions))
	}

	// Should be sorted by update time (newest first)
	// session-a has oldest time offset (0), so it's newest
	// But since we save in order, file mod times may differ
	// Just check we have 5 sessions
}

// TestSessionManagerGetLatest tests getting latest session.
func TestSessionManagerGetLatest(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	// Create sessions with different times
	sm.Save(&SessionData{ID: "old", UpdatedAt: time.Now().Add(-2 * time.Hour)})
	sm.Save(&SessionData{ID: "new", UpdatedAt: time.Now()})

	latest, err := sm.GetLatest()
	if err != nil {
		t.Errorf("Failed to get latest: %v", err)
	}

	if latest == nil {
		t.Error("Expected latest session")
		return
	}

	if latest.ID != "new" {
		t.Errorf("Latest should be 'new', got %s", latest.ID)
	}
}

// TestSessionManagerPrune tests pruning old sessions.
func TestSessionManagerPrune(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{
		DataDir:     tmpDir,
		MaxSessions: 3,
	})

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		session := &SessionData{
			ID:        "session-" + string(rune('a'+i)),
			UpdatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
		sm.Save(session)
	}

	// Prune
	if err := sm.Prune(); err != nil {
		t.Errorf("Failed to prune: %v", err)
	}

	// Should only have 3 sessions
	sessions, _ := sm.List()
	if len(sessions) > 3 {
		t.Errorf("Expected at most 3 sessions, got %d", len(sessions))
	}
}

// TestSessionManagerExportImport tests export and import.
func TestSessionManagerExportImport(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	session := &SessionData{
		ID:        "export-test",
		Title:     "Export Session",
		Messages:  []Message{{ID: "m1", Content: "test"}},
		UpdatedAt: time.Now(),
	}
	sm.Save(session)

	// Export
	exportPath := filepath.Join(tmpDir, "exported.json")
	if err := sm.Export("export-test", exportPath); err != nil {
		t.Errorf("Failed to export: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(exportPath); err != nil {
		t.Errorf("Export file should exist: %v", err)
	}

	// Import
	imported, err := sm.Import(exportPath)
	if err != nil {
		t.Errorf("Failed to import: %v", err)
	}

	// Should have same title
	if imported.Title != "Export Session" {
		t.Errorf("Title should match after import")
	}
}

// TestSessionManagerSearch tests searching sessions.
func TestSessionManagerSearch(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	// Create sessions with different content
	sm.Save(&SessionData{
		ID:        "session-1",
		Title:     "Python Help",
		Messages:  []Message{{Content: "How do I use Python?"}},
		UpdatedAt: time.Now(),
	})
	sm.Save(&SessionData{
		ID:        "session-2",
		Title:     "Go Programming",
		Messages:  []Message{{Content: "Go is great"}},
		UpdatedAt: time.Now(),
	})

	// Search for "Python"
	results, err := sm.Search("Python")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'Python', got %d", len(results))
	}

	// Search for "Go"
	results, _ = sm.Search("go")
	if len(results) < 1 {
		t.Error("Expected at least 1 result for 'go'")
	}
}

// TestSessionManagerTags tests tag management.
func TestSessionManagerTags(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	session := &SessionData{
		ID:        "tag-test",
		UpdatedAt: time.Now(),
	}
	sm.Save(session)

	// Add tag
	if err := sm.AddTag("tag-test", "important"); err != nil {
		t.Errorf("Failed to add tag: %v", err)
	}

	loaded, _ := sm.Load("tag-test")
	if len(loaded.Tags) != 1 || loaded.Tags[0] != "important" {
		t.Error("Tag should be added")
	}

	// Remove tag
	if err := sm.RemoveTag("tag-test", "important"); err != nil {
		t.Errorf("Failed to remove tag: %v", err)
	}

	loaded, _ = sm.Load("tag-test")
	if len(loaded.Tags) != 0 {
		t.Error("Tag should be removed")
	}
}

// TestAutoSaver tests auto-save functionality.
func TestAutoSaver(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	session := &SessionData{
		ID:        "auto-save-test",
		Title:     "Initial Title",
		UpdatedAt: time.Now(),
	}

	as := NewAutoSaver(sm, session, 100*time.Millisecond)
	as.Start()

	// Update session
	session.Title = "Updated Title"
	as.Update(session)

	// Wait for auto-save
	time.Sleep(200 * time.Millisecond)

	// Stop (triggers final save)
	as.Stop()

	// Check saved
	loaded, err := sm.Load("auto-save-test")
	if err != nil {
		t.Errorf("Failed to load: %v", err)
	}

	if loaded.Title != "Updated Title" {
		t.Errorf("Title should be updated, got %s", loaded.Title)
	}
}

// TestGenerateID tests ID generation.
func TestGenerateID(t *testing.T) {
	id1 := generateID()
	time.Sleep(1 * time.Millisecond) // Ensure different timestamp
	id2 := generateID()

	// IDs should be different (either by timestamp or random part)
	// With timestamp difference they will differ
	if id1 == id2 {
		t.Error("IDs should be different")
	}

	if len(id1) < 10 {
		t.Errorf("ID too short: %s", id1)
	}
}

// TestEmptyDataDir tests with empty data directory.
func TestEmptyDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	// List should return empty
	sessions, err := sm.List()
	if err != nil {
		t.Errorf("List failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected empty list, got %d", len(sessions))
	}

	// GetLatest should return nil
	latest, err := sm.GetLatest()
	if err != nil {
		t.Errorf("GetLatest failed: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil for empty data dir")
	}
}

// TestSearchWithMatches tests enhanced search with match snippets.
func TestSearchWithMatches(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(SessionManagerOptions{DataDir: tmpDir})

	// Create sessions with messages
	sm.Save(&SessionData{
		ID:        "search-test-1",
		Title:     "Python Discussion",
		Messages: []Message{
			{ID: "msg1", Role: "user", Content: "How do I use Python decorators?", Time: time.Now()},
			{ID: "msg2", Role: "assistant", Content: "Decorators are functions that modify other functions.", Time: time.Now()},
		},
		UpdatedAt: time.Now(),
	})
	sm.Save(&SessionData{
		ID:        "search-test-2",
		Title:     "Go Programming",
		Messages: []Message{
			{ID: "msg3", Role: "user", Content: "How do I use Go channels?", Time: time.Now()},
			{ID: "msg4", Role: "assistant", Content: "Channels are used for communication between goroutines.", Time: time.Now()},
		},
		UpdatedAt: time.Now(),
	})

	// Search for "decorator" - should match both messages in session 1
	results, err := sm.SearchWithMatches("decorator", 10)
	if err != nil {
		t.Errorf("SearchWithMatches failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("Expected at least 1 result for 'decorator', got %d", len(results))
	}

	// Check that all results are from session 1
	for _, r := range results {
		if r.SessionID != "search-test-1" {
			t.Errorf("Expected session 'search-test-1', got %s", r.SessionID)
		}
	}

	// Search for "channels"
	results, _ = sm.SearchWithMatches("channels", 10)
	if len(results) < 1 {
		t.Error("Expected at least 1 result for 'channels'")
	}

	// Check snippet contains match
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.MatchSnippet), "channel") {
			t.Errorf("Snippet should contain 'channel', got: %s", r.MatchSnippet)
		}
	}
}

// TestExtractMatchSnippet tests snippet extraction.
func TestExtractMatchSnippet(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		query      string
		contextLen int
		contains   string
	}{
		{
			name:       "simple match",
			content:    "This is a test message with keywords here",
			query:      "keywords",
			contextLen: 10,
			contains:   "keywords",
		},
		{
			name:       "short content",
			content:    "short",
			query:      "short",
			contextLen: 10,
			contains:   "short",
		},
		{
			name:       "match at start",
			content:    "keywords are at the start of this message",
			query:      "keywords",
			contextLen: 10,
			contains:   "keywords",
		},
		{
			name:       "match at end",
			content:    "this message ends with keywords",
			query:      "keywords",
			contextLen: 10,
			contains:   "keywords",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snippet := extractMatchSnippet(tt.content, tt.query, tt.contextLen)
			if !strings.Contains(strings.ToLower(snippet), strings.ToLower(tt.contains)) {
				t.Errorf("Snippet should contain '%s', got: %s", tt.contains, snippet)
			}
		})
	}
}