// Package cli provides configuration file loading for SuperTerminal.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFile represents a configuration file structure.
type ConfigFile struct {
	// API Configuration
	Model     string `json:"model" yaml:"model"`
	APIKey    string `json:"api_key" yaml:"api_key"`
	BaseURL   string `json:"base_url" yaml:"base_url"`
	MaxTokens int    `json:"max_tokens" yaml:"max_tokens"`

	// UI Configuration
	UIMode string `json:"ui_mode" yaml:"ui_mode"`
	Web    WebConfig `json:"web" yaml:"web"`
	TUI    TUIConfig `json:"tui" yaml:"tui"`

	// Data Configuration
	DataDir  string `json:"data_dir" yaml:"data_dir"`
	Logging  LoggingConfig `json:"logging" yaml:"logging"`

	// Session Configuration
	Session SessionConfig `json:"session" yaml:"session"`

	// MCP Configuration
	MCP MCPConfig `json:"mcp" yaml:"mcp"`

	// Permissions
	Permissions PermissionsConfig `json:"permissions" yaml:"permissions"`
}

// WebConfig contains web UI configuration.
type WebConfig struct {
	Port int    `json:"port" yaml:"port"`
	Host string `json:"host" yaml:"host"`
}

// TUIConfig contains terminal UI configuration.
type TUIConfig struct {
	Theme      string `json:"theme" yaml:"theme"`
	ShowCost   bool   `json:"show_cost" yaml:"show_cost"`
	ShowTokens bool   `json:"show_tokens" yaml:"show_tokens"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	File  string `json:"file" yaml:"file"`
	Level string `json:"level" yaml:"level"`
	Debug bool   `json:"debug" yaml:"debug"`
}

// SessionConfig contains session configuration.
type SessionConfig struct {
	AutoSave bool `json:"auto_save" yaml:"auto_save"`
	MaxSaved int  `json:"max_saved" yaml:"max_saved"`
}

// MCPConfig contains MCP configuration.
type MCPConfig struct {
	Enable bool `json:"enable" yaml:"enable"`
	Port   int  `json:"port" yaml:"port"`
}

// PermissionsConfig contains permission configuration.
type PermissionsConfig struct {
	DefaultLevel string            `json:"default_level" yaml:"default_level"`
	Tools        map[string]string `json:"tools" yaml:"tools"`
}

// ConfigLoader handles configuration file loading.
type ConfigLoader struct {
	dataDir string
}

// NewConfigLoader creates a new config loader.
func NewConfigLoader(dataDir string) *ConfigLoader {
	return &ConfigLoader{dataDir: dataDir}
}

// Load loads configuration from file.
func (cl *ConfigLoader) Load() (*ConfigFile, error) {
	// Try JSON first, then YAML-like (actually still JSON for simplicity)
	configPath := cl.configPath()
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config
		return cl.defaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		// Try to parse as simple format
		config = *cl.defaultConfig()
		if err := cl.parseSimpleFormat(string(data), &config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	return &config, nil
}

// Save saves configuration to file.
func (cl *ConfigLoader) Save(config *ConfigFile) error {
	configPath := cl.configPath()
	
	// Ensure directory exists
	if err := os.MkdirAll(cl.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// configPath returns the configuration file path.
func (cl *ConfigLoader) configPath() string {
	return filepath.Join(cl.dataDir, "config.json")
}

// defaultConfig returns the default configuration.
func (cl *ConfigLoader) defaultConfig() *ConfigFile {
	return &ConfigFile{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
		UIMode:    "tui",
		Web: WebConfig{
			Port: 8080,
			Host: "localhost",
		},
		TUI: TUIConfig{
			Theme:      "default",
			ShowCost:   true,
			ShowTokens: true,
		},
		Logging: LoggingConfig{
			Level: "info",
			Debug: false,
		},
		Session: SessionConfig{
			AutoSave: true,
			MaxSaved: 100,
		},
		MCP: MCPConfig{
			Enable: false,
			Port:   9000,
		},
		Permissions: PermissionsConfig{
			DefaultLevel: "ask",
			Tools: map[string]string{
				"bash":    "ask",
				"read":    "allow",
				"write":   "ask",
				"edit":    "ask",
				"glob":    "allow",
				"grep":    "allow",
			},
		},
	}
}

// parseSimpleFormat parses a simple key=value format.
func (cl *ConfigLoader) parseSimpleFormat(content string, config *ConfigFile) error {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "model":
			config.Model = value
		case "api_key":
			config.APIKey = value
		case "max_tokens":
			config.MaxTokens = parseInt(value)
		case "ui_mode":
			config.UIMode = value
		case "web_port":
			config.Web.Port = parseInt(value)
		case "web_host":
			config.Web.Host = value
		case "log_file":
			config.Logging.File = value
		case "log_level":
			config.Logging.Level = value
		case "debug":
			config.Logging.Debug = value == "true"
		case "data_dir":
			config.DataDir = value
		case "mcp_enable":
			config.MCP.Enable = value == "true"
		case "mcp_port":
			config.MCP.Port = parseInt(value)
		}
	}
	return nil
}

// parseInt parses an integer from string.
func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result * 10 + int(c - '0')
		}
	}
	return result
}

// MergeOptions merges config file with command-line options.
func MergeOptions(config *ConfigFile, opts Options) Options {
	result := opts

	// Config file values are defaults, command-line overrides
	if result.Model == "" || result.Model == DefaultOptions().Model {
		if config.Model != "" {
			result.Model = config.Model
		}
	}

	if result.APIKey == "" {
		if config.APIKey != "" {
			result.APIKey = config.APIKey
		}
	}

	if result.BaseURL == "" {
		if config.BaseURL != "" {
			result.BaseURL = config.BaseURL
		}
	}

	if result.MaxTokens == 0 || result.MaxTokens == DefaultOptions().MaxTokens {
		if config.MaxTokens > 0 {
			result.MaxTokens = config.MaxTokens
		}
	}

	if result.UIMode == "" || result.UIMode == DefaultOptions().UIMode {
		if config.UIMode != "" {
			result.UIMode = config.UIMode
		}
	}

	if result.WebPort == 0 || result.WebPort == DefaultOptions().WebPort {
		if config.Web.Port > 0 {
			result.WebPort = config.Web.Port
		}
	}

	if result.WebHost == "" || result.WebHost == DefaultOptions().WebHost {
		if config.Web.Host != "" {
			result.WebHost = config.Web.Host
		}
	}

	if result.DataDir == "" {
		if config.DataDir != "" {
			result.DataDir = config.DataDir
		}
	}

	if result.LogFile == "" {
		if config.Logging.File != "" {
			result.LogFile = config.Logging.File
		}
	}

	if result.LogLevel == "" || result.LogLevel == DefaultOptions().LogLevel {
		if config.Logging.Level != "" {
			result.LogLevel = config.Logging.Level
		}
	}

	if !result.Debug && config.Logging.Debug {
		result.Debug = config.Logging.Debug
	}

	if !result.MCPEnable && config.MCP.Enable {
		result.MCPEnable = config.MCP.Enable
	}

	if result.MCPPort == 0 || result.MCPPort == DefaultOptions().MCPPort {
		if config.MCP.Port > 0 {
			result.MCPPort = config.MCP.Port
		}
	}

	return result
}

// InitializeDataDir initializes the data directory with default files.
func InitializeDataDir(dataDir string) error {
	// Create directory
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{
		"sessions",
		"logs",
		"cache",
	}
	for _, subdir := range subdirs {
		path := filepath.Join(dataDir, subdir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Create default config if not exists
	cl := NewConfigLoader(dataDir)
	configPath := cl.configPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := cl.defaultConfig()
		if err := cl.Save(config); err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}
	}

	return nil
}

// GetConfigPath returns the configuration file path.
func GetConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "config.json")
}

// EditConfig opens the config file for editing (creates if not exists).
func EditConfig(dataDir string) error {
	cl := NewConfigLoader(dataDir)
	configPath := cl.configPath()

	// Ensure config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := cl.defaultConfig()
		if err := cl.Save(config); err != nil {
			return err
		}
	}

	// Print config path for user
	fmt.Printf("Config file location: %s\n", configPath)
	fmt.Println("Edit this file to customize SuperTerminal settings.")
	
	return nil
}