package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Aliyun DASHSCOPE constants for CODING Plan
const (
	// DashScopeBaseURL is the base URL for Aliyun DashScope API (OpenAI compatible)
	DashScopeBaseURL = "https://coding.dashscope.aliyuncs.com/v1"
	
	// DashScopeBaseURLAnthropic is the Anthropic-compatible endpoint
	DashScopeBaseURLAnthropic = "https://coding.dashscope.aliyuncs.com/apps/anthropic"
	
	// DashScopeAPIKeyEnv is the environment variable for DashScope API key
	DashScopeAPIKeyEnv = "DASHSCOPE_API_KEY"
)

// Config holds the engine configuration.
type Config struct {
	// API settings
	APIKey      string `json:"api_key"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	MaxTokens   int    `json:"max_tokens"`
	
	// Behavior settings
	PermissionMode string `json:"permission_mode"` // "ask", "auto", "bypass"
	AutoCommit     bool   `json:"auto_commit"`
	AutoSave       bool   `json:"auto_save"`      // Auto-save session after each message
	
	// UI settings
	Theme         string `json:"theme"`
	ShowCost      bool   `json:"show_cost"`
	ShowTokens    bool   `json:"show_tokens"`
	
	// New settings
	DataDir       string `json:"data_dir"`
	Debug         bool   `json:"debug"`
	
	// MCP settings
	MCPServers []MCPServerConfig `json:"mcp_servers"`
	
	// Skills settings
	SkillsDirs []string `json:"skills_dirs"`

	// File paths
	ConfigPath string `json:"-"`
}

// MCPServerConfig represents an MCP server configuration.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Type    string            `json:"type"` // "stdio", "sse", "http"
	URL     string            `json:"url"`  // For SSE/HTTP types
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	
	// Check for DashScope API key first
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	baseURL := "https://api.anthropic.com"
	model := "claude-sonnet-4-20250514"
	
	if dashKey := os.Getenv("DASHSCOPE_API_KEY"); dashKey != "" {
		apiKey = dashKey
		baseURL = DashScopeBaseURL
		model = "qwen3.5-plus"
	}
	
	return &Config{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		Model:          model,
		MaxTokens:      4096,
		PermissionMode: "ask",
		AutoSave:       true,
		Theme:          "dark",
		ShowCost:       true,
		ShowTokens:     true,
		DataDir:        filepath.Join(homeDir, ".superterminal"),
		Debug:          false,
		SkillsDirs: []string{
			filepath.Join(homeDir, ".superterminal", "skills"),
		},
	}
}

// ConfigManager handles loading and saving configuration.
type ConfigManager struct {
	config *Config
	path   string
	mu     sync.RWMutex
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(path string) *ConfigManager {
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".superterminal", "config.json")
	}
	
	cm := &ConfigManager{
		path: path,
	}
	
	cm.config = cm.load()
	return cm
}

// load reads configuration from file.
func (cm *ConfigManager) load() *Config {
	config := DefaultConfig()
	config.ConfigPath = cm.path
	
	data, err := os.ReadFile(cm.path)
	if err != nil {
		// Config file doesn't exist, use defaults
		return config
	}
	
	if err := json.Unmarshal(data, config); err != nil {
		// Config file invalid, use defaults
		return config
	}
	
	return config
}

// Save writes configuration to file.
func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	// Ensure directory exists
	dir := filepath.Dir(cm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(cm.path, data, 0644)
}

// Get returns the current configuration.
func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// Update modifies the configuration.
func (cm *ConfigManager) Update(newConfig *Config) {
	cm.mu.Lock()
	cm.config = newConfig
	cm.mu.Unlock()
}

// SetModel changes the model setting.
func (cm *ConfigManager) SetModel(model string) {
	cm.mu.Lock()
	cm.config.Model = model
	cm.mu.Unlock()
}

// SetAPIKey changes the API key.
func (cm *ConfigManager) SetAPIKey(key string) {
	cm.mu.Lock()
	cm.config.APIKey = key
	cm.mu.Unlock()
}

// SetPermissionMode changes the permission mode.
func (cm *ConfigManager) SetPermissionMode(mode string) {
	cm.mu.Lock()
	cm.config.PermissionMode = mode
	cm.mu.Unlock()
}

// GetConfigPath returns the path to the config file.
func (cm *ConfigManager) GetConfigPath() string {
	return cm.path
}

// EnableDashScope configures the engine to use Aliyun DashScope (CODING Plan).
func (cm *ConfigManager) EnableDashScope(apiKey, model string) {
	cm.mu.Lock()
	cm.config.APIKey = apiKey
	cm.config.BaseURL = DashScopeBaseURL
	if model != "" {
		cm.config.Model = model
	} else {
		cm.config.Model = "qwen3.5-plus"
	}
	cm.mu.Unlock()
}

// IsDashScope returns true if using Aliyun DashScope.
func (cm *ConfigManager) IsDashScope() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.BaseURL == DashScopeBaseURL || cm.config.BaseURL == DashScopeBaseURLAnthropic
}

// GetAvailableModels returns list of available models based on current provider.
func (cm *ConfigManager) GetAvailableModels() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	if cm.config.BaseURL == DashScopeBaseURL || cm.config.BaseURL == DashScopeBaseURLAnthropic {
		return []string{
			"qwen3.5-plus",
			"qwen3.5",
			"qwen3-plus",
			"qwen3",
			"qwen2.5-coder-plus",
			"qwen2.5-coder",
		}
	}
	
	// Default Anthropic models
	return []string{
		"claude-sonnet-4-20250514",
		"claude-sonnet-4",
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}
}