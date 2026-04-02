package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Tool is the interface for all tools.
type Tool interface {
	// Name returns the tool name.
	Name() string
	
	// Description returns the tool description.
	Description() string
	
	// InputSchema returns the JSON schema for tool input.
	InputSchema() map[string]interface{}
	
	// Execute runs the tool with the given input.
	Execute(ctx context.Context, input string) (string, error)
}

// ToolInfo contains information about a tool execution.
type ToolInfo struct {
	Name      string `json:"name"`
	Input     string `json:"input,omitempty"`
	Output    string `json:"output,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Error     string `json:"error,omitempty"`
	Duration  int64  `json:"duration_ms,omitempty"`
}

// PermissionRequest represents a request for tool permission.
type PermissionRequest struct {
	ToolName  string `json:"tool_name"`
	Input     string `json:"input"`
	ToolUseID string `json:"tool_use_id"`
	Message   string `json:"message"`
}

// ToolDef represents a tool definition for the API.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolManager manages all available tools.
type ToolManager struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewToolManager creates a new ToolManager.
func NewToolManager() *ToolManager {
	return &ToolManager{
		tools: make(map[string]Tool),
	}
}

// RegisterTool adds a tool to the manager.
func (tm *ToolManager) RegisterTool(name string, tool Tool) {
	tm.mu.Lock()
	tm.tools[name] = tool
	tm.mu.Unlock()
}

// GetTool returns a tool by name.
func (tm *ToolManager) GetTool(name string) (Tool, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tool, ok := tm.tools[name]
	return tool, ok
}

// GetToolDefinitions returns all tool definitions for the API.
func (tm *ToolManager) GetToolDefinitions() []ToolDef {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	defs := []ToolDef{}
	for name, tool := range tm.tools {
		defs = append(defs, ToolDef{
			Name:        name,
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}
	return defs
}

// ListTools returns all registered tool names.
func (tm *ToolManager) ListTools() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	names := []string{}
	for name := range tm.tools {
		names = append(names, name)
	}
	return names
}

// === Built-in Tools ===

// BashTool executes shell commands.
type BashTool struct {
	WorkingDir string // Optional working directory
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Execute a shell command and return the output. Use for system operations, file management, git commands, etc."
}

func (t *BashTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default 60, max 600)",
				"default":     60,
			},
			"working_dir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command (optional)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		Command    string `json:"command"`
		Timeout    int    `json:"timeout"`
		WorkingDir string `json:"working_dir"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	// Validate timeout
	if params.Timeout <= 0 {
		params.Timeout = 60
	}
	if params.Timeout > 600 {
		params.Timeout = 600 // Max 10 minutes
	}
	
	// Determine shell
	shell := "/bin/sh"
	if _, err := os.Stat("/bin/bash"); err == nil {
		shell = "/bin/bash"
	}
	
	// Create command with context
	ctx, cancel := context.WithTimeout(ctx, time.Duration(params.Timeout)*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, shell, "-c", params.Command)
	
	// Set working directory
	if params.WorkingDir != "" {
		cmd.Dir = params.WorkingDir
	} else if t.WorkingDir != "" {
		cmd.Dir = t.WorkingDir
	}
	
	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)
	
	// Build result
	result := fmt.Sprintf("Command: %s\n", params.Command)
	result += fmt.Sprintf("Duration: %v\n", duration)
	result += fmt.Sprintf("Exit Code: %d\n", cmd.ProcessState.ExitCode())
	
	if stdout.Len() > 0 {
		result += fmt.Sprintf("\n--- STDOUT ---\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		result += fmt.Sprintf("\n--- STDERR ---\n%s", stderr.String())
	}
	
	// Truncate if too long
	if len(result) > 50000 {
		result = result[:50000] + "\n... [output truncated]"
	}
	
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %d seconds", params.Timeout)
		}
		return result, fmt.Errorf("command failed: %w", err)
	}
	
	return result, nil
}

// FileReadTool reads file contents.
type FileReadTool struct {
	MaxFileSize int64 // Max file size in bytes (default 10MB)
}

func (t *FileReadTool) Name() string { return "read" }

func (t *FileReadTool) Description() string {
	return "Read the contents of a file. Returns the file content with line numbers."
}

func (t *FileReadTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file to read",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "Starting line number (1-based, optional)",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "Ending line number (1-based, optional)",
			},
		},
		"required": []string{"file_path"},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		FilePath  string `json:"file_path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	// Expand ~ to home directory
	filePath := expandPath(params.FilePath)
	
	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot access file: %w", err)
	}
	
	// Check file size
	maxSize := t.MaxFileSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxSize)
	}
	
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	
	// Read with line numbers
	scanner := bufio.NewScanner(file)
	var result strings.Builder
	lineNum := 1
	
	// Adjust buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	for scanner.Scan() {
		// Check context
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		
		// Filter by line range
		if params.StartLine > 0 && lineNum < params.StartLine {
			lineNum++
			continue
		}
		if params.EndLine > 0 && lineNum > params.EndLine {
			break
		}
		
		// Write line with number
		result.WriteString(fmt.Sprintf("%6d\t%s\n", lineNum, scanner.Text()))
		lineNum++
		
		// Truncate if too long
		if result.Len() > 100000 {
			result.WriteString("... [file truncated]\n")
			break
		}
	}
	
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	
	return result.String(), nil
}

// FileWriteTool writes content to a file.
type FileWriteTool struct{}

func (t *FileWriteTool) Name() string { return "write" }

func (t *FileWriteTool) Description() string {
	return "Write content to a file. Creates the file if it doesn't exist, overwrites if it does."
}

func (t *FileWriteTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	filePath := expandPath(params.FilePath)
	
	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	
	// Write file
	if err := os.WriteFile(filePath, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), filePath), nil
}

// FileEditTool edits a file by replacing text.
type FileEditTool struct{}

func (t *FileEditTool) Name() string { return "edit" }

func (t *FileEditTool) Description() string {
	return "Edit a file by replacing specific text. All occurrences of old_text are replaced with new_text."
}

func (t *FileEditTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the file to edit",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "The text to find and replace",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		"required": []string{"file_path", "old_text", "new_text"},
	}
}

func (t *FileEditTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		FilePath string `json:"file_path"`
		OldText  string `json:"old_text"`
		NewText  string `json:"new_text"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	filePath := expandPath(params.FilePath)
	
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	
	// Count occurrences
	count := strings.Count(string(content), params.OldText)
	if count == 0 {
		return "", fmt.Errorf("text not found in file: %s", params.OldText)
	}
	
	// Replace
	newContent := strings.ReplaceAll(string(content), params.OldText, params.NewText)
	
	// Write back
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	
	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", count, filePath), nil
}

// GlobTool finds files matching a pattern.
type GlobTool struct{}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern. Returns list of matching file paths."
}

func (t *GlobTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The glob pattern to match (e.g., **/*.go)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory to search (default: current directory)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	searchPath := params.Path
	if searchPath == "" {
		searchPath = "."
	}
	searchPath = expandPath(searchPath)
	
	// Use filepath.Glob for simple patterns
	// For ** patterns, we need to walk
	var matches []string
	var err error
	
	if strings.Contains(params.Pattern, "**") {
		// Recursive glob
		err = filepath.Walk(searchPath, func(path string, info os.FileInfo, walkErr error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			
			if walkErr != nil {
				return nil // Skip errors
			}
			if info.IsDir() {
				return nil
			}
			
			// Match pattern
			relPath, relErr := filepath.Rel(searchPath, path)
			if relErr != nil {
				return nil
			}
			
			matched, matchErr := filepath.Match(strings.ReplaceAll(params.Pattern, "**", "*"), relPath)
			if matchErr != nil {
				return nil
			}
			
			// Also try matching with full path
			if !matched {
				matched, _ = filepath.Match(params.Pattern, path)
			}
			
			// Simple ** matching
			if !matched && strings.Contains(params.Pattern, "**") {
				basePattern := strings.TrimPrefix(params.Pattern, "**/")
				matched, _ = filepath.Match(basePattern, filepath.Base(path))
			}
			
			if matched {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walk directory: %w", err)
		}
	} else {
		// Simple glob
		fullPattern := filepath.Join(searchPath, params.Pattern)
		matches, err = filepath.Glob(fullPattern)
		if err != nil {
			return "", fmt.Errorf("glob: %w", err)
		}
	}
	
	if len(matches) == 0 {
		return "No files found matching pattern", nil
	}
	
	// Limit output
	if len(matches) > 1000 {
		matches = matches[:1000]
		return strings.Join(matches, "\n") + "\n... [truncated, more files found]", nil
	}
	
	return strings.Join(matches, "\n"), nil
}

// GrepTool searches file contents.
type GrepTool struct{}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Search for a pattern in file contents. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory or file to search (default: current directory)",
			},
			"case_sensitive": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to match case (default: true)",
			},
			"include_pattern": map[string]interface{}{
				"type":        "string",
				"description": "File name pattern to include (e.g., *.go)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		Pattern        string `json:"pattern"`
		Path           string `json:"path"`
		CaseSensitive  bool   `json:"case_sensitive"`
		IncludePattern string `json:"include_pattern"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	
	searchPath := params.Path
	if searchPath == "" {
		searchPath = "."
	}
	searchPath = expandPath(searchPath)
	
	// Build regex pattern
	pattern := params.Pattern
	if !params.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	
	var results []string
	maxResults := 100
	
	// Walk and search
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		if err != nil || info.IsDir() {
			return nil
		}
		
		// Check include pattern
		if params.IncludePattern != "" {
			matched, _ := filepath.Match(params.IncludePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}
		
		// Skip binary files and common non-text files
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".git" || ext == ".gitignore" {
			return nil
		}
		
		// Read and search file
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		
		scanner := bufio.NewScanner(file)
		lineNum := 1
		
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(line) {
				relPath, _ := filepath.Rel(searchPath, path)
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, lineNum, line))
				
				if len(results) >= maxResults {
					return fmt.Errorf("max results reached")
				}
			}
			lineNum++
		}
		
		return nil
	})
	
	if len(results) == 0 {
		return "No matches found", nil
	}
	
	result := strings.Join(results, "\n")
	if len(results) >= maxResults {
		result += "\n... [truncated, more matches found]"
	}
	
	return result, nil
}

// === Helper Functions ===

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// === Web Tools ===

// WebSearchTool searches the web using DuckDuckGo.
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Search the web for information. Returns search results with titles, URLs, and snippets. Use for finding current information, documentation, or answers to questions."
}

func (t *WebSearchTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		Query      string `json:"query"`
		NumResults int    `json:"num_results"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.NumResults <= 0 {
		params.NumResults = 5
	}
	if params.NumResults > 10 {
		params.NumResults = 10
	}
	
	// Use DuckDuckGo HTML search
	url := "https://html.duckduckgo.com/html/?q=" + params.Query
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SuperTerminal/1.0)")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search failed with status: %d", resp.StatusCode)
	}
	
	// Parse HTML response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	
	results := parseDuckDuckGoResults(string(body), params.NumResults)
	
	if len(results) == 0 {
		return "No results found for: " + params.Query, nil
	}
	
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Search results for: %s\n\n", params.Query))
	
	for i, result := range results {
		output.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		output.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		if result.Snippet != "" {
			output.WriteString(fmt.Sprintf("   %s\n", result.Snippet))
		}
		output.WriteString("\n")
	}
	
	return output.String(), nil
}

// SearchResult represents a search result.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// parseDuckDuckGoResults parses DuckDuckGo HTML search results.
func parseDuckDuckGoResults(html string, maxResults int) []SearchResult {
	var results []SearchResult
	
	// Simple regex-based parsing
	titleRegex := regexp.MustCompile(`<a[^>]*class="result__a"[^>]*>([^<]+)</a>`)
	urlRegex := regexp.MustCompile(`<a[^>]*class="result__url"[^>]*>([^<]+)</a>`)
	snippetRegex := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>([^<]+)</a>`)
	
	titles := titleRegex.FindAllStringSubmatch(html, maxResults)
	urls := urlRegex.FindAllStringSubmatch(html, maxResults)
	snippets := snippetRegex.FindAllStringSubmatch(html, maxResults)
	
	for i := 0; i < len(titles) && i < maxResults; i++ {
		result := SearchResult{}
		
		if len(titles[i]) > 1 {
			result.Title = strings.TrimSpace(titles[i][1])
		}
		
		if i < len(urls) && len(urls[i]) > 1 {
			result.URL = strings.TrimSpace(urls[i][1])
			// Add protocol if missing
			if !strings.HasPrefix(result.URL, "http") {
				result.URL = "https://" + result.URL
			}
		}
		
		if i < len(snippets) && len(snippets[i]) > 1 {
			result.Snippet = strings.TrimSpace(snippets[i][1])
		}
		
		if result.Title != "" || result.URL != "" {
			results = append(results, result)
		}
	}
	
	return results
}

// WebFetchTool fetches content from a URL.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch content from a URL. Returns the text content of the page. Use for retrieving web pages, documentation, or API responses."
}

func (t *WebFetchTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch",
			},
			"max_length": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum content length to return (default: 10000)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, input string) (string, error) {
	var params struct {
		URL       string `json:"url"`
		MaxLength int    `json:"max_length"`
	}
	
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	
	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if params.MaxLength <= 0 {
		params.MaxLength = 10000
	}
	
	// Validate URL
	if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
		params.URL = "https://" + params.URL
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SuperTerminal/1.0)")
	req.Header.Set("Accept", "text/html,application/json,text/plain")
	
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetch failed with status: %d %s", resp.StatusCode, resp.Status)
	}
	
	// Read response body
	limitedReader := io.LimitReader(resp.Body, int64(params.MaxLength+1000))
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	
	contentType := resp.Header.Get("Content-Type")
	
	// Process content based on type
	var result string
	
	if strings.Contains(contentType, "application/json") {
		// Format JSON
		var jsonContent interface{}
		if err := json.Unmarshal(body, &jsonContent); err == nil {
			formatted, _ := json.MarshalIndent(jsonContent, "", "  ")
			result = string(formatted)
		} else {
			result = string(body)
		}
	} else if strings.Contains(contentType, "text/html") {
		// Extract text from HTML
		result = extractTextFromHTML(string(body))
	} else {
		result = string(body)
	}
	
	// Truncate if needed
	if len(result) > params.MaxLength {
		result = result[:params.MaxLength] + "\n... [truncated]"
	}
	
	return fmt.Sprintf("URL: %s\nContent-Type: %s\n\n%s", params.URL, contentType, result), nil
}

// extractTextFromHTML extracts readable text from HTML.
func extractTextFromHTML(html string) string {
	// Remove scripts and styles
	html = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<!--[\s\S]*?-->`).ReplaceAllString(html, "")
	
	// Remove HTML tags but preserve structure
	html = regexp.MustCompile(`<br[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<p[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`</p>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<li[^>]*>`).ReplaceAllString(html, "\n- ")
	html = regexp.MustCompile(`<h[1-6][^>]*>`).ReplaceAllString(html, "\n\n")
	html = regexp.MustCompile(`</h[1-6]>`).ReplaceAllString(html, "\n")
	
	// Remove remaining tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	
	// Decode HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	
	// Clean up whitespace
	lines := strings.Split(html, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	
	result := strings.Join(cleanLines, "\n")
	
	// Remove excessive newlines
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	
	return result
}