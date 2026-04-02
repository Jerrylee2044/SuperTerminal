// SuperTerminal - A high-performance AI terminal assistant with dual UI.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	
	"superterminal/internal/cli"
	"superterminal/internal/engine"
	"superterminal/internal/logger"
	"superterminal/internal/tui"
	"superterminal/internal/webui"
)

// Build information (set at build time).
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	// Set version info
	cli.Version = Version
	cli.BuildDate = BuildDate

	// Parse command-line arguments
	opts := cli.Parse()
	opts.MergeWithEnv()

	// Initialize data directory
	if err := cli.InitializeDataDir(opts.DataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize data directory: %v\n", err)
		os.Exit(1)
	}

	// Load config file
	configLoader := cli.NewConfigLoader(opts.DataDir)
	configFile, err := configLoader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Merge config with options (CLI takes priority)
	opts = cli.MergeOptions(configFile, opts)

	// Validate API key
	if opts.APIKey == "" {
		fmt.Println("⚠️  No API key configured.")
		fmt.Println()
		fmt.Println("Set your Anthropic API key using one of these methods:")
		fmt.Println()
		fmt.Println("  1. Environment variable:")
		fmt.Println("     export ANTHROPIC_API_KEY=your-key-here")
		fmt.Println()
		fmt.Println("  2. Command-line argument:")
		fmt.Println("     superterminal --api-key your-key-here")
		fmt.Println()
		fmt.Println("  3. Config file (~/.superterminal/config.json):")
		fmt.Println("     { \"api_key\": \"your-key-here\" }")
		fmt.Println()
		os.Exit(1)
	}

	// Convert CLI options to engine config
	engineConfig := &engine.Config{
		Model:       opts.Model,
		APIKey:      opts.APIKey,
		MaxTokens:   opts.MaxTokens,
		DataDir:     opts.DataDir,
		Debug:       opts.Debug,
	}

	// Create engine with new options
	e := engine.NewEngine(engine.EngineOptions{
		Config:     engineConfig,
		BufferSize: 100,
		DataDir:    opts.DataDir,
		EnableLog:  true,
		LogFile:    opts.LogFile,
	})

	// Setup logging
	setupLogging(e, opts)

	// Setup graceful shutdown
	setupShutdown(e, opts)

	// Start based on UI mode
	switch opts.UIMode {
	case "web":
		startWebOnly(e, opts)
	case "both":
		startHybrid(e, opts)
	default:
		startTUI(e, opts)
	}
}

func setupLogging(e *engine.Engine, opts cli.Options) {
	log := e.GetLogger()
	if opts.Debug {
		log.SetLevel(logger.LevelDebug)
	}
	log.Info("SuperTerminal starting",
		logger.Field{Key: "version", Value: cli.Version},
		logger.Field{Key: "model", Value: opts.Model},
		logger.Field{Key: "ui_mode", Value: opts.UIMode},
	)
}

func setupShutdown(e *engine.Engine, opts cli.Options) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		e.GetLogger().Info("Shutdown signal received")
		e.Shutdown()
		os.Exit(0)
	}()
}

func startTUI(e *engine.Engine, opts cli.Options) {
	log := e.GetLogger()
	log.Info("Starting TUI mode")

	// Create Bubble Tea program
	p := tea.NewProgram(
		tui.NewModel(e),
		tea.WithAltScreen(),
	)

	// Run TUI
	if _, err := p.Run(); err != nil {
		log.Error("TUI error", logger.Field{Key: "error", Value: err.Error()})
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	e.Shutdown()
}

func startWebOnly(e *engine.Engine, opts cli.Options) {
	log := e.GetLogger()
	log.Info("Starting Web mode", logger.Field{Key: "port", Value: opts.WebPort})

	server := webui.NewServer(e, webui.ServerOptions{
		Port:       opts.WebPort,
		StaticPath: opts.DataDir + "/web",
	})

	fmt.Printf("SuperTerminal Web UI running on http://%s:%d\n", opts.WebHost, opts.WebPort)
	fmt.Println("Press Ctrl+C to stop")

	if err := server.Start(); err != nil {
		log.Error("Web UI error", logger.Field{Key: "error", Value: err.Error()})
		fmt.Fprintf(os.Stderr, "Web UI error: %v\n", err)
		os.Exit(1)
	}
}

func startHybrid(e *engine.Engine, opts cli.Options) {
	log := e.GetLogger()
	log.Info("Starting hybrid mode (TUI + Web)")

	// Start Web UI in background
	server := webui.NewServer(e, webui.ServerOptions{
		Port:       opts.WebPort,
		StaticPath: opts.DataDir + "/web",
	})

	go func() {
		fmt.Printf("Web UI available at http://%s:%d\n", opts.WebHost, opts.WebPort)
		if err := server.Start(); err != nil {
			log.Error("Web UI error", logger.Field{Key: "error", Value: err.Error()})
		}
	}()

	// Run TUI in foreground
	startTUI(e, opts)

	// Stop Web UI when TUI exits
	server.Stop()
}