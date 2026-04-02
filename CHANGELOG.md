# Changelog

All notable changes to SuperTerminal will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.4.0] - 2026-04-02

### Added
- **Documentation & Release**
  - Complete README.md with feature comparison table
  - User guide documentation
  - Installation and release scripts
  - VERSION file for version management
  - CHANGELOG.md for tracking changes

## [v0.3.0] - 2026-04-02

### Added
- **User Experience Enhancements**
  - TUI: Command history navigation (↑/↓)
  - TUI: Autocomplete for commands (Tab)
  - TUI: Multiline input mode (Ctrl+O)
  - TUI: Confirmation dialogs
  - TUI: Progress indicators
  - Web UI: Dark/Light theme toggle
  - Web UI: Syntax highlighting for code blocks
  - Web UI: Copy code button with feedback
  - Web UI: Progress bar component

## [v0.2.0] - 2026-04-02

### Added
- **Performance Optimization**
  - Cache system (API, File, Tool result caches) - 19 tests
  - Concurrency utilities (ParallelExecutor, RateLimiter, CircuitBreaker) - 13 tests
  - Resource management (MemoryMonitor, SessionManager, LogRotator) - 14 tests
- **MCP Enhancement**
  - MCP client implementation (stdio/http/sse)
  - ClientManager for multiple connections
  - MCP commands (/mcp list/tools/resources/prompts)
  - Dynamic tool registration from MCP servers
  - 21 new MCP tests

### Changed
- Codebase grew to 15,586 lines Go + 1,724 lines Web UI
- 180 tests all passing

## [v0.1.0] - 2026-04-01

### Added
- **Core Engine**
  - Event bus for real-time updates
  - Claude API client with streaming
  - Session management
  - Configuration system
  - Tool execution framework
- **Terminal UI (Bubble Tea)**
  - Real-time message display
  - Streaming response animation
  - Tool execution panel
  - Status bar with cost tracking
- **Web UI**
  - WebSocket real-time sync
  - Session sidebar
  - Settings panel
  - 51KB HTML frontend
- **CLI & Config**
  - Command-line arguments parsing
  - JSON configuration file support
  - Environment initialization
- **Security**
  - Secure credential storage (PBKDF2 encryption)
  - Permission control framework
  - Audit logging
- **Persistence**
  - Session auto-save
  - Session search with match context
  - Export (text/json/markdown)
- **Tools**
  - bash, read, write, edit, glob, grep
  - web_search (DuckDuckGo HTML)
  - web_fetch (HTTP content fetch)

### Statistics
- 12,932 lines Go code
- 134 tests passing
- 10MB binary size

---

## Version History Summary

| Version | Date | Focus | Tests | Lines |
|:---|:---|:---|:---|:---|
| v0.4.0 | 2026-04-02 | Documentation & Release | 180 | 17,310 |
| v0.3.0 | 2026-04-02 | UX Enhancements | 180 | 17,310 |
| v0.2.0 | 2026-04-02 | Performance & MCP | 166 | 16,810 |
| v0.1.0 | 2026-04-01 | Initial Release | 134 | 12,932 |

---

## Roadmap

### Upcoming (v0.5.0)
- [ ] Image tool (read, describe, convert)
- [ ] Calendar integration
- [ ] Email tool
- [ ] Plugin system

### Future
- [ ] Multi-agent support
- [ ] Git integration
- [ ] Code review features
- [ ] Desktop app (Wails)
- [ ] Mobile companion app