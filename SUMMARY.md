# Devloop Project Summary

This document summarizes the design, progress, and future plans for the `devloop` tool.

## 1. Project Vision & Core Idea

`devloop` is envisioned as a generic, multi-variant tool combining functionalities of `air` (Go's live-reloading tool) and `make` (a build automation tool). Its primary purpose is to act as an intelligent orchestrator for development workflows, especially within Multi-Component Projects (MCPs).

**Key Principles:**
- **Configuration-driven:** Behavior defined in `.devloop.yaml`.
- **Glob-based Triggers:** Actions initiated by file changes matching defined globs.
- **Unopinionated Actions:** The tool focuses on change detection and lifecycle management, not the semantics of the commands themselves. Commands can be any shell script (Go build, Python script, Node.js minifier, etc.).
- **Idempotency at Trigger Level:** Debouncing ensures a rule is triggered only once per set of rapid changes.
- **Robust Process Management:** Graceful termination of previously spawned processes (and their children) before re-execution.

## 2. Usefulness as an MCP Tool

`devloop` aims to streamline development in multi-component environments by:
- Providing a **unified development experience** from a single entry point.
- Enabling **intelligent, targeted rebuilds/restarts** based on specific file changes, avoiding unnecessary work.
- Managing **long-running processes** (e.g., backend servers, frontend dev servers) with automatic restarts.
- Offering **simplified configuration** for the entire development environment.
- Improving **resource efficiency** by only acting on affected components.

## 3. `.devloop.yaml` Configuration Structure

The tool's behavior is defined by `rules` in a `.devloop.yaml` file.

```yaml
# .devloop.yaml

rules:
  - name: "Go Backend Build and Run" # A unique name for this rule, used for process management
    watch:
      - "**/*.go"
      - "go.mod"
      - "go.sum"
    commands:
      - "echo 'Building backend...'"
      - "go build -o ./bin/server ./cmd/server"
      - "./bin/server" # This is a long-running process

  - name: "Frontend Assets Build"
    watch:
      - "web/static/**/*.css"
      - "web/static/**/*.js"
    commands:
      - "echo 'Rebuilding frontend assets...'"
      - "npm run build --prefix web/" # This is a short-lived process

  # ... other rules for WASM, docs, etc.
```

## 4. Execution Flow

1.  **Startup:** `devloop` reads `.devloop.yaml` and registers all defined `rules`.
2.  **File Watching:** A single file watcher monitors the project directory.
3.  **Event Processing:** When a file changes, `devloop` identifies *all* rules whose `watch` globs match the file.
4.  **Debouncing (per rule):** Each matched rule is debounced independently. If multiple files change rapidly, the affected rules are triggered only once after a configurable debounce period.
5.  **Action Execution (per rule):**
    *   Once a rule's debounce timer expires, any previously running processes for that rule are terminated (via `SIGTERM` to their process group).
    *   The `commands` for the rule are then executed sequentially in a new process group.

## 5. Incremental Build Plan

**Phase 1: Core Structure & Configuration**
1.  **Project Setup:** `devloop` directory, `go.mod`, `main.go` created. (✅ Done)
2.  **Configuration Definition:** Go structs for `Config` and `Rule` defined in `config.go`. YAML unmarshaling implemented. (✅ Done)
3.  **Test Configuration Loading:** Dummy `.devloop.yaml` created and `main.go` modified to load and print config. (✅ Done)

**Phase 2: Basic File Watching & Glob Matching**
4.  File Watching Integration (`fsnotify`). (✅ Done)
5.  Glob Matching Logic. (✅ Done)

**Phase 3: Command Execution & Basic Process Management**
6.  Simple Command Execution. (✅ Done)
7.  Sequential Command Execution. (✅ Done)

**Phase 4: Debouncing & Advanced Process Management**
8.  Debouncing Implementation. (✅ Done)
9.  Process Group Management. (✅ Done)

**Phase 5: Robustness, CLI & Polish**
10. Error Handling & Logging. (✅ Done)
11. CLI Arguments. (✅ Done)
12. Graceful Shutdown. (⏳ In Progress)
13. Documentation & Testing. (⏳ In Progress)

## 6. Progress & Next Steps

**Current Status:**
- All core functionalities (configuration loading, file watching, glob matching, command execution, debouncing, and process management) are implemented and covered by unit and end-to-end tests.
- CLI argument parsing for the config file path is implemented.
- Error handling and logging have been improved.

**Next Steps:**
- Implement graceful shutdown to handle OS signals (SIGINT/SIGTERM) for clean termination.
- Continue with general documentation and testing improvements.
