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

`devloop` has evolved to a gRPC-based architecture to provide a robust and flexible API for monitoring and interaction. The API is defined in Protobuf (`protos/devloop/v1/devloop_gateway.proto`) and exposed via a gRPC-Gateway, providing both gRPC and RESTful HTTP/JSON endpoints.

The tool can operate in three distinct modes:

1.  **Standalone Mode (Default):**
    *   This is the standard mode for individual projects.
    *   `devloop` runs as a single daemon, watching files and executing commands as defined in `.devloop.yaml`.
    *   It runs an **in-process gRPC server and gateway**, allowing you to interact with it via the gRPC or HTTP API (e.g., to check status or trigger rules from a script).

2.  **Agent Mode:**
    *   In this mode, the `devloop` instance does *not* host its own server.
    *   Instead, it connects as a client to a central `devloop` instance running in **Gateway Mode**.
    *   It registers itself and streams its logs and status updates to the central gateway. This is ideal for MCPs where you want a single point of control and observation.

3.  **Gateway Mode:**
    *   This instance acts as a central hub.
    *   It runs the gRPC server and gateway, but does not perform any file watching or command execution itself.
    *   Its primary role is to accept connections from multiple `devloop` instances running in **Agent Mode**, aggregate their logs and statuses, and provide a unified API for clients to interact with the entire project ecosystem.

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

**OrchestratorV2 Architecture (as of 2025-07-03):**
- **Separation of Concerns:** File watching (Orchestrator) is now separate from command execution (RuleRunner)
- **RuleRunner Pattern:** Each rule has its own RuleRunner instance managing its lifecycle, debouncing, and process management
- **Improved Process Management:** Platform-specific handling (Linux uses Pdeathsig, Darwin uses Setpgid) prevents zombie processes
- **Sequential Execution:** Commands within a rule execute sequentially with proper failure propagation (like GNU Make)
- **Testing Infrastructure:** Factory pattern allows testing both v1 and v2 implementations side-by-side

## 6. Configuration Enhancements

**Rule-Specific Settings:**
```yaml
rules:
  - name: "backend"
    debounce_delay: 1s    # Override default debounce
    verbose: true         # Enable verbose logging for this rule
    commands: [...]
```

**Global Defaults:**
```yaml
settings:
  default_debounce_delay: 500ms
  verbose: false
  prefix_logs: true
  prefix_max_length: 10
```

## 7. Progress & Next Steps

**Current Status (as of 2025-07-03):**
- ✅ All core functionalities fully implemented and tested
- ✅ Dual orchestrator architecture (v1 and v2) with comprehensive testing
- ✅ Process management issues resolved (no more zombie processes)
- ✅ Sequential command execution with failure propagation
- ✅ Rule-specific configuration for fine-grained control
- ✅ Test infrastructure supporting both implementations:
  - `make test` - runs all tests against both versions
  - `make testv1` - tests v1 orchestrator only
  - `make testv2` - tests v2 orchestrator only
- ✅ Complete gateway integration for OrchestratorV2 (all handler methods ported)
- ✅ All tests passing for both v1 and v2 implementations

**Next Steps:**
- Performance benchmarking between v1 and v2
- Migration guide for switching to v2 as default
- Switch default orchestrator to v2
- Finalize the implementation and testing for the `agent` and `gateway` modes
- Add comprehensive tests for the gRPC API endpoints