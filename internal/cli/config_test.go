package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigLoaderLoad tests loading configuration.
func TestConfigLoaderLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cl := NewConfigLoader(tmpDir)

	// Load default when no file exists
	config, err := cl.Load()
	if err != nil {
		t.Errorf("Failed to load default config: %v", err)
	}
	if config.Model == "" {
		t.Error("Default config should have model")
	}
}

// TestConfigLoaderSave tests saving configuration.
func TestConfigLoaderSave(t *testing.T) {
	tmpDir := t.TempDir()
	cl := NewConfigLoader(tmpDir)

	config := &ConfigFile{
		Model:     "test-model",
		MaxTokens: 4000,
		UIMode:    "web",
	}

	if err := cl.Save(config); err != nil {
		t.Errorf("Failed to save config: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("Config file should exist: %v", err)
	}

	// Load and verify
	loaded, err := cl.Load()
	if err != nil {
		t.Errorf("Failed to load saved config: %v", err)
	}
	if loaded.Model != "test-model" {
		t.Errorf("Model mismatch: got %s, want test-model", loaded.Model)
	}
}

// TestDefaultConfig tests default configuration values.
func TestDefaultConfig(t *testing.T) {
	cl := NewConfigLoader("")
	config := cl.defaultConfig()

	if config.Model == "" {
		t.Error("Default model should not be empty")
	}
	if config.MaxTokens <= 0 {
		t.Error("Default max tokens should be positive")
	}
	if config.Web.Port <= 0 {
		t.Error("Default web port should be positive")
	}
	if len(config.Permissions.Tools) == 0 {
		t.Error("Default should have permission tools")
	}
}

// TestParseSimpleFormat tests simple format parsing.
func TestParseSimpleFormat(t *testing.T) {
	cl := NewConfigLoader("")
	config := cl.defaultConfig()

	content := `# Test config
model = claude-opus-4-20250514
max_tokens = 4096
ui_mode = web
web_port = 9000
debug = true
`

	if err := cl.parseSimpleFormat(content, config); err != nil {
		t.Errorf("Failed to parse simple format: %v", err)
	}

	if config.Model != "claude-opus-4-20250514" {
		t.Errorf("Model mismatch: got %s", config.Model)
	}
	if config.MaxTokens != 4096 {
		t.Errorf("Max tokens mismatch: got %d", config.MaxTokens)
	}
	if config.UIMode != "web" {
		t.Errorf("UI mode mismatch: got %s", config.UIMode)
	}
	if config.Web.Port != 9000 {
		t.Errorf("Web port mismatch: got %d", config.Web.Port)
	}
	if !config.Logging.Debug {
		t.Error("Debug should be true")
	}
}

// TestMergeOptions tests merging options.
func TestMergeOptions(t *testing.T) {
	config := &ConfigFile{
		Model:     "config-model",
		MaxTokens: 4000,
		UIMode:    "web",
		Web: WebConfig{
			Port: 9000,
			Host: "0.0.0.0",
		},
	}

	// CLI overrides config
	opts := Options{Model: "cli-model"}
	result := MergeOptions(config, opts)
	if result.Model != "cli-model" {
		t.Error("CLI model should override config")
	}

	// Config fills empty CLI
	opts = Options{Model: ""}
	result = MergeOptions(config, opts)
	if result.Model != "config-model" {
		t.Error("Config model should fill empty CLI")
	}

	// Default CLI with config override
	opts = DefaultOptions()
	result = MergeOptions(config, opts)
	if result.Model != "config-model" {
		t.Error("Config should override default CLI model")
	}
}

// TestInitializeDataDir tests data directory initialization.
func TestInitializeDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "st-data")

	if err := InitializeDataDir(dataDir); err != nil {
		t.Errorf("Failed to initialize data dir: %v", err)
	}

	// Check directory exists
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("Data dir should exist: %v", err)
	}

	// Check subdirectories
	subdirs := []string{"sessions", "logs", "cache"}
	for _, subdir := range subdirs {
		path := filepath.Join(dataDir, subdir)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s dir should exist: %v", subdir, err)
		}
	}

	// Check default config exists
	configPath := filepath.Join(dataDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("Default config should exist: %v", err)
	}
}

// TestParseInt tests integer parsing.
func TestParseInt(t *testing.T) {
	tests := []struct {
		input  string
		expect int
	}{
		{"123", 123},
		{"0", 0},
		{"8080", 8080},
		{"100abc", 100},
		{"", 0},
	}

	for _, tt := range tests {
		result := parseInt(tt.input)
		if result != tt.expect {
			t.Errorf("parseInt(%s) = %d, want %d", tt.input, result, tt.expect)
		}
	}
}

// TestGetConfigPath tests config path generation.
func TestGetConfigPath(t *testing.T) {
	path := GetConfigPath("/home/user/.superterminal")
	expected := "/home/user/.superterminal/config.json"
	if path != expected {
		t.Errorf("Config path mismatch: got %s, want %s", path, expected)
	}
}

// TestEditConfig tests config editing.
func TestEditConfig(t *testing.T) {
	tmpDir := t.TempDir()

	if err := EditConfig(tmpDir); err != nil {
		t.Errorf("EditConfig failed: %v", err)
	}

	// Config file should exist after EditConfig
	configPath := filepath.Join(tmpDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("Config should be created: %v", err)
	}
}

// TestConfigFileJSON tests JSON serialization.
func TestConfigFileJSON(t *testing.T) {
	config := &ConfigFile{
		Model:     "test-model",
		MaxTokens: 8192,
		Web: WebConfig{
			Port: 8080,
			Host: "localhost",
		},
		Permissions: PermissionsConfig{
			DefaultLevel: "ask",
			Tools: map[string]string{
				"bash": "ask",
			},
		},
	}

	tmpDir := t.TempDir()
	cl := NewConfigLoader(tmpDir)

	// Save and load tests JSON roundtrip
	if err := cl.Save(config); err != nil {
		t.Errorf("Failed to save: %v", err)
	}

	loaded, err := cl.Load()
	if err != nil {
		t.Errorf("Failed to load: %v", err)
	}

	if loaded.Model != config.Model {
		t.Errorf("Model mismatch after JSON roundtrip")
	}
	if loaded.MaxTokens != config.MaxTokens {
		t.Errorf("MaxTokens mismatch after JSON roundtrip")
	}
}