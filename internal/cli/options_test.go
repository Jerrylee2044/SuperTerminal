package cli

import (
	"os"
	"testing"
)

// TestDefaultOptions tests default values.
func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Model == "" {
		t.Error("Default model should not be empty")
	}
	if opts.MaxTokens <= 0 {
		t.Error("Default max tokens should be positive")
	}
	if opts.UIMode == "" {
		t.Error("Default UI mode should not be empty")
	}
	if opts.WebPort <= 0 {
		t.Error("Default web port should be positive")
	}
}

// TestValidateUIMode tests UI mode validation.
func TestValidateUIMode(t *testing.T) {
	opts := DefaultOptions()

	// Valid modes
	for _, mode := range []string{"tui", "web", "both"} {
		opts.UIMode = mode
		if err := Validate(opts); err != nil {
			t.Errorf("UI mode '%s' should be valid: %v", mode, err)
		}
	}

	// Invalid mode
	opts.UIMode = "invalid"
	if err := Validate(opts); err == nil {
		t.Error("Invalid UI mode should return error")
	}
}

// TestValidateLogLevel tests log level validation.
func TestValidateLogLevel(t *testing.T) {
	opts := DefaultOptions()

	// Valid levels
	for _, level := range []string{"debug", "info", "warn", "error"} {
		opts.LogLevel = level
		if err := Validate(opts); err != nil {
			t.Errorf("Log level '%s' should be valid: %v", level, err)
		}
	}

	// Invalid level
	opts.LogLevel = "invalid"
	if err := Validate(opts); err == nil {
		t.Error("Invalid log level should return error")
	}
}

// TestValidatePort tests port validation.
func TestValidatePort(t *testing.T) {
	opts := DefaultOptions()

	// Valid ports
	validPorts := []int{1, 80, 443, 8080, 9000, 65535}
	for _, port := range validPorts {
		opts.WebPort = port
		if err := Validate(opts); err != nil {
			t.Errorf("Port %d should be valid: %v", port, err)
		}
	}

	// Invalid ports
	invalidPorts := []int{0, -1, 65536, 70000}
	for _, port := range invalidPorts {
		opts.WebPort = port
		if err := Validate(opts); err == nil {
			t.Errorf("Port %d should be invalid", port)
		}
	}
}

// TestValidateMaxTokens tests max tokens validation.
func TestValidateMaxTokens(t *testing.T) {
	opts := DefaultOptions()

	// Valid tokens
	validTokens := []int{1, 100, 8192, 100000}
	for _, tokens := range validTokens {
		opts.MaxTokens = tokens
		if err := Validate(opts); err != nil {
			t.Errorf("Max tokens %d should be valid: %v", tokens, err)
		}
	}

	// Invalid tokens
	invalidTokens := []int{0, -1, 100001}
	for _, tokens := range invalidTokens {
		opts.MaxTokens = tokens
		if err := Validate(opts); err == nil {
			t.Errorf("Max tokens %d should be invalid", tokens)
		}
	}
}

// TestContains tests the contains helper.
func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !contains(slice, "a") {
		t.Error("Should contain 'a'")
	}
	if contains(slice, "d") {
		t.Error("Should not contain 'd'")
	}
}

// TestGetEnvOrDefault tests env retrieval.
func TestGetEnvOrDefault(t *testing.T) {
	// Set env
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	val := GetEnvOrDefault("TEST_VAR", "default")
	if val != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", val)
	}

	// Missing env
	val = GetEnvOrDefault("MISSING_VAR", "default")
	if val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}
}

// TestGetAPIKeyFromEnv tests API key retrieval.
func TestGetAPIKeyFromEnv(t *testing.T) {
	// Clear all
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("CLAUDE_API_KEY")
	os.Unsetenv("API_KEY")

	key := GetAPIKeyFromEnv()
	if key != "" {
		t.Error("Should be empty when no env set")
	}

	// Set ANTHROPIC_API_KEY
	os.Setenv("ANTHROPIC_API_KEY", "test-key-1")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	key = GetAPIKeyFromEnv()
	if key != "test-key-1" {
		t.Errorf("Expected 'test-key-1', got '%s'", key)
	}

	// Set multiple, should prefer ANTHROPIC_API_KEY
	os.Setenv("CLAUDE_API_KEY", "test-key-2")
	os.Setenv("API_KEY", "test-key-3")

	key = GetAPIKeyFromEnv()
	if key != "test-key-1" {
		t.Error("Should prefer ANTHROPIC_API_KEY")
	}
}

// TestMergeWithEnv tests env merging.
func TestMergeWithEnv(t *testing.T) {
	opts := DefaultOptions()
	opts.DataDir = "" // Let it use default
	opts.APIKey = ""

	// Set env vars
	os.Setenv("ANTHROPIC_API_KEY", "env-api-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	opts.MergeWithEnv()

	if opts.APIKey != "env-api-key" {
		t.Errorf("API key should be merged from env, got '%s'", opts.APIKey)
	}

	// Data dir should be set
	if opts.DataDir == "" {
		t.Error("Data dir should be set after merge")
	}

	// Debug should set log level
	opts.Debug = true
	opts.MergeWithEnv()
	if opts.LogLevel != "debug" {
		t.Error("Debug mode should set log level to debug")
	}
}

// TestOptionsString tests string representation.
func TestOptionsString(t *testing.T) {
	opts := DefaultOptions()
	opts.Model = "test-model"
	opts.UIMode = "web"

	str := opts.String()
	if str == "" {
		t.Error("String should not be empty")
	}
	// Check that string contains key info
	if !containsStr(str, "test-model") {
		t.Error("String should contain model")
	}
	if !containsStr(str, "web") {
		t.Error("String should contain UI mode")
	}
}

// containsStr checks if a string contains another string.
func containsStr(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestVersionInfo tests version variables.
func TestVersionInfo(t *testing.T) {
	// Just check they exist and have expected format
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
}