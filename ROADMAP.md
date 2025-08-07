# Devloop Project Roadmap

This document outlines the high-level roadmap for the `devloop` project.

## Phase 1: Core Functionality (Complete)

- [x] **Configuration Loading:** Define and parse `.devloop.yaml` files, resolving relative paths.
- [x] **File Watching:** Monitor file systems for changes using `fsnotify`.
- [x] **Glob Matching:** Match file changes against user-defined glob patterns (both relative and absolute).
  - [x] Migrated to `doublestar` library for conventional `**` glob behavior
  - [x] Pattern `src/**/*.go` now matches both `src/main.go` and `src/pkg/utils.go`
- [x] **Command Execution:** Run shell commands in response to file changes.
- [x] **Process Management:** Gracefully terminate and restart long-running processes.
- [x] **Debouncing:** Prevent command storms by debouncing file change events.
- [x] **CLI Arguments:** Basic command-line interface for specifying configuration files.
- [x] **Graceful Shutdown:** Handle `SIGINT` and `SIGTERM` to shut down cleanly.
- [x] **gRPC/HTTP API:** Implement a unified API layer using gRPC and a gRPC-Gateway for monitoring and interaction.
- [x] **Test Suite:** Comprehensive test coverage with all tests passing.
- [x] **Code Organization:** Restructured into `agent/`, `gateway/`, and `utils/` directories.

## Phase 2: Multi-Instance Architecture & Usability (In Progress)

- [x] **Architecture Refactoring:**
  - [x] Separated concerns: Orchestrator (file watching) and RuleRunner (command execution)
  - [x] Simplified to single orchestrator implementation with Agent Service integration
  - [x] Created comprehensive testing infrastructure
  - [x] Rule-specific configuration (debounce delay, verbose logging)
- [x] **Process Management Improvements:**
  - [x] Fixed zombie process issues with proper signal handling
  - [x] Implemented sequential command execution with failure propagation
  - [x] Platform-specific process management (Linux, Darwin, Windows)
- [x] **Testing Infrastructure:**
  - [x] Simplified testing infrastructure for single orchestrator implementation
  - [x] Comprehensive test coverage for all core functionality
  - [x] Cross-platform testing support
- [x] **Port Configuration & Auto-Discovery:**
  - [x] Updated default ports to avoid common conflicts (HTTP: 8080→9999, gRPC: 50051→5555)
  - [x] Implemented automatic port discovery with --auto-ports flag
  - [x] Fast TCP bind/close port availability checking
  - [x] Fallback port search with configurable range limits
- [ ] **Gateway Mode (grpcrouter-based):**
  - [ ] Research and integrate grpcrouter library for automatic gateway/proxy functionality
  - [ ] Implement simplified gateway mode using grpcrouter instead of custom implementation
  - [ ] Add agent connection logic for grpcrouter-based gateway
  - [ ] Add comprehensive tests for distributed mode operations with new architecture
  - [x] Agent Service integration completed (provides foundation for gateway)
- [ ] **Enhanced Logging:**
  - [ ] Structured logging (e.g., JSON) for machine-readability.
  - [ ] Log filtering and searching capabilities via the API.
- [x] **Improved CLI (Completed 2025-07-07):**
  - [x] **Cobra Framework Integration:** Complete CLI restructuring with modern subcommand architecture
  - [x] **Client Commands:** `devloop config`, `devloop status`, `devloop trigger`, `devloop paths`
  - [x] **Server Command:** `devloop server` with all original functionality
  - [x] **Default Behavior:** `devloop` starts server, subcommands act as gRPC client
  - [x] **Signal Handling:** Fixed Ctrl-C interrupt handling for proper server shutdown
  - [x] **Code Organization:** Modular structure with `cmd/`, `server/`, `client/` packages
  - [x] **Professional UX:** Help system, usage examples, and consistent flag handling
  - [x] **Project Initialization (Completed 2025-08-07):** Added `devloop init` command with embedded profile templates
    - [x] **Profile-Based Bootstrapping:** Pre-configured templates for Go, TypeScript, and Python Flask projects
    - [x] **Multiple Profile Support:** Single command multi-service setup (`devloop init go ts py`)
    - [x] **Extensible Architecture:** Easy addition of new profiles via `cmd/profiles/` YAML files
- [x] **Cycle Detection & Prevention (Completed 2025-07-08):**
  - [x] **Static Validation:** Startup detection of self-referential patterns in rule configurations
  - [x] **Dynamic Rate Limiting:** TriggerTracker with frequency monitoring and exponential backoff
  - [x] **Cross-Rule Cycle Detection:** Trigger chain tracking with configurable max depth limits
  - [x] **File Thrashing Detection:** Sliding window frequency analysis for rapid file modifications
  - [x] **Emergency Cycle Breaking:** Rule disabling and cycle resolution suggestions
  - [x] **Configuration Parser Fix:** Fixed YAML-to-protobuf parsing for cycle_detection settings
  - [x] **Comprehensive Configuration:** Full cycle_detection settings block with all parameters
- [x] **Startup Resilience & Retry Logic (Completed 2025-07-16):**
  - [x] **Exponential Backoff Retries:** Configurable retry logic for failed rule startup with exponential backoff
  - [x] **Graceful Failure Handling:** Rules fail independently without stopping devloop unless explicitly configured
  - [x] **Configurable Exit Behavior:** `exit_on_failed_init` flag for critical rules that must succeed
  - [x] **Retry Configuration:** `max_init_retries` and `init_retry_backoff_base` for fine-tuned retry behavior
  - [x] **Comprehensive Logging:** Detailed retry attempt logging with next retry time and success notifications
  - [x] **Backward Compatibility:** Default behavior allows devloop to continue running despite startup failures
  - [x] **Non-Blocking Implementation:** Fixed blocking issue by moving retry logic to background execution
- [ ] **Web-based UI:**
  - [ ] A simple web interface, built on the Agent Service HTTP API, to visualize rule status, logs, and trigger commands manually.

## Phase 3: Ecosystem & Integration (Future)

- [ ] **Enhanced MCP Integration:**
  - [ ] Add streaming log support to MCP tools for real-time monitoring
  - [ ] Implement additional MCP resources for project files and configurations
  - [ ] Explore MCP prompts for common development workflows

- [ ] **IDE Integration:**
  - [ ] Plugins for popular IDEs (e.g., VS Code, GoLand) to provide a seamless development experience.
- [ ] **Cloud-Native Development:**
  - [ ] Integration with tools like Docker and Kubernetes for containerized development workflows.
- [ ] **Performance Optimization:**
  - [ ] Optimize file watching and command execution for large projects.
- [ ] **Community & Documentation:**
  - [ ] Build a community around `devloop` and create comprehensive documentation and tutorials for the new architecture.
