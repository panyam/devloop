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

## 4. Progress & Next Steps

**Current Status:**
- All core functionalities (configuration loading, file watching, glob matching, command execution, debouncing, and process management) are implemented.
- The architecture has been successfully migrated from a simple HTTP server to a unified gRPC and gRPC-Gateway foundation.
- The three operating modes (`standalone`, `agent`, `gateway`) are defined at the CLI level.
- The `standalone` mode is functional and tested.
- The test suite has been significantly refactored for robustness, using a test context helper to manage timeouts and temporary environments.

**Next Steps:**
- Finalize the implementation and testing for the `agent` and `gateway` modes.
- Add comprehensive tests for the gRPC API endpoints.
- Continue to improve documentation and user guides for the new architecture.