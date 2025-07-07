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
- [ ] **Improved CLI:**
  - [ ] Subcommands for common tasks (e.g., `devloop init`, `devloop status`).
  - [ ] More flexible flag options for the different modes.
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
