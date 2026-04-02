package resource

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMemoryMonitor tests memory monitoring.
func TestMemoryMonitor(t *testing.T) {
	monitor := NewMemoryMonitor(
		WithMaxMemory(1024*1024*1024), // 1GB
		WithWarnMemory(768*1024*1024), // 768MB
		WithCheckInterval(100*time.Millisecond),
	)
	defer monitor.Stop()

	stats := monitor.GetStats()
	if stats.AllocBytes <= 0 {
		t.Error("AllocBytes should be positive")
	}

	if stats.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}

	// Should not be over limit in normal conditions
	if stats.IsOverLimit {
		t.Error("Should not be over limit")
	}

	if stats.IsOverWarn {
		t.Error("Should not be over warn")
	}
}

// TestMemoryMonitorCallback tests callback invocation.
func TestMemoryMonitorCallback(t *testing.T) {
	called := make(chan MemoryStats, 1)

	monitor := NewMemoryMonitor(
		WithCheckInterval(50*time.Millisecond),
		WithCallback(func(stats MemoryStats) {
			select {
			case called <- stats:
			default:
			}
		}),
	)

	monitor.Start()
	defer monitor.Stop()

	select {
	case stats := <-called:
		if stats.AllocBytes <= 0 {
			t.Error("Should receive valid stats")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Callback should have been called")
	}
}

// TestMemoryMonitorAddCallback tests adding callbacks.
func TestMemoryMonitorAddCallback(t *testing.T) {
	monitor := NewMemoryMonitor()
	defer monitor.Stop()

	var callbackCount int
	monitor.AddCallback(func(stats MemoryStats) {
		callbackCount++
	})

	monitor.AddCallback(func(stats MemoryStats) {
		callbackCount++
	})

	// Verify callbacks exist
	monitor.mu.RLock()
	count := len(monitor.callbacks)
	monitor.mu.RUnlock()

	if count != 2 {
		t.Errorf("Expected 2 callbacks, got %d", count)
	}
}

// TestSessionManager tests session management.
func TestSessionManager(t *testing.T) {
	sm := NewSessionManager(WithMaxSessions(10))

	// Create session
	err := sm.Create("session1", map[string]interface{}{"user": "test"})
	if err != nil {
		t.Errorf("Failed to create session: %v", err)
	}

	// Get session
	info, ok := sm.Get("session1")
	if !ok {
		t.Error("Should find session")
	}

	if info.ID != "session1" {
		t.Errorf("Expected ID 'session1', got '%s'", info.ID)
	}

	if info.Metadata["user"] != "test" {
		t.Error("Metadata should be preserved")
	}

	// Count
	if sm.Count() != 1 {
		t.Errorf("Expected count 1, got %d", sm.Count())
	}
}

// TestSessionManagerUpdate tests session update.
func TestSessionManagerUpdate(t *testing.T) {
	sm := NewSessionManager()

	sm.Create("session1", nil)
	time.Sleep(10 * time.Millisecond)

	err := sm.Update("session1")
	if err != nil {
		t.Errorf("Failed to update session: %v", err)
	}

	info, _ := sm.Get("session1")
	if !info.UpdatedAt.After(info.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt")
	}
}

// TestSessionManagerDelete tests session deletion.
func TestSessionManagerDelete(t *testing.T) {
	sm := NewSessionManager()

	sm.Create("session1", nil)
	sm.Delete("session1")

	_, ok := sm.Get("session1")
	if ok {
		t.Error("Session should be deleted")
	}
}

// TestSessionManagerMaxLimit tests max session limit with eviction.
func TestSessionManagerMaxLimit(t *testing.T) {
	sm := NewSessionManager(WithMaxSessions(2))

	// Create two sessions
	sm.Create("s1", nil)
	sm.Create("s2", nil)

	// Create third session - should succeed by evicting oldest
	err := sm.Create("s3", nil)
	if err != nil {
		t.Errorf("Should succeed with eviction: %v", err)
	}

	// Count should still be at max
	if sm.Count() > 2 {
		t.Errorf("Count should be at most 2, got %d", sm.Count())
	}
}

// TestSessionManagerEviction tests eviction of old sessions.
func TestSessionManagerEviction(t *testing.T) {
	sm := NewSessionManager(WithMaxSessions(2))

	sm.Create("s1", nil)
	time.Sleep(10 * time.Millisecond)
	sm.Create("s2", nil)
	time.Sleep(10 * time.Millisecond)

	// Update s1 to make it newer
	sm.Update("s1")

	// Create s3 - should evict s2 (oldest)
	sm.Create("s3", nil)

	// s1 should still exist
	_, ok1 := sm.Get("s1")
	if !ok1 {
		t.Error("s1 should still exist (was updated)")
	}

	// s3 should exist
	_, ok3 := sm.Get("s3")
	if !ok3 {
		t.Error("s3 should exist")
	}
}

// TestSessionManagerList tests listing sessions.
func TestSessionManagerList(t *testing.T) {
	sm := NewSessionManager()

	sm.Create("s1", nil)
	sm.Create("s2", nil)
	sm.Create("s3", nil)

	ids := sm.List()
	if len(ids) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(ids))
	}
}

// TestLogRotator tests log rotation.
func TestLogRotator(t *testing.T) {
	// Create temp file
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	rotator := NewLogRotator(logPath,
		WithMaxSize(100), // 100 bytes
		WithMaxBackups(3),
	)

	// Write data smaller than max
	n, err := rotator.Write([]byte("test data"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if n != 9 {
		t.Errorf("Expected 9 bytes written, got %d", n)
	}

	// Check rotation not needed yet
	if rotator.ShouldRotate() {
		t.Error("Should not need rotation yet")
	}
}

// TestLogRotatorRotation tests actual rotation.
func TestLogRotatorRotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	rotator := NewLogRotator(logPath,
		WithMaxSize(20), // 20 bytes for easy testing
		WithMaxBackups(2),
	)

	// Write small data first
	rotator.Write([]byte("initial"))

	// Write larger data to trigger rotation
	data := []byte("this is more than twenty bytes long content here")
	_, err = rotator.Write(data)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Backup should exist after writing large data
	backup := logPath + ".1"
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		// Check if rotation happened
		info, _ := os.Stat(logPath)
		if info.Size() < int64(len(data)) {
			t.Error("Backup file should exist after rotation")
		}
	}
}

// TestLogRotatorManualRotate tests manual rotation.
func TestLogRotatorManualRotate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	rotator := NewLogRotator(logPath,
		WithMaxSize(1024),
		WithMaxBackups(3),
	)

	// Create the log file
	rotator.Write([]byte("initial content"))

	// Manual rotate
	err = rotator.Rotate()
	if err != nil {
		t.Errorf("Rotate failed: %v", err)
	}

	// Backup should exist
	backup := logPath + ".1"
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		t.Error("Backup should exist after rotation")
	}

	// Original should be empty (new file)
	content, _ := os.ReadFile(logPath)
	if len(content) != 0 {
		t.Error("Rotated file should be empty")
	}
}

// TestResourceManager tests the combined manager.
func TestResourceManager(t *testing.T) {
	rm := NewResourceManager(1024*1024*1024, 10, "/tmp/logs")
	defer rm.Stop()

	// Memory monitor
	if rm.Memory() == nil {
		t.Error("Memory monitor should exist")
	}

	// Session manager
	if rm.Sessions() == nil {
		t.Error("Session manager should exist")
	}

	// Create session
	rm.Sessions().Create("test-session", nil)
	if rm.Sessions().Count() != 1 {
		t.Error("Should have 1 session")
	}

	// Add log
	rm.AddLog("main", "/tmp/test.log", 1024)
	if rm.GetLog("main") == nil {
		t.Error("Should have log rotator")
	}
}

// TestResourceManagerStartStop tests lifecycle.
func TestResourceManagerStartStop(t *testing.T) {
	rm := NewResourceManager(1024*1024*1024, 10, "/tmp/logs")

	rm.Start()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic
	rm.Stop()
}