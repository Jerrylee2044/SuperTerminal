// Package cli provides command-line argument parsing for SuperTerminal.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Version information (set at build time).
var (
	Version   = "dev"
	BuildDate = "unknown"
)

// Options contains all command-line options.
type Options struct {
	// API Configuration
	Model     string
	APIKey    string
	MaxTokens int

	// UI Mode
	UIMode    string // "tui", "web", "both"
	WebPort   int
	WebHost   string

	// Data & Logging
	DataDir   string
	LogFile   string
	LogLevel  string
	Debug     bool

	// Session
	SessionID string
	LoadLatest bool

	// MCP
	MCPEnable bool
	MCPPort   int

	// Commands
	ShowHelp    bool
	ShowVersion bool
}

// DefaultOptions returns default options.
func DefaultOptions() Options {
	return Options{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
		UIMode:    "tui",
		WebPort:   8080,
		WebHost:   "localhost",
		LogLevel:  "info",
		Debug:     false,
		MCPEnable: false,
		MCPPort:   9000,
	}
}

// Parse parses command-line arguments.
func Parse() Options {
	opts := DefaultOptions()

	// Define flags
	flag.StringVar(&opts.Model, "model", opts.Model, "AI model to use")
	flag.StringVar(&opts.APIKey, "api-key", "", "API key (or set ANTHROPIC_API_KEY env)")
	flag.IntVar(&opts.MaxTokens, "max-tokens", opts.MaxTokens, "Maximum tokens per response")

	flag.StringVar(&opts.UIMode, "ui", opts.UIMode, "UI mode: tui, web, both")
	flag.IntVar(&opts.WebPort, "port", opts.WebPort, "Web UI port")
	flag.StringVar(&opts.WebHost, "host", opts.WebHost, "Web UI host")

	flag.StringVar(&opts.DataDir, "data-dir", "", "Data directory (default: ~/.superterminal)")
	flag.StringVar(&opts.LogFile, "log-file", "", "Log file path")
	flag.StringVar(&opts.LogLevel, "log-level", opts.LogLevel, "Log level: debug, info, warn, error")
	flag.BoolVar(&opts.Debug, "debug", false, "Enable debug mode")

	flag.StringVar(&opts.SessionID, "session", "", "Load specific session by ID")
	flag.BoolVar(&opts.LoadLatest, "latest", false, "Load latest session on startup")

	flag.BoolVar(&opts.MCPEnable, "mcp", false, "Enable MCP server")
	flag.IntVar(&opts.MCPPort, "mcp-port", opts.MCPPort, "MCP server port")

	flag.BoolVar(&opts.ShowHelp, "help", false, "Show help")
	flag.BoolVar(&opts.ShowVersion, "version", false, "Show version")

	// Short flags
	flag.BoolVar(&opts.ShowHelp, "h", false, "Show help (short)")
	flag.BoolVar(&opts.ShowVersion, "v", false, "Show version (short)")
	flag.StringVar(&opts.UIMode, "u", opts.UIMode, "UI mode (short)")

	// Custom usage
	flag.Usage = func() {
		printUsage()
	}

	flag.Parse()

	// Handle help and version
	if opts.ShowHelp {
		flag.Usage()
		os.Exit(0)
	}

	if opts.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	// Validate
	if err := Validate(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		flag.Usage()
		os.Exit(1)
	}

	return opts
}

// Validate validates the options.
func Validate(opts Options) error {
	// Validate UI mode
	validModes := []string{"tui", "web", "both"}
	if !contains(validModes, opts.UIMode) {
		return fmt.Errorf("invalid UI mode '%s', must be one of: %s", opts.UIMode, strings.Join(validModes, ", "))
	}

	// Validate log level
	validLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLevels, opts.LogLevel) {
		return fmt.Errorf("invalid log level '%s', must be one of: %s", opts.LogLevel, strings.Join(validLevels, ", "))
	}

	// Validate port
	if opts.WebPort < 1 || opts.WebPort > 65535 {
		return fmt.Errorf("invalid port %d, must be 1-65535", opts.WebPort)
	}

	// Validate max tokens
	if opts.MaxTokens < 1 || opts.MaxTokens > 100000 {
		return fmt.Errorf("invalid max tokens %d, must be 1-100000", opts.MaxTokens)
	}

	return nil
}

// contains checks if a string is in a slice.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// printUsage prints the usage information.
func printUsage() {
	fmt.Printf(`SuperTerminal - AI-Powered Terminal Assistant

Usage: superterminal [options]

Options:
  --model <name>       AI model to use (default: claude-sonnet-4-20250514)
  --api-key <key>      API key (or set ANTHROPIC_API_KEY environment variable)
  --max-tokens <n>     Maximum tokens per response (default: 8192)

  --ui <mode>          UI mode: tui, web, both (default: tui)
  --port <n>           Web UI port (default: 8080)
  --host <host>        Web UI host (default: localhost)

  --data-dir <dir>     Data directory (default: ~/.superterminal)
  --log-file <file>    Log file path
  --log-level <level>  Log level: debug, info, warn, error (default: info)
  --debug              Enable debug mode

  --session <id>       Load specific session by ID
  --latest             Load latest session on startup

  --mcp                Enable MCP server
  --mcp-port <n>       MCP server port (default: 9000)

  --help, -h           Show this help
  --version, -v        Show version

Examples:
  superterminal                    # Start with terminal UI
  superterminal --ui web           # Start with web UI only
  superterminal --ui both          # Start both UIs
  superterminal --model claude-opus-4-20250514
  superterminal --debug --log-file /tmp/st.log
  superterminal --latest           # Resume last session
  superterminal --session 20260402-120000-abc123

Environment Variables:
  ANTHROPIC_API_KEY    API key (required if not provided via --api-key)
  SUPERTERMINAL_DATA_DIR   Override data directory

`)
}

// printVersion prints version information.
func printVersion() {
	version := Version
	// Remove leading 'v' if present to avoid double 'v'
	if len(version) > 0 && version[0] == 'v' {
		version = version[1:]
	}
	fmt.Printf(`SuperTerminal v%s
Build: %s

A high-performance AI terminal assistant with dual UI support.
`, version, BuildDate)
}

// GetEnvOrDefault gets an environment variable or returns default.
func GetEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// GetDefaultDataDir returns the default data directory.
func GetDefaultDataDir() string {
	if dir := os.Getenv("SUPERTERMINAL_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".superterminal"
	}
	return home + "/.superterminal"
}

// GetAPIKeyFromEnv gets API key from environment.
func GetAPIKeyFromEnv() string {
	// Try multiple env vars
	keys := []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_API_KEY",
		"API_KEY",
	}
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

// MergeWithOptions merges environment variables with options.
func (opts *Options) MergeWithEnv() {
	// Data dir
	if opts.DataDir == "" {
		opts.DataDir = GetDefaultDataDir()
	}

	// API key
	if opts.APIKey == "" {
		opts.APIKey = GetAPIKeyFromEnv()
	}

	// Debug mode enables debug log level
	if opts.Debug {
		opts.LogLevel = "debug"
	}
}

// String returns a string representation of options.
func (opts Options) String() string {
	return fmt.Sprintf(`Options:
  Model: %s
  UI Mode: %s
  Data Dir: %s
  Log Level: %s
  Web: %s:%d
  MCP: %v (port %d)
`,
		opts.Model,
		opts.UIMode,
		opts.DataDir,
		opts.LogLevel,
		opts.WebHost, opts.WebPort,
		opts.MCPEnable, opts.MCPPort,
	)
}