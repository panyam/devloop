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

- [ ] **Agent & Gateway Modes:**
  - [ ] Finalize implementation for the `agent` mode, allowing a `devloop` instance to connect to a central gateway.
  - [ ] Finalize implementation for the `gateway` mode, allowing a `devloop` instance to act as a central hub for multiple agents.
  - [ ] Add comprehensive tests for agent-gateway communication and interaction.
- [ ] **Enhanced Logging:**
  - [ ] Structured logging (e.g., JSON) for machine-readability.
  - [ ] Log filtering and searching capabilities via the API.
- [ ] **Improved CLI:**
  - [ ] Subcommands for common tasks (e.g., `devloop init`, `devloop status`).
  - [ ] More flexible flag options for the different modes.
- [ ] **Web-based UI:**
  - [ ] A simple web interface, built on the gRPC-Gateway, to visualize rule status, logs, and trigger commands manually.

## Phase 3: Ecosystem & Integration (Future)

- [ ] **IDE Integration:**
  - [ ] Plugins for popular IDEs (e.g., VS Code, GoLand) to provide a seamless development experience.
- [ ] **Cloud-Native Development:**
  - [ ] Integration with tools like Docker and Kubernetes for containerized development workflows.
- [ ] **Performance Optimization:**
  - [ ] Optimize file watching and command execution for large projects.
- [ ] **Community & Documentation:**
  - [ ] Build a community around `devloop` and create comprehensive documentation and tutorials for the new architecture.
