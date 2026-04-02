package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoggerLevels tests log level filtering.
func TestLoggerLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(Options{
		Name:   "test",
		Level:  LevelInfo,
		Output: buf,
	})

	// Debug should be filtered
	l.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("Debug should be filtered when level is Info")
	}

	// Info should pass
	l.Info("info message")
	if buf.Len() == 0 {
		t.Error("Info should pass")
	}
	if !strings.Contains(buf.String(), "info message") {
		t.Error("Log should contain 'info message'")
	}

	// Reset buffer
	buf.Reset()

	// Warn should pass
	l.Warn("warn message")
	if !strings.Contains(buf.String(), "WARN") {
		t.Error("Log should contain WARN level")
	}

	// Reset buffer
	buf.Reset()

	// Error should pass
	l.Error("error message")
	if !strings.Contains(buf.String(), "ERROR") {
		t.Error("Log should contain ERROR level")
	}
}

// TestLoggerFields tests field formatting.
func TestLoggerFields(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(Options{
		Name:   "test",
		Level:  LevelInfo,
		Output: buf,
	})

	l.Info("test", F("key1", "value1"), F("key2", 123))

	output := buf.String()
	if !strings.Contains(output, "key1=value1") {
		t.Error("Log should contain key1=value1")
	}
	if !strings.Contains(output, "key2=123") {
		t.Error("Log should contain key2=123")
	}
}

// TestLoggerWithFields tests field inheritance.
func TestLoggerWithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(Options{
		Name:   "test",
		Level:  LevelInfo,
		Output: buf,
	})

	l2 := l.WithFields(F("base", "field"))
	l2.Info("test", F("extra", "value"))

	output := buf.String()
	if !strings.Contains(output, "base=field") {
		t.Error("Log should contain base field")
	}
	if !strings.Contains(output, "extra=value") {
		t.Error("Log should contain extra field")
	}
}

// TestLoggerFileOutput tests file logging.
func TestLoggerFileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	buf := &bytes.Buffer{}
	l := NewLogger(Options{
		Name:   "test",
		Level:  LevelInfo,
		Output: buf,
		File:   logFile,
	})

	l.Info("file test")

	// Check file exists
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("Log file should exist: %v", err)
	}

	// Check file content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "file test") {
		t.Error("File should contain log message")
	}

	l.Close()
}

// TestAuditLog tests audit logging.
func TestAuditLog(t *testing.T) {
	tmpDir := t.TempDir()
	auditFile := filepath.Join(tmpDir, "audit.log")

	l := NewLogger(Options{
		Name:      "test",
		Level:     LevelInfo,
		Output:    &bytes.Buffer{},
		AuditFile: auditFile,
	})

	l.Audit("test_action", "test_actor", "test_target", "success")

	// Check file content
	content, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !strings.Contains(output, "action=test_action") {
		t.Error("Audit should contain action")
	}
	if !strings.Contains(output, "actor=test_actor") {
		t.Error("Audit should contain actor")
	}
	if !strings.Contains(output, "target=test_target") {
		t.Error("Audit should contain target")
	}
	if !strings.Contains(output, "result=success") {
		t.Error("Audit should contain result")
	}

	l.Close()
}

// TestAuditToolCall tests tool call audit.
func TestAuditToolCall(t *testing.T) {
	tmpDir := t.TempDir()
	auditFile := filepath.Join(tmpDir, "audit.log")

	l := NewLogger(Options{
		Name:      "test",
		Level:     LevelInfo,
		Output:    &bytes.Buffer{},
		AuditFile: auditFile,
	})

	l.AuditToolCall("bash", "ls -la", true, 150)

	content, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !strings.Contains(output, "tool_call") {
		t.Error("Audit should contain tool_call action")
	}
	if !strings.Contains(output, "bash") {
		t.Error("Audit should contain tool name")
	}

	l.Close()
}

// TestAuditAPIRequest tests API request audit.
func TestAuditAPIRequest(t *testing.T) {
	tmpDir := t.TempDir()
	auditFile := filepath.Join(tmpDir, "audit.log")

	l := NewLogger(Options{
		Name:      "test",
		Level:     LevelInfo,
		Output:    &bytes.Buffer{},
		AuditFile: auditFile,
	})

	l.AuditAPIRequest("claude-sonnet-4", 1000, true, 0.01)

	content, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !strings.Contains(output, "api_request") {
		t.Error("Audit should contain api_request action")
	}
	if !strings.Contains(output, "claude-sonnet-4") {
		t.Error("Audit should contain model name")
	}

	l.Close()
}

// TestLevelString tests level names.
func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}

// TestGlobalFunctions tests global logger functions.
func TestGlobalFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	Init(Options{
		Name:   "global",
		Level:  LevelInfo,
		Output: buf,
	})

	Info("global info")
	if !strings.Contains(buf.String(), "global info") {
		t.Error("Global Info should work")
	}

	buf.Reset()
	Debug("global debug")
	if buf.Len() > 0 {
		t.Error("Global Debug should be filtered")
	}

	Close()
}

// TestFormattedLogging tests formatted logging.
func TestLoggerFormatted(t *testing.T) {
	buf := &bytes.Buffer{}
	l := NewLogger(Options{
		Name:   "test",
		Level:  LevelInfo,
		Output: buf,
	})

	l.Infof("formatted %s with %d", "message", 42)
	if !strings.Contains(buf.String(), "formatted message with 42") {
		t.Error("Formatted log should work")
	}
}