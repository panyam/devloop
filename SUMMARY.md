# Devloop Project Summary

This document summarizes the design, progress, and future plans for the `devloop` tool.

## 1. Project Vision & Core Idea

`devloop` is envisioned as a generic, multi-variant tool combining functionalities of `air` (Go's live-reloading tool) and `make` (a build automation tool). Its primary purpose is to act as an intelligent orchestrator for development workflows, especially within Multi-Component Projects (MCPs).

**Key Principles:**
- **Configuration-driven:** Behavior defined in `.devloop.yaml`.
- **Glob-based Triggers:** Actions initiated by file changes matching defined globs. Paths can be relative to the config file or absolute.
- **Unopinionated Actions:** The tool focuses on change detection and lifecycle management, not the semantics of the commands themselves. Commands can be any shell script.
- **Idempotency at Trigger Level:** Debouncing ensures a rule is triggered only once per set of rapid changes.
- **Robust Process Management:** Graceful termination of previously spawned processes (and their children) before re-execution.

## 2. Architecture & Operating Modes

`devloop` has evolved to a gRPC-based architecture to provide a robust and flexible API for monitoring and interaction. The API is defined in Protobuf (`protos/devloop/v1/agents.proto`) and exposed via a gRPC-Gateway, providing both gRPC and RESTful HTTP/JSON endpoints.

**Current Architecture:**
The system consists of three main components:
1. **Orchestrator**: File watching daemon that monitors changes and manages rule execution
2. **Agent Service**: gRPC service that provides API access to the orchestrator
3. **Main**: Entry point that coordinates both the orchestrator and agent service

**MCP (Model Context Protocol) Integration:**
As of 2025-07-07, devloop supports simplified MCP integration for AI-powered development automation. The MCP server is enabled with the `--enable-mcp` flag and runs as an HTTP handler alongside the gRPC gateway. Tools are auto-generated from protobuf definitions using protoc-gen-go-mcp. Uses modern StreamableHTTP transport (MCP 2025-03-26 spec) for maximum compatibility.

The tool can operate in three distinct modes:

1.  **Standalone Mode (Default):**
    *   This is the standard mode for individual projects.
    *   `devloop` runs as a single daemon, watching files and executing commands as defined in `.devloop.yaml`.
    *   It runs an **in-process gRPC server and gateway**, allowing you to interact with it via the gRPC or HTTP API (e.g., to check status or trigger rules from a script).
    *   MCP integration is available via the `/mcp` HTTP endpoint when enabled.

2.  **Agent Mode:**
    *   In this mode, the `devloop` instance connects as a client to a central `devloop` instance running in **Gateway Mode**.
    *   It registers itself and streams its logs and status updates to the central gateway. This is ideal for multi-project setups where you want a single point of control and observation.

3.  **Gateway Mode:**
    *   This instance acts as a central hub.
    *   It runs the gRPC server and gateway, but does not perform any file watching or command execution itself.
    *   Its primary role is to accept connections from multiple `devloop` instances running in **Agent Mode**, aggregate their logs and statuses, and provide a unified API for clients to interact with the entire project ecosystem.
    *   **Note**: Gateway mode is temporarily removed and will be reimplemented using the grpcrouter library for simplified proxy and reverse tunnel functionality.

## 3. Execution Flow (Standalone Mode)

1.  **Startup:** `devloop` reads `.devloop.yaml`. Relative `watch` paths are resolved to absolute paths based on the config file's location.
2.  **Server Start:** The combined gRPC server and gRPC-Gateway proxy is started in the background.
3.  **File Watching:** A single file watcher monitors the project directory.
4.  **Event Processing:** When a file changes, `devloop` identifies all rules whose `watch` globs match the file's absolute path.
5.  **Debouncing (per rule):** Each matched rule is debounced independently.
6.  **Action Execution (per rule):**
    *   Once a rule's debounce timer expires, any previously running processes for that rule are terminated.
    *   The `commands` for the rule are then executed sequentially.

## 4. Key Technical Decisions

**Glob Pattern Matching:**
- Switched from `gobwas/glob` to `bmatcuk/doublestar` library for conventional glob behavior
- Pattern `**` now matches zero or more directories (following git, VS Code, and other tools' conventions)
- Example: `src/**/*.go` matches both `src/main.go` AND `src/pkg/utils.go`

**File Watching:**
- File watcher starts from the project root (directory containing the config file)
- All relative patterns in config are resolved to absolute paths relative to the config file location
- This ensures consistent behavior regardless of where devloop is executed from

**Code Organization:**
- Core logic moved to `agent/` directory for better modularity
- `main.go` remains at project root as the entry point
- Utilities moved to `utils/` directory
- Clear separation between agent logic, gateway logic, and main application

## 5. Architecture Evolution

**Current Orchestrator Architecture (as of 2025-07-07):**
- **Simplified Architecture:** Single orchestrator implementation with no version distinction
- **Separation of Concerns:** File watching (Orchestrator) is separate from command execution (RuleRunner)
- **RuleRunner Pattern:** Each rule has its own RuleRunner instance managing its lifecycle, debouncing, and process management
- **Agent Service Integration:** gRPC service provides API access to orchestrator functionality
- **Improved Process Management:** Platform-specific handling (Linux uses Pdeathsig, Darwin uses Setpgid) prevents zombie processes
- **Sequential Execution:** Commands within a rule execute sequentially with proper failure propagation (like GNU Make)
- **Gateway Preparation:** Architecture designed to support future grpcrouter-based gateway implementation
- **Simplified MCP Integration:** MCP server runs as HTTP handler using the same Agent Service

## 6. Configuration Enhancements

**Rule-Specific Settings:**
```yaml
rules:
  - name: "backend"
    debounce_delay: 1s    # Override default debounce
    verbose: true         # Enable verbose logging for this rule
    color: "blue"         # Custom color for this rule's output
    workdir: "./backend"  # Custom working directory
    commands: [...]
```

**Global Defaults:**
```yaml
settings:
  default_debounce_delay: 500ms
  verbose: false
  prefix_logs: true
  prefix_max_length: 10
  color_logs: true              # Enable colored output
  color_scheme: "auto"          # Auto-detect terminal theme
  custom_colors:                # Custom color mappings
    backend: "blue"
    frontend: "green"
```

## 7. Progress & Next Steps

**Current Status (as of 2025-08-01):**
- ✅ All core functionalities fully implemented and tested
- ✅ **Simplified Single Orchestrator Architecture:** Removed version distinction and simplified codebase
- ✅ **Agent Service Integration:** New gRPC service provides API access to orchestrator
- ✅ **Fixed Rule Matching Logic:** Resolved critical bug where exclude patterns were ignored
- ✅ Process management issues resolved (no more zombie processes)
- ✅ Sequential command execution with failure propagation
- ✅ Cross-platform command execution (Windows, macOS, Linux)
- ✅ **Color-coded rule output with configurable schemes:** Fixed YAML parsing and TTY detection issues
- ✅ Rule-specific configuration for fine-grained control
- ✅ **Action-Based File Filtering:** Rules now properly respect include/exclude actions
- ✅ **Configurable Default Behavior:** Rule-level and global `default_action` settings
- ✅ All tests passing with simplified test infrastructure
- ✅ **Port Configuration & Auto-Discovery:** Updated defaults to avoid conflicts, added automatic port discovery
- ✅ **Gateway Mode Preparation:** Architecture ready for grpcrouter-based gateway implementation
- ✅ **Simplified MCP Integration:** MCP server integration completed:
  - **MCP as HTTP Handler:** Enabled with `--enable-mcp` flag, runs on `/mcp` endpoint
  - Auto-generated MCP tools from protobuf definitions using protoc-gen-go-mcp
  - **StreamableHTTP Transport:** Uses MCP 2025-03-26 specification for modern compatibility
  - **Stateless Design:** No sessionId requirement - compatible with Claude Code and other MCP clients
  - **Simplified Architecture:** Uses same Agent Service as gRPC API
  - Core tools: GetConfig, GetRule, ListWatchedPaths, TriggerRule, StreamLogs
  - Manual project ID configuration support for consistent AI tool identification
- ✅ **Non-Blocking Auto-Restart:** Fixed critical blocking issue in startup retry system
  - **Problem Fixed:** Auto-restart was blocking file watching, preventing responses to file changes during startup
  - **Solution:** Moved retry logic from orchestrator startup to background rule execution
  - **Non-Blocking Startup:** File watching starts immediately while rules initialize in background
  - **Background Retry Logic:** Rules retry startup failures using exponential backoff in background goroutines
  - **Critical Failure Handling:** Added `criticalFailure` channel for rules with `exit_on_failed_init: true`
  - **Preserved Features:** All existing retry configuration and logging preserved
  - **Test Fix:** Corrected `TestDebouncing` to use proper `skip_run_on_init: true` configuration
- ✅ **Startup Resilience & Retry Logic:** Comprehensive startup retry system with exponential backoff
  - **Exponential Backoff Retries:** Configurable retry logic for failed rule startup (default: 10 attempts, 3s base backoff)
  - **Graceful Failure Handling:** Rules fail independently without stopping devloop unless explicitly configured
  - **Configurable Exit Behavior:** `exit_on_failed_init` flag for critical rules that must succeed
  - **Comprehensive Logging:** Detailed retry attempt logging with next retry time and success notifications
  - **Backward Compatibility:** Default behavior allows devloop to continue running despite startup failures

**Major Bug Fixes:**
- **Rule Matching Logic (Critical):** Fixed orchestrator ignoring `Action` field in matchers
  - Before: Exclude patterns matched but still triggered rules
  - After: Exclude patterns properly skip rule execution
  - Impact: Projects with exclusion patterns now work correctly
- **Port Configuration & Auto-Discovery (Critical):** Eliminated most common startup failure cause
  - Before: Default ports 8080/50051 frequently conflicted with other services
  - After: New defaults 9999/5555 with --auto-ports flag for automatic conflict resolution
  - Impact: `devloop` now starts reliably without manual port configuration
- **Architecture Simplification (Critical):** Removed complexity from dual orchestrator implementations
  - Before: Complex factory pattern with V1/V2 switching
  - After: Single orchestrator implementation with Agent Service wrapper
  - Impact: Cleaner codebase and easier maintenance
- **MCP Integration Simplification (Enhancement):** Streamlined MCP server implementation
  - Before: Complex MCP mode with separate service management
  - After: Simple HTTP handler using existing Agent Service
  - Impact: Easier maintenance and better integration with core functionality
- **Startup Resilience Enhancement (Critical):** Added comprehensive retry logic for startup failures
  - Before: Devloop quit entirely if any rule failed during startup, preventing development workflow
  - After: Rules retry with exponential backoff (3s, 6s, 12s...), devloop continues running even with failed rules
  - Impact: Major usability improvement - eliminates frustration of devloop quitting on transient startup failures
- **Non-Blocking Auto-Restart Fix (Critical):** Fixed auto-restart blocking file watching behavior
  - Before: Auto-restart blocked file watching startup, preventing response to file changes during rule initialization
  - After: File watching starts immediately while rules initialize in background with retry logic
  - Impact: File changes now trigger during startup, enabling continuous development workflow
- **Color Scheme Prefix Fix (Enhancement):** Fixed color configuration parsing and TTY detection
  - Before: Color schemes not working on prefixes despite `color_logs: true` configuration due to YAML parsing and TTY detection issues
  - After: Proper YAML-to-protobuf parsing for color fields and global TTY control instead of per-subprocess checks
  - Impact: Rule prefixes now display in configured colors with proper ANSI codes for enhanced visual distinction
- **Subprocess Color Suppression Fix (Enhancement):** Fixed devloop suppressing colors from subprocess output
  - Before: Subprocess tools (npm, go test, etc.) output appeared in black and white while devloop prefixes were colored
  - After: Decoupled devloop's color control from subprocess color decisions, added environment variables for color detection
  - Impact: Full color preservation - npm errors show in red, success in green, while devloop prefixes maintain their colors

**Current Architecture Strengths:**
- **Simplified Single Implementation:** Single orchestrator implementation with no version complexity
- **Correct Pattern Matching:** First-match semantics with proper action-based filtering
- **Agent Service Integration:** Clean gRPC service layer providing API access to orchestrator
- **Streamlined MCP Integration:** MCP runs as HTTP handler using existing Agent Service
- **Auto-generated MCP Tools:** Leverages protoc-gen-go-mcp for automatic tool generation from protobuf
- **Modern MCP Transport:** StreamableHTTP (2025-03-26) for stateless, serverless-ready deployment
- **Universal Client Compatibility:** Works with Claude Code, Python SDK, and other MCP implementations
- **Port Conflict Resolution:** Automatic port discovery eliminates common startup failures
- **User-Friendly Defaults:** Non-conflicting default ports (9999/5555) for seamless operation
- **Modular Design:** Clear separation between file watching (Orchestrator) and API access (Agent Service)
- **Gateway Ready:** Architecture prepared for future grpcrouter-based gateway implementation
- **Comprehensive Testing:** All core functionality covered with automated tests
- **Cross-Platform Support:** Works reliably on Windows, macOS, and Linux
- **Flexible Configuration:** Rule-level and global configuration options

**Next Steps:**
- ✅ Orchestrator simplification completed
- ✅ Rule matching logic fixed
- ✅ Port configuration & auto-discovery implemented
- ✅ Agent Service integration completed
- ✅ MCP integration simplified and working
- ✅ **CLI Restructuring with Cobra (2025-07-07):** Complete refactoring to modern CLI structure
  - **Professional CLI Experience:** Replaced flag-based CLI with Cobra framework
  - **Subcommand Architecture:** Added client commands for interacting with running server
  - **Default Behavior:** `devloop` starts server, subcommands act as client
  - **Signal Handling Fix:** Resolved Ctrl-C interrupt issues in server mode
  - **Code Organization:** Modular structure with `cmd/`, `server/`, `client/` packages
- ✅ **Watcher Robustness & Pattern Resolution Fix (2025-07-08):** Fixed critical pattern resolution and watcher reliability issues
  - **Workdir-Relative Patterns:** Patterns now resolve relative to rule's working directory instead of project root
  - **Dynamic Directory Watching:** Added support for directories created/deleted after startup
  - **Pattern-Based Exclusions:** Replaced hard-coded exclusions with intelligent pattern-based logic
  - **Cross-Rule Isolation:** Rules with different workdirs now properly isolate their pattern matching
  - **Comprehensive Testing:** Added tests for workdir-relative patterns and pattern resolution
- Implement grpcrouter-based gateway mode
- Add comprehensive tests for the gRPC API endpoints
- Enhance Agent Service with streaming capabilities
- Add distributed logging and monitoring for gateway mode
- Consider adding streaming log support to MCP tools