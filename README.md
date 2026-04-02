# SuperTerminal

> A high-performance AI terminal assistant with dual UI (Terminal + Web)

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-green.svg)]()

SuperTerminal is a Claude-powered CLI tool written in Go, featuring:

- **🚀 High Performance** - ~50ms startup, ~20MB memory, streaming responses
- **🖥️ Dual UI** - Bubble Tea terminal + Web UI with real-time sync
- **🔧 Rich Tool System** - Bash, file operations, web search, MCP integration
- **💾 Session Management** - Auto-save, search, export (text/json/markdown)
- **🔒 Security First** - Permission control, secure storage, audit logs
- **🌐 MCP Protocol** - Connect external MCP servers for extended capabilities

## Features

| Feature | Terminal UI | Web UI |
|:---|:---|:---|
| Fast startup (~50ms) | ✅ | ✅ |
| Low memory (~20MB) | ✅ | ✅ |
| Keyboard shortcuts | ✅ | ✅ |
| Streaming responses | ✅ | ✅ |
| Tool execution | ✅ | ✅ |
| Cost tracking | ✅ | ✅ |
| Session management | ✅ | ✅ |
| Rich formatting | ⚠️ Basic | ✅ Full |
| Syntax highlighting | ⚠️ Basic | ✅ Full |
| Theme switching | ❌ | ✅ Dark/Light |
| Charts/graphs | ❌ | ✅ |
| Remote access | ❌ | ✅ |
| Mobile support | ❌ | ✅ Responsive |

## Quick Start

```bash
# Install dependencies
make deps

# Build
make build

# Set API key
export ANTHROPIC_API_KEY=your-key-here

# Run terminal UI
./bin/superterminal

# Run with Web UI
./bin/superterminal --web

# Run Web UI only (for remote access)
./bin/superterminal --web-only --port 8080
```

## Installation

### From Source

```bash
git clone https://github.com/yourname/SuperTerminal.git
cd SuperTerminal
make deps build
```

### Binary Download

```bash
# Download latest release
curl -sL https://github.com/yourname/SuperTerminal/releases/latest/download/superterminal-linux-amd64 -o superterminal
chmod +x superterminal
sudo mv superterminal /usr/local/bin/
```

### One-line Install

```bash
curl -sL https://raw.githubusercontent.com/yourname/SuperTerminal/main/scripts/install.sh | bash
```

## Usage Modes

### Terminal UI (Default)

```bash
superterminal
```

Fastest mode - pure terminal interface with keyboard shortcuts.

**Keyboard Shortcuts:**

| Key | Action |
|:---|:---|
| `Enter` | Send message |
| `↑/↓` | Command history navigation |
| `Tab` | Autocomplete commands |
| `Ctrl+O` | Toggle multiline mode |
| `Ctrl+H` | Toggle help panel |
| `Ctrl+L` | Clear screen |
| `Ctrl+P` | Toggle progress indicator |
| `Ctrl+C/Esc` | Cancel or exit |

### Terminal + Web (Hybrid)

```bash
superterminal --web --port 8080
```

Run both UIs simultaneously. Web UI accessible at `http://localhost:8080`.

### Web Only

```bash
superterminal --web-only --port 8080 --host 0.0.0.0
```

For server deployment or remote access. No terminal UI.

## Commands

| Command | Description |
|:---|:---|
| `/help` | Show available commands |
| `/exit` | Exit SuperTerminal |
| `/clear` | Clear message history |
| `/reset` | Reset conversation |
| `/model [name]` | Set or show model |
| `/cost` | Show cost statistics |
| `/sessions` | List saved sessions |
| `/load <id>` | Load a session |
| `/save [title]` | Save current session |
| `/export [format]` | Export session (text/json/markdown) |
| `/search <query>` | Search sessions |
| `/mcp` | MCP status and commands |
| `/mcp list` | List MCP connections |
| `/mcp tools` | List MCP tools |
| `/mcp connect <url>` | Connect MCP server |
| `/config` | View configuration |
| `/version` | Show version |

## Built-in Tools

| Tool | Description |
|:---|:---|
| `bash` | Execute shell commands |
| `read` | Read file contents |
| `write` | Write file contents |
| `edit` | Find-replace in files |
| `glob` | Find files matching pattern |
| `grep` | Search file contents |
| `web_search` | Search the web (DuckDuckGo) |
| `web_fetch` | Fetch web page content |
| `browser_use` | Control browser with Playwright |
| `send_file` | Send file to user |

## MCP Integration

SuperTerminal supports connecting to external MCP (Model Context Protocol) servers:

```bash
# Connect MCP server via stdio
/mcp connect stdio:/path/to/mcp-server

# Connect via HTTP
/mcp connect http://localhost:3000/mcp

# Connect via SSE
/mcp connect sse://localhost:3000/sse

# List available MCP tools
/mcp tools

# Use MCP tool (auto-registered)
```

## Configuration

Config file: `~/.superterminal/config.json`

```json
{
  "api_key": "",
  "base_url": "https://api.anthropic.com",
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 4096,
  "permission_mode": "ask",
  "show_cost": true,
  "show_tokens": true,
  "auto_save": true,
  "log_level": "info",
  "web_port": 8080
}
```

### Environment Variables

| Variable | Description |
|:---|:---|
| `ANTHROPIC_API_KEY` | Claude API key |
| `SUPERTERMINAL_DATA_DIR` | Data directory (default: ~/.superterminal) |
| `SUPERTERMINAL_LOG_LEVEL` | Log level (debug/info/warn/error) |

### CLI Options

| Option | Description |
|:---|:---|
| `--web` | Enable Web UI alongside TUI |
| `--web-only` | Run Web UI only (no TUI) |
| `--port` | Web UI port (default: 8080) |
| `--host` | Web UI host (default: localhost) |
| `--model` | Override model |
| `--api-key` | Override API key |
| `--data-dir` | Override data directory |
| `--debug` | Enable debug logging |
| `--log-file` | Log file path |
| `--version` | Show version |
| `--help` | Show help |

## Architecture

```
SuperTerminal/
├── cmd/superterminal/         # Entry point
├── internal/
│   ├── engine/                # Core engine (shared)
│   │   ├── engine.go          # Main engine
│   │   ├── event_bus.go       # Event distribution
│   │   ├── api_client.go      # Claude API client
│   │   ├── session.go         # Session state
│   │   ├── config.go          # Configuration
│   │   └── tools.go           # Tool execution
│   │   └── mcp_manager.go     # MCP integration
│   ├── tui/                   # Bubble Tea terminal UI
│   │   └── app.go             # TUI application
│   ├── webui/                 # Web UI
│   │   ├── server.go          # HTTP/WebSocket server
│   │   └── websocket.go       # WebSocket handler
│   ├── cli/                   # CLI parsing
│   ├── logger/                # Logging system
│   ├── security/              # Security & permissions
│   │   ├── storage.go         # Secure storage
│   │   └── permissions.go     # Permission control
│   ├── persistence/           # Session persistence
│   │   └── session.go         # Session storage
│   ├── mcp/                   # MCP protocol
│   │   ├── client.go          # MCP client
│   │   └── client_manager.go  # Connection manager
│   ├── cache/                 # Caching system
│   │   └── cache.go           # API/File/Tool cache
│   ├── concurrency/           # Parallel execution
│   │   └── parallel.go        # Executor, RateLimiter
│   └── resource/              # Resource management
│       └── manager.go         # Memory, Sessions, Logs
├── web/
│   └── index.html             # Web UI frontend
├── scripts/
│   ├── release.sh             # Release script
│   └── install.sh             # Install script
├── docs/
│   └── USER_GUIDE.md          # User guide
├── Makefile                   # Build automation
├── VERSION                    # Version file
├── CHANGELOG.md               # Change log
└── README.md                  # This file
```

## Project Status

**Current Version:** v0.4.0

| Phase | Status | Features |
|:---|:---|:---|
| Phase 1 | ✅ Complete | CLI, Config, Environment init |
| Phase 2 | ✅ Complete | WebSocket, Web UI |
| Phase 3 | ✅ Complete | Tool permissions, web_search/fetch |
| Phase 4 | ✅ Complete | Session management, search |
| Phase 5 | ✅ Complete | MCP client, tools, resources |
| Phase 6 | ✅ Complete | Cache, concurrency, resource management |
| Phase 7 | ✅ Complete | TUI/Web UX enhancements |
| Phase 8 | ✅ Complete | Documentation, release scripts |

**Statistics:**
- Go Code: 15,586 lines
- Web UI: 1,724 lines
- Tests: 180 (all passing)
- Binary Size: 6.9MB (stripped)

## Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Clean build artifacts
make clean

# Create release
make release VERSION=0.4.0
```

## Development

```bash
# Run in development mode
make dev

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Lint
make lint
```

## Dependencies

- Go 1.21+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - UI components
- [Gorilla WebSocket](https://github.com/gorilla/websocket) - WebSocket

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) for details.

## Credits

- Inspired by [Claude Code](https://claude.ai) by Anthropic
- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) by Charmbracelet
- MCP protocol from [Anthropic MCP](https://modelcontextprotocol.io)

---

**Made with ❤️ by the SuperTerminal Team**