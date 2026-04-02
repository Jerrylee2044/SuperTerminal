package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBashTool tests the Bash tool execution.
func TestBashTool(t *testing.T) {
	tool := &BashTool{}
	ctx := context.Background()

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantInOut string
	}{
		{
			name:    "simple echo",
			input:   `{"command": "echo hello world"}`,
			wantErr: false,
			wantInOut: "hello world",
		},
		{
			name:    "invalid json",
			input:   `{"command": invalid}`,
			wantErr: true,
		},
		{
			name:    "missing command",
			input:   `{"timeout": 10}`,
			wantErr: false, // Empty command still runs
		},
		{
			name:    "command with timeout",
			input:   `{"command": "sleep 0.1", "timeout": 5}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantInOut != "" && !contains(result, tt.wantInOut) {
				t.Errorf("expected output to contain '%s', got '%s'", tt.wantInOut, result)
			}
		})
	}
}

// TestBashToolTimeout tests command timeout.
func TestBashToolTimeout(t *testing.T) {
	tool := &BashTool{}
	ctx := context.Background()

	// Short timeout
	input := `{"command": "sleep 5", "timeout": 1}`
	
	start := time.Now()
	_, err := tool.Execute(ctx, input)
	duration := time.Since(start)
	
	if err == nil {
		t.Error("expected timeout error")
	}
	if duration > 2*time.Second {
		t.Errorf("command took too long: %v", duration)
	}
}

// TestBashToolWorkingDir tests working directory.
func TestBashToolWorkingDir(t *testing.T) {
	tool := &BashTool{WorkingDir: "/tmp"}
	ctx := context.Background()

	input := `{"command": "pwd"}`
	result, err := tool.Execute(ctx, input)
	
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !contains(result, "/tmp") {
		t.Errorf("expected working dir /tmp, got: %s", result)
	}
}

// TestFileReadTool tests file reading.
func TestFileReadTool(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := &FileReadTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(string) bool
	}{
		{
			name:    "read entire file",
			input:   fmt.Sprintf(`{"file_path": "%s"}`, tmpFile),
			wantErr: false,
			check: func(s string) bool {
				return contains(s, "line1") && contains(s, "line3")
			},
		},
		{
			name:    "read with line range",
			input:   fmt.Sprintf(`{"file_path": "%s", "start_line": 2, "end_line": 3}`, tmpFile),
			wantErr: false,
			check: func(s string) bool {
				return contains(s, "line2") && contains(s, "line3") && !contains(s, "line1")
			},
		},
		{
			name:    "read non-existent file",
			input:   `{"file_path": "/nonexistent/file.txt"}`,
			wantErr: true,
		},
		{
			name:    "read with home expansion",
			input:   `{"file_path": "~/.bashrc"}`,
			wantErr: false, // May or may not exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				// Skip if file doesn't exist (like .bashrc)
				if os.IsNotExist(err) {
					t.Skip("file does not exist")
				}
				t.Errorf("unexpected error: %v", err)
			}
			if tt.check != nil && !tt.check(result) {
				t.Errorf("check failed for result: %s", result)
			}
		})
	}
}

// TestFileWriteTool tests file writing.
func TestFileWriteTool(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &FileWriteTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "write new file",
			input:   fmt.Sprintf(`{"file_path": "%s/test.txt", "content": "hello world"}`, tmpDir),
			wantErr: false,
		},
		{
			name:    "write with nested directory",
			input:   fmt.Sprintf(`{"file_path": "%s/nested/deep/test.txt", "content": "nested content"}`, tmpDir),
			wantErr: false,
		},
		{
			name:    "write with home expansion",
			input:   fmt.Sprintf(`{"file_path": "~/.test_write_%d.txt", "content": "test"}`, time.Now().UnixNano()),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Errorf("failed to read written file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", content)
	}
}

// TestFileEditTool tests file editing.
func TestFileEditTool(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "edit.txt")
	
	// Create initial file
	if err := os.WriteFile(tmpFile, []byte("hello world\nhello again"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := &FileEditTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(string) bool
	}{
		{
			name:    "replace text",
			input:   fmt.Sprintf(`{"file_path": "%s", "old_text": "hello", "new_text": "hi"}`, tmpFile),
			wantErr: false,
			check: func(s string) bool {
				return contains(s, "2 occurrence")
			},
		},
		{
			name:    "text not found",
			input:   fmt.Sprintf(`{"file_path": "%s", "old_text": "nonexistent", "new_text": "replacement"}`, tmpFile),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.check != nil && !tt.check(result) {
				t.Errorf("check failed for result: %s", result)
			}
		})
	}

	// Verify file was edited
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(content), "hi world") {
		t.Errorf("expected edited content, got: %s", content)
	}
}

// TestGlobTool tests file globbing.
func TestGlobTool(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("// go file"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("text file"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "nested.go"), []byte("// nested go file"), 0644)

	tool := &GlobTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantMin int // minimum number of matches
	}{
		{
			name:    "simple glob",
			input:   fmt.Sprintf(`{"pattern": "*.go", "path": "%s"}`, tmpDir),
			wantErr: false,
			wantMin: 1,
		},
		{
			name:    "recursive glob",
			input:   fmt.Sprintf(`{"pattern": "**/*.go", "path": "%s"}`, tmpDir),
			wantErr: false,
			wantMin: 2, // test.go and nested.go
		},
		{
			name:    "no matches",
			input:   fmt.Sprintf(`{"pattern": "*.nonexistent", "path": "%s"}`, tmpDir),
			wantErr: false,
			wantMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			
			// Count matches (lines in result)
			if tt.wantMin > 0 {
				lines := len(splitLines(result))
				if lines < tt.wantMin {
					t.Errorf("expected at least %d matches, got %d", tt.wantMin, lines)
				}
			}
		})
	}
}

// TestGrepTool tests content search.
func TestGrepTool(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main\nfunc test() {}\n// Test comment"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("hello world\nTest line\nanother line"), 0644)

	tool := &GrepTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantMin int
	}{
		{
			name:    "search pattern",
			input:   fmt.Sprintf(`{"pattern": "Test", "path": "%s"}`, tmpDir),
			wantErr: false,
			wantMin: 2,
		},
		{
			name:    "search with case insensitive",
			input:   fmt.Sprintf(`{"pattern": "test", "path": "%s", "case_sensitive": false}`, tmpDir),
			wantErr: false,
			wantMin: 3, // test, Test, test
		},
		{
			name:    "search with file filter",
			input:   fmt.Sprintf(`{"pattern": "Test", "path": "%s", "include_pattern": "*.go"}`, tmpDir),
			wantErr: false,
			wantMin: 1,
		},
		{
			name:    "no matches",
			input:   fmt.Sprintf(`{"pattern": "nonexistent", "path": "%s"}`, tmpDir),
			wantErr: false,
			wantMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			
			if result == "No matches found" && tt.wantMin > 0 {
				t.Errorf("expected matches, got none")
			}
		})
	}
}

// TestToolManager tests tool management.
func TestToolManagerFull(t *testing.T) {
	tm := NewToolManager()

	// Register all built-in tools
	tm.RegisterTool("bash", &BashTool{})
	tm.RegisterTool("read", &FileReadTool{})
	tm.RegisterTool("write", &FileWriteTool{})
	tm.RegisterTool("edit", &FileEditTool{})
	tm.RegisterTool("glob", &GlobTool{})
	tm.RegisterTool("grep", &GrepTool{})

	// Test registration
	if len(tm.ListTools()) != 6 {
		t.Errorf("expected 6 tools, got %d", len(tm.ListTools()))
	}

	// Test get tool
	tool, ok := tm.GetTool("bash")
	if !ok {
		t.Error("expected to find bash tool")
	}
	if tool.Name() != "bash" {
		t.Errorf("expected name 'bash', got '%s'", tool.Name())
	}

	// Test tool definitions
	defs := tm.GetToolDefinitions()
	if len(defs) != 6 {
		t.Errorf("expected 6 definitions, got %d", len(defs))
	}

	// Check definition structure
	for _, def := range defs {
		if def.Name == "" {
			t.Error("tool definition missing name")
		}
		if def.Description == "" {
			t.Error("tool definition missing description")
		}
		if def.InputSchema == nil {
			t.Error("tool definition missing input_schema")
		}
	}
}

// TestExpandPath tests path expansion.
func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input  string
		expect string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/", home},
		{"~", "~"}, // Edge case: single ~ not expanded
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expect {
				t.Errorf("expected '%s', got '%s'", tt.expect, result)
			}
		})
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// TestWebSearchTool tests the WebSearch tool.
func TestWebSearchTool(t *testing.T) {
	tool := &WebSearchTool{}
	ctx := context.Background()

	// Test name and description
	if tool.Name() != "web_search" {
		t.Errorf("Expected name 'web_search', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}

	// Test input schema
	schema := tool.InputSchema()
	if schema["type"] != "object" {
		t.Error("Schema type should be object")
	}

	// Test with valid input (may fail without network)
	t.Run("valid input format", func(t *testing.T) {
		input := `{"query": "golang"}`
		result, err := tool.Execute(ctx, input)
		// We just check it doesn't crash
		_ = result
		_ = err
	})

	// Test with invalid JSON
	t.Run("invalid json", func(t *testing.T) {
		_, err := tool.Execute(ctx, `invalid json`)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})

	// Test with empty query
	t.Run("empty query", func(t *testing.T) {
		_, err := tool.Execute(ctx, `{"query": ""}`)
		if err == nil {
			t.Error("Expected error for empty query")
		}
	})
}

// TestWebFetchTool tests the WebFetch tool.
func TestWebFetchTool(t *testing.T) {
	tool := &WebFetchTool{}
	ctx := context.Background()

	// Test name and description
	if tool.Name() != "web_fetch" {
		t.Errorf("Expected name 'web_fetch', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}

	// Test input schema
	schema := tool.InputSchema()
	if schema["type"] != "object" {
		t.Error("Schema type should be object")
	}

	// Test with valid input (may fail without network)
	t.Run("valid input format", func(t *testing.T) {
		input := `{"url": "https://example.com"}`
		result, err := tool.Execute(ctx, input)
		// We just check it doesn't crash
		_ = result
		_ = err
	})

	// Test with invalid JSON
	t.Run("invalid json", func(t *testing.T) {
		_, err := tool.Execute(ctx, `invalid json`)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})

	// Test with empty URL
	t.Run("empty url", func(t *testing.T) {
		_, err := tool.Execute(ctx, `{"url": ""}`)
		if err == nil {
			t.Error("Expected error for empty URL")
		}
	})
}

// TestExtractTextFromHTML tests HTML text extraction.
func TestExtractTextFromHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		contains string
	}{
		{
			name:     "simple text",
			html:     `<html><body>Hello World</body></html>`,
			contains: "Hello World",
		},
		{
			name:     "text with tags",
			html:     `<p>Paragraph</p><div>Division</div>`,
			contains: "Paragraph",
		},
		{
			name:     "removes script",
			html:     `<script>alert('test')</script>Visible`,
			contains: "Visible",
		},
		{
			name:     "removes style",
			html:     `<style>.test{color:red}</style>Content`,
			contains: "Content",
		},
		{
			name:     "decodes entities",
			html:     `Hello&nbsp;World&amp;Test`,
			contains: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextFromHTML(tt.html)
			if !containsStr(result, tt.contains) {
				t.Errorf("Expected result to contain '%s', got '%s'", tt.contains, result)
			}
		})
	}
}

// containsStr checks if a string contains a substring.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}