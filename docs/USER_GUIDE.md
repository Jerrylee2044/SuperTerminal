# SuperTerminal User Guide

> Complete guide for using SuperTerminal - your AI-powered terminal assistant

## Table of Contents

1. [Getting Started](#getting-started)
2. [Terminal UI Guide](#terminal-ui-guide)
3. [Web UI Guide](#web-ui-guide)
4. [Commands Reference](#commands-reference)
5. [Tools Reference](#tools-reference)
6. [MCP Integration](#mcp-integration)
7. [Configuration](#configuration)
8. [Session Management](#session-management)
9. [Tips & Best Practices](#tips--best-practices)

---

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Anthropic API key
- Terminal with color support (for TUI mode)

### Installation

**From Source:**
```bash
git clone https://github.com/yourname/SuperTerminal.git
cd SuperTerminal
make deps build
```

**Binary Download:**
```bash
curl -sL https://github.com/yourname/SuperTerminal/releases/latest/download/superterminal-linux-amd64 -o superterminal
chmod +x superterminal
sudo mv superterminal /usr/local/bin/
```

### First Run

1. Set your API key:
   ```bash
   export ANTHROPIC_API_KEY=sk-ant-xxxxx
   ```

2. Run SuperTerminal:
   ```bash
   ./bin/superterminal
   ```

3. Type a message and press Enter to start!

---

## Terminal UI Guide

### Layout

```
┌─────────────────────────────────────────────────┐
│ ⚡ SuperTerminal ○                               │  Title
│                                                 │
│ 👤 You: What files are here?                    │  Messages
│ 🤖 Assistant: Let me check...                   │
│ [bash] ls -la                                   │  Tool panel
│                                                 │
│ Ready | Cost: $0.01 | Tokens: 50/100           │  Status bar
│                                                 │
│ ┌─ Type a message... ────────────────────────── │  Input box
│ >                                               │
│                                                 │
└─────────────────────────────────────────────────┘
```

### Keyboard Shortcuts

| Shortcut | Action | Description |
|:---|:---|:---|
| `Enter` | Send | Send current message |
| `↑` | History Up | Navigate to previous message in history |
| `↓` | History Down | Navigate to next message in history |
| `Tab` | Autocomplete | Cycle through command suggestions |
| `Shift+Tab` | Reverse Autocomplete | Cycle suggestions backwards |
| `Ctrl+O` | Multiline | Toggle multiline input mode |
| `Ctrl+H` | Help | Toggle help panel |
| `Ctrl+L` | Clear | Clear message history |
| `Ctrl+P` | Progress | Toggle progress indicator |
| `Ctrl+C` | Cancel | Cancel current operation |
| `Esc` | Exit | Exit application |

### Multiline Input

When you need to write longer messages or code:

1. Press `Ctrl+O` to enter multiline mode
2. Type each line and press `Enter` to add it
3. Press `Ctrl+O` again to send all lines
4. Press `Esc` to cancel and exit multiline mode

### Command Autocomplete

When typing commands starting with `/`:

1. Start typing: `/s`
2. Press `Tab` to see suggestions: `/save`, `/search`, `/sessions`
3. Keep pressing `Tab` to cycle through options
4. Press `Enter` when the desired command is shown

---

## Web UI Guide

### Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ 📋 Sessions │ ⚡ SuperTerminal │ Cost: $0.01 │ 🌙 │ Ready │ ⚙️  │
├─────────────┼───────────────────────────────────────────────────┤
│ Current     │                                                   │
│ Session     │  👤 You: What files are in current directory?     │
│             │                                                   │
│ Session 1   │  🤖 Assistant: Let me check with ls -la...        │
│ 2 hours ago │                                                   │
│             │  ┌─ Tool: bash ────────────────────────────────── │
│ Session 2   │  │ running: ls -la                                │
│ yesterday   │  └─────────────────────────────────────────────── │
│             │                                                   │
│ + New       │  Ready | Tokens: 50/100                           │
├─────────────┼───────────────────────────────────────────────────┤
│             │  ┌─ Type a message or /help... ────────────────── │
│             │  │                                                │
│             │  └────────────────────────────────────────────────│
└─────────────┴───────────────────────────────────────────────────┘
```

### Features

**Theme Toggle:**
- Click 🌙/☀️ button to switch between dark and light themes
- Theme preference is saved in browser localStorage

**Code Blocks:**
- Syntax highlighting for common languages (Go, Python, JavaScript, Bash)
- Copy button with "✓ Copied" feedback
- Language label in header

**Session Sidebar:**
- List all saved sessions
- Click to load a session
- Create new session with "+ New" button

**Keyboard Shortcuts (Web):**
- `Enter` - Send message
- `Shift+Enter` - New line in input
- `Ctrl+/` - Focus input
- `Esc` - Cancel streaming

---

## Commands Reference

### Basic Commands

| Command | Usage | Example |
|:---|:---|:---|
| `/help` | Show all commands | `/help` |
| `/exit` | Exit SuperTerminal | `/exit` |
| `/clear` | Clear messages | `/clear` |
| `/reset` | Reset conversation | `/reset` |
| `/version` | Show version | `/version` |

### Model Commands

| Command | Usage | Example |
|:---|:---|:---|
| `/model` | Show current model | `/model` |
| `/model <name>` | Set model | `/model claude-opus-4` |

### Session Commands

| Command | Usage | Example |
|:---|:---|:---|
| `/sessions` | List sessions | `/sessions` |
| `/load <id>` | Load session | `/load session-001` |
| `/save [title]` | Save session | `/save My Project` |
| `/export [fmt]` | Export session | `/export markdown` |
| `/search <q>` | Search sessions | `/search error handling` |

### MCP Commands

| Command | Usage | Example |
|:---|:---|:---|
| `/mcp` | MCP status | `/mcp` |
| `/mcp list` | List connections | `/mcp list` |
| `/mcp tools` | List MCP tools | `/mcp tools` |
| `/mcp resources` | List resources | `/mcp resources` |
| `/mcp prompts` | List prompts | `/mcp prompts` |
| `/mcp connect <url>` | Connect server | `/mcp connect stdio:/path/to/server` |
| `/mcp disconnect <id>` | Disconnect | `/mcp disconnect mcp-1` |

### Info Commands

| Command | Usage | Example |
|:---|:---|:---|
| `/cost` | Show costs | `/cost` |
| `/status` | Engine status | `/status` |
| `/config` | Show config | `/config` |

---

## Tools Reference

### File Operations

**read** - Read file contents
```
Please read the file config.json
```

**write** - Write/create file
```
Create a new file test.txt with content "Hello World"
```

**edit** - Find and replace in file
```
Replace "old" with "new" in config.json
```

**glob** - Find files by pattern
```
Find all .go files in the project
```

**grep** - Search file contents
```
Search for "function" in all .go files
```

### Shell Operations

**bash** - Execute shell commands
```
List all files with details: ls -la
Check git status: git status
```

### Web Operations

**web_search** - Search the web
```
Search for "Go programming best practices"
```

**web_fetch** - Fetch web content
```
Fetch content from https://example.com
```

---

## MCP Integration

### What is MCP?

MCP (Model Context Protocol) lets SuperTerminal connect to external servers that provide additional tools, resources, and prompts.

### Connection Types

**stdio** - Local process communication
```bash
/mcp connect stdio:/path/to/mcp-server
```

**HTTP** - HTTP-based MCP server
```bash
/mcp connect http://localhost:3000/mcp
```

**SSE** - Server-Sent Events
```bash
/mcp connect sse://localhost:3000/sse
```

### Using MCP Tools

Once connected, MCP tools are automatically registered:

1. Check available tools: `/mcp tools`
2. Use them like built-in tools in your messages
3. Example: "Use the database_query tool to get all users"

---

## Configuration

### Config File Location

`~/.superterminal/config.json`

### Configuration Options

| Option | Type | Default | Description |
|:---|:---|:---|:---|
| `api_key` | string | "" | Anthropic API key |
| `base_url` | string | "https://api.anthropic.com" | API endpoint |
| `model` | string | "claude-sonnet-4-20250514" | Model to use |
| `max_tokens` | int | 4096 | Max output tokens |
| `permission_mode` | string | "ask" | Permission mode (ask/allow/deny) |
| `show_cost` | bool | true | Show cost in UI |
| `show_tokens` | bool | true | Show token counts |
| `auto_save` | bool | true | Auto-save sessions |
| `log_level` | string | "info" | Logging level |
| `web_port` | int | 8080 | Web UI port |

### Example Config

```json
{
  "api_key": "",
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 4096,
  "permission_mode": "ask",
  "show_cost": true,
  "auto_save": true,
  "web_port": 8080
}
```

### Environment Variables

| Variable | Description |
|:---|:---|
| `ANTHROPIC_API_KEY` | API key (overrides config) |
| `SUPERTERMINAL_DATA_DIR` | Data directory |
| `SUPERTERMINAL_LOG_LEVEL` | Log level |

---

## Session Management

### Auto-Save

Sessions are automatically saved after each conversation when `auto_save: true`.

### Manual Save

```bash
/save My Project Discussion
```

### Load Session

```bash
# List sessions
/sessions

# Load by ID
/load session-2026-04-02-001
```

### Search Sessions

```bash
/search error handling
```

Results show matching sessions with context snippets.

### Export Session

```bash
/export text      # Plain text
/export json      # JSON format
/export markdown  # Markdown format
```

Files are saved to `~/.superterminal/exports/`

---

## Tips & Best Practices

### Efficient Communication

1. **Be specific** - "Read main.go and explain the architecture" vs "explain code"
2. **Use context** - "In the current directory, find all test files"
3. **Chain operations** - "Find .go files, then search for 'error' in them"

### Performance Tips

1. **Use appropriate model** - Sonnet for most tasks, Opus for complex reasoning
2. **Batch operations** - "Do X, then Y, then Z" in one message
3. **Cache results** - Similar requests are cached automatically

### Security Tips

1. **Review tool permissions** - Check before allowing sensitive operations
2. **Use ask mode** - Set `permission_mode: ask` for safety
3. **Audit logs** - Check `~/.superterminal/logs/audit.log` for activity

### Keyboard Efficiency

1. **History navigation** - Use ↑/↓ to recall previous messages
2. **Autocomplete** - Use Tab for commands, save typing
3. **Multiline** - Use Ctrl+O for code snippets or long descriptions

---

## Troubleshooting

### Common Issues

**API Key Error:**
```
Error: API key not configured
Solution: export ANTHROPIC_API_KEY=sk-ant-xxxxx
```

**Connection Refused:**
```
Error: WebSocket connection failed
Solution: Check if port is available, try different port
```

**Permission Denied:**
```
Error: Tool execution denied
Solution: Use /permission to adjust settings
```

### Debug Mode

```bash
./bin/superterminal --debug --log-file debug.log
```

### Reset Configuration

```bash
rm ~/.superterminal/config.json
./bin/superterminal  # Will create fresh config
```

---

## Support

- **Documentation**: `docs/`
- **Issues**: GitHub Issues
- **Community**: Discussions

---

*SuperTerminal v0.4.0 - User Guide*