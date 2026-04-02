package security

import (
	"testing"
)

// TestPermissionLevels tests permission level constants.
func TestPermissionLevels(t *testing.T) {
	if PermissionDenied >= PermissionAsk {
		t.Error("PermissionDenied should be less than PermissionAsk")
	}
	if PermissionAsk >= PermissionAllow {
		t.Error("PermissionAsk should be less than PermissionAllow")
	}
}

// TestDefaultPermissionConfig tests default configuration.
func TestDefaultPermissionConfig(t *testing.T) {
	config := DefaultPermissionConfig()

	if config.DefaultLevel != PermissionAsk {
		t.Error("Default level should be PermissionAsk")
	}

	if len(config.Tools) == 0 {
		t.Error("Should have default tools")
	}

	// Check bash has deny list
	bash, ok := config.Tools["bash"]
	if !ok {
		t.Error("Should have bash tool")
	}
	if len(bash.DenyList) == 0 {
		t.Error("Bash should have deny list")
	}
}

// TestCheckToolPermissionAllow tests allowed tools.
func TestCheckToolPermissionAllow(t *testing.T) {
	pm := NewPermissionManager("")

	// Read should be allowed by default
	result := pm.CheckToolPermission("read", `{"file_path": "/tmp/test.txt"}`)
	if !result.Allowed {
		t.Error("Read should be allowed")
	}

	// Glob should be allowed
	result = pm.CheckToolPermission("glob", `{"pattern": "*.go"}`)
	if !result.Allowed {
		t.Error("Glob should be allowed")
	}

	// Grep should be allowed
	result = pm.CheckToolPermission("grep", `{"pattern": "test"}`)
	if !result.Allowed {
		t.Error("Grep should be allowed")
	}
}

// TestCheckToolPermissionDeny tests denied commands.
func TestCheckToolPermissionDeny(t *testing.T) {
	pm := NewPermissionManager("")

	// Dangerous commands should be blocked
	dangerous := []string{
		"rm -rf /",
		"rm -rf ~",
		"sudo rm -rf something",
		"mkfs /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		"chmod -R 777 /",
	}

	for _, cmd := range dangerous {
		result := pm.CheckToolPermission("bash", cmd)
		if result.Allowed {
			t.Errorf("Command should be denied: %s", cmd)
		}
		if result.Reason == "" {
			t.Errorf("Should have denial reason for: %s", cmd)
		}
	}
}

// TestCheckToolPermissionAsk tests commands requiring confirmation.
func TestCheckToolPermissionAsk(t *testing.T) {
	pm := NewPermissionManager("")

	// Normal commands should ask
	result := pm.CheckToolPermission("bash", "ls -la")
	if result.Level != PermissionAsk {
		t.Error("Normal bash command should ask")
	}

	// Write should ask
	result = pm.CheckToolPermission("write", `{"file_path": "/tmp/test.txt", "content": "test"}`)
	if result.Level != PermissionAsk {
		t.Error("Write should ask")
	}

	// Edit should ask
	result = pm.CheckToolPermission("edit", `{"file_path": "/tmp/test.txt", "old_text": "a", "new_text": "b"}`)
	if result.Level != PermissionAsk {
		t.Error("Edit should ask")
	}
}

// TestIsDangerousCommand tests dangerous command detection.
func TestIsDangerousCommand(t *testing.T) {
	pm := NewPermissionManager("")

	tests := []struct {
		cmd       string
		dangerous bool
	}{
		{"ls -la", false},
		{"rm -rf /", true},
		{"rm -rf ~", true},
		{"sudo ls", true},
		{"chmod 777 file", true},
		{"shutdown now", true},
		{"cat /etc/passwd", false},
		{"echo hello", false},
	}

	for _, tt := range tests {
		result := pm.isDangerousCommand(tt.cmd)
		if result != tt.dangerous {
			t.Errorf("isDangerousCommand(%s) = %v, want %v", tt.cmd, result, tt.dangerous)
		}
	}
}

// TestIsProtectedPath tests protected path detection.
func TestIsProtectedPath(t *testing.T) {
	pm := NewPermissionManager("")

	tests := []struct {
		input     string
		protected bool
	}{
		{`{"file_path": "/tmp/test.txt"}`, false},
		{`{"file_path": "/etc/passwd"}`, true},
		{`{"file_path": "~/.ssh/id_rsa"}`, true},
		{`{"file_path": "/home/user/.env"}`, true},
		{`{"file_path": "/home/user/secrets.json"}`, true},
		// Note: .config path matching depends on home directory expansion
		{`{"file_path": "/home/user/document.txt"}`, false},
	}

	for _, tt := range tests {
		result := pm.isProtectedPath(tt.input)
		if result != tt.protected {
			t.Errorf("isProtectedPath(%s) = %v, want %v", tt.input, result, tt.protected)
		}
	}
}

// TestSetToolPermission tests setting permissions.
func TestSetToolPermission(t *testing.T) {
	pm := NewPermissionManager("")

	// Set to allow
	pm.SetToolPermission("bash", PermissionAllow)
	
	config := pm.GetConfig()
	if config.Tools["bash"].Level != PermissionAllow {
		t.Error("Bash should be set to allow")
	}

	// Set to deny
	pm.SetToolPermission("bash", PermissionDenied)
	config = pm.GetConfig()
	if config.Tools["bash"].Level != PermissionDenied {
		t.Error("Bash should be set to denied")
	}
}

// TestAllowDenyPatterns tests allow/deny patterns.
func TestAllowDenyPatterns(t *testing.T) {
	pm := NewPermissionManager("")

	// Add allow pattern
	pm.AllowToolPattern("bash", "git ")
	result := pm.CheckToolPermission("bash", "git status")
	if !result.Allowed || result.Level != PermissionAllow {
		t.Error("Git command should be allowed by pattern")
	}

	// Add deny pattern
	pm.DenyToolPattern("bash", "curl ")
	result = pm.CheckToolPermission("bash", "curl http://example.com")
	if result.Allowed {
		t.Error("Curl command should be denied by pattern")
	}
}

// TestAddDangerousCommand tests adding dangerous commands.
func TestAddDangerousCommand(t *testing.T) {
	pm := NewPermissionManager("")

	pm.AddDangerousCommand("mydangerouscmd")
	
	if !pm.isDangerousCommand("mydangerouscmd --force") {
		t.Error("Custom dangerous command should be detected")
	}
}

// TestAddProtectedPath tests adding protected paths.
func TestAddProtectedPath(t *testing.T) {
	pm := NewPermissionManager("")

	pm.AddProtectedPath("/my/protected/path")
	
	if !pm.isProtectedPath(`{"file_path": "/my/protected/path/file.txt"}`) {
		t.Error("Custom protected path should be detected")
	}
}

// TestGlobalFunctions tests global permission functions.
func TestGlobalPermissionFunctions(t *testing.T) {
	// Check default manager works
	result := CheckToolPermission("read", `{"file_path": "/tmp"}`)
	if !result.Allowed {
		t.Error("Read should be allowed via global function")
	}

	// Set permission
	SetToolPermission("test_tool", PermissionAllow)
	AllowToolPattern("test_tool", "safe")
	DenyToolPattern("test_tool", "unsafe")
}

// TestPermissionResult tests result structure.
func TestPermissionResult(t *testing.T) {
	pm := NewPermissionManager("")
	
	// Test denied result
	result := pm.CheckToolPermission("bash", "rm -rf /")
	if result.Allowed {
		t.Error("Should not be allowed")
	}
	if result.Reason == "" {
		t.Error("Should have reason")
	}
	if result.Message == "" {
		t.Error("Should have message")
	}
}

// TestSaveLoadConfig tests saving and loading configuration.
func TestSaveLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/permissions.json"

	pm1 := NewPermissionManager(configFile)
	pm1.SetToolPermission("bash", PermissionAllow)
	pm1.AddDangerousCommand("testcmd")
	pm1.Save()

	// Create new manager with same file
	pm2 := NewPermissionManager(configFile)
	config := pm2.GetConfig()
	
	if config.Tools["bash"].Level != PermissionAllow {
		t.Error("Bash permission should be loaded from file")
	}
}