// Package security provides tool permission control for SuperTerminal.
package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// PermissionLevel defines the permission level for tools.
type PermissionLevel int

const (
	PermissionDenied PermissionLevel = iota // Tool is not allowed
	PermissionAsk                            // Ask user for confirmation
	PermissionAllow                          // Tool is allowed without asking
)

// ToolPermission defines permissions for a tool.
type ToolPermission struct {
	Name        string          `json:"name"`
	Level       PermissionLevel `json:"level"`
	AllowList   []string        `json:"allow_list,omitempty"`   // Allowed patterns
	DenyList    []string        `json:"deny_list,omitempty"`    // Denied patterns
	MaxTimeout  int             `json:"max_timeout,omitempty"`  // Max execution time in seconds
	RequireConfirm bool         `json:"require_confirm,omitempty"` // Always require confirmation
}

// PermissionConfig contains all tool permissions.
type PermissionConfig struct {
	DefaultLevel PermissionLevel            `json:"default_level"`
	Tools        map[string]ToolPermission  `json:"tools"`
	DangerousCommands []string               `json:"dangerous_commands"` // Commands that need confirmation
	ProtectedPaths    []string               `json:"protected_paths"`    // Paths that need confirmation
	mu               sync.RWMutex
}

// PermissionManager manages tool permissions.
type PermissionManager struct {
	config     *PermissionConfig
	configFile string
	mu         sync.RWMutex
}

// PermissionResult contains the result of a permission check.
type PermissionResult struct {
	Allowed   bool
	Level     PermissionLevel
	Reason    string
	AskUser   bool
	Message   string
}

var (
	defaultPermManager *PermissionManager
	permOnce           sync.Once
)

// DefaultPermissionConfig returns the default permission configuration.
func DefaultPermissionConfig() *PermissionConfig {
	return &PermissionConfig{
		DefaultLevel: PermissionAsk,
		Tools: map[string]ToolPermission{
			"bash": {
				Name:      "bash",
				Level:     PermissionAsk,
				MaxTimeout: 60,
				DenyList: []string{
					"rm -rf /",
					"rm -rf ~",
					"rm -rf /*",
					"mkfs",
					"dd if=",
					":(){:|:&};:",  // Fork bomb
					"chmod -R 777 /",
					"chown -R",
					"shutdown",
					"reboot",
					"halt",
					"init 0",
					"init 6",
					"sudo rm",
					"sudo chmod",
					"sudo chown",
				},
			},
			"read": {
				Name:    "read",
				Level:   PermissionAllow,
			},
			"write": {
				Name:    "write",
				Level:   PermissionAsk,
				RequireConfirm: true,
			},
			"edit": {
				Name:    "edit",
				Level:   PermissionAsk,
				RequireConfirm: true,
			},
			"glob": {
				Name:    "glob",
				Level:   PermissionAllow,
			},
			"grep": {
				Name:    "grep",
				Level:   PermissionAllow,
			},
		},
		DangerousCommands: []string{
			"rm", "rmdir", "del", "format", "erase",
			"sudo", "su", "chmod", "chown",
			"shutdown", "reboot", "halt", "poweroff",
			"mkfs", "fdisk", "parted",
			"dd", "shred", "wipe",
		},
		ProtectedPaths: []string{
			"/etc/passwd",
			"/etc/shadow",
			"/etc/sudoers",
			"~/.ssh",
			"~/.gnupg",
			"~/.config",
		},
	}
}

// NewPermissionManager creates a new permission manager.
func NewPermissionManager(configFile string) *PermissionManager {
	config := DefaultPermissionConfig()
	
	// Load from file if exists
	if configFile != "" {
		if _, err := os.Stat(configFile); err == nil {
			data, err := os.ReadFile(configFile)
			if err == nil {
				json.Unmarshal(data, config)
			}
		}
	}
	
	return &PermissionManager{
		config:     config,
		configFile: configFile,
	}
}

// GetPermissionManager returns the default manager.
func GetPermissionManager() *PermissionManager {
	permOnce.Do(func() {
		defaultPermManager = NewPermissionManager("")
	})
	return defaultPermManager
}

// CheckToolPermission checks if a tool is allowed.
func (pm *PermissionManager) CheckToolPermission(toolName, input string) PermissionResult {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := PermissionResult{
		Allowed: true,
		Level:   pm.config.DefaultLevel,
	}

	// Get tool-specific permission
	perm, ok := pm.config.Tools[toolName]
	if !ok {
		// Use default
		result.Level = pm.config.DefaultLevel
		result.Reason = "using default permission"
		return result
	}

	result.Level = perm.Level

	// Check deny list first
	if len(perm.DenyList) > 0 {
		for _, pattern := range perm.DenyList {
			if strings.Contains(input, pattern) {
				result.Allowed = false
				result.Reason = "matches denied pattern: " + pattern
				result.Message = "This command is blocked for safety: " + pattern
				return result
			}
		}
	}

	// Check allow list
	if len(perm.AllowList) > 0 {
		for _, pattern := range perm.AllowList {
			if strings.Contains(input, pattern) {
				result.Allowed = true
				result.Level = PermissionAllow
				result.Reason = "matches allowed pattern: " + pattern
				return result
			}
		}
	}

	// Check dangerous commands
	if toolName == "bash" {
		if pm.isDangerousCommand(input) {
			result.Level = PermissionAsk
			result.AskUser = true
			result.Message = "This command may be dangerous. Proceed?"
		}
	}

	// Check protected paths
	if toolName == "write" || toolName == "edit" {
		if pm.isProtectedPath(input) {
			result.Level = PermissionAsk
			result.AskUser = true
			result.Message = "This path is protected. Proceed?"
		}
	}

	// Check if confirmation required
	if perm.RequireConfirm {
		result.AskUser = true
		result.Message = "This action requires confirmation."
	}

	return result
}

// isDangerousCommand checks if a command contains dangerous patterns.
func (pm *PermissionManager) isDangerousCommand(cmd string) bool {
	cmdLower := strings.ToLower(cmd)
	for _, dangerous := range pm.config.DangerousCommands {
		if strings.Contains(cmdLower, dangerous) {
			return true
		}
	}
	
	// Check for common dangerous patterns
	dangerousPatterns := []string{
		`\brm\s+-rf\b`,
		`\bsudo\b`,
		`\bchmod\s+[0-7]{3,4}\b`,
		`\bdd\s+if=`,
		`\bformat\b`,
		`\bshutdown\b`,
		`\breboot\b`,
	}
	
	for _, pattern := range dangerousPatterns {
		matched, _ := regexp.MatchString(pattern, cmdLower)
		if matched {
			return true
		}
	}
	
	return false
}

// isProtectedPath checks if a path is protected.
func (pm *PermissionManager) isProtectedPath(input string) bool {
	// Extract path from input
	var path string
	
	// Try to parse as JSON
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(input), &params); err == nil {
		path = params.FilePath
	} else {
		// Try to extract path from command
		path = input
	}
	
	// Expand home directory
	home, _ := os.UserHomeDir()
	path = strings.Replace(path, "~", home, 1)
	
	// Check against protected paths
	for _, protected := range pm.config.ProtectedPaths {
		protPath := strings.Replace(protected, "~", home, 1)
		if strings.HasPrefix(path, protPath) || strings.HasPrefix(protPath, path) {
			return true
		}
	}
	
	// Check for sensitive files
	sensitiveFiles := []string{
		"id_rsa", "id_ed25519", "id_ecdsa",
		".pem", ".key",
		".env", ".htpasswd",
		"credentials", "secrets",
	}
	
	pathBase := strings.ToLower(filepath.Base(path))
	for _, sensitive := range sensitiveFiles {
		if strings.Contains(pathBase, sensitive) {
			return true
		}
	}
	
	return false
}

// SetToolPermission sets permission for a tool.
func (pm *PermissionManager) SetToolPermission(toolName string, level PermissionLevel) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	perm := pm.config.Tools[toolName]
	perm.Name = toolName
	perm.Level = level
	pm.config.Tools[toolName] = perm
}

// AllowToolPattern adds an allow pattern for a tool.
func (pm *PermissionManager) AllowToolPattern(toolName, pattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	perm := pm.config.Tools[toolName]
	perm.AllowList = append(perm.AllowList, pattern)
	pm.config.Tools[toolName] = perm
}

// DenyToolPattern adds a deny pattern for a tool.
func (pm *PermissionManager) DenyToolPattern(toolName, pattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	perm := pm.config.Tools[toolName]
	perm.DenyList = append(perm.DenyList, pattern)
	pm.config.Tools[toolName] = perm
}

// AddDangerousCommand adds a command to the dangerous list.
func (pm *PermissionManager) AddDangerousCommand(cmd string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config.DangerousCommands = append(pm.config.DangerousCommands, cmd)
}

// AddProtectedPath adds a path to the protected list.
func (pm *PermissionManager) AddProtectedPath(path string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config.ProtectedPaths = append(pm.config.ProtectedPaths, path)
}

// Save saves the configuration to file.
func (pm *PermissionManager) Save() error {
	if pm.configFile == "" {
		return nil
	}
	
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	data, err := json.MarshalIndent(pm.config, "", "  ")
	if err != nil {
		return err
	}
	
	dir := filepath.Dir(pm.configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	return os.WriteFile(pm.configFile, data, 0644)
}

// GetConfig returns the current configuration.
func (pm *PermissionManager) GetConfig() *PermissionConfig {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.config
}

// === Global functions ===

// CheckToolPermission checks using default manager.
func CheckToolPermission(toolName, input string) PermissionResult {
	return GetPermissionManager().CheckToolPermission(toolName, input)
}

// SetToolPermission sets using default manager.
func SetToolPermission(toolName string, level PermissionLevel) {
	GetPermissionManager().SetToolPermission(toolName, level)
}

// AllowToolPattern allows a pattern.
func AllowToolPattern(toolName, pattern string) {
	GetPermissionManager().AllowToolPattern(toolName, pattern)
}

// DenyToolPattern denies a pattern.
func DenyToolPattern(toolName, pattern string) {
	GetPermissionManager().DenyToolPattern(toolName, pattern)
}