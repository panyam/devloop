# devloop: Intelligent Development Workflow Orchestrator

![devloop Logo/Banner (Placeholder)](https://via.placeholder.com/1200x300?text=devloop+Logo)

`devloop` is a generic, multi-variant tool designed to streamline development workflows, particularly within Multi-Component Projects (MCPs). It combines functionalities inspired by live-reloading tools like `air` (for Go) and build automation tools like `make`, focusing on intelligent, configuration-driven orchestration of tasks based on file system changes.

## 🚀 Project Vision & Core Idea

`devloop` aims to be an intelligent orchestrator for your development environment. Instead of manually running build commands or restarting services after every code change, `devloop` automates these repetitive tasks by watching your file system and triggering predefined actions.

### Key Principles:

-   **Configuration-driven:** All behavior is defined in a simple `multi.yaml` file.
-   **Glob-based Triggers:** Actions are initiated by file changes that match defined glob patterns.
-   **Unopinionated Actions:** `devloop` focuses on change detection and lifecycle management, not the semantics of the commands themselves. Commands can be any shell script (e.g., `go build`, `npm run build`, Python scripts, Docker commands).
-   **Idempotency at Trigger Level:** Debouncing ensures that a rule is triggered only once per set of rapid changes, preventing redundant executions.
-   **Robust Process Management:** It gracefully terminates previously spawned processes (and their children) before re-executing commands, ensuring clean restarts and preventing orphaned processes.

## ✨ Usefulness as an MCP Tool

`devloop` is especially beneficial in multi-component environments by:

-   Providing a **unified development experience** from a single entry point.
-   Enabling **intelligent, targeted rebuilds/restarts** based on specific file changes, avoiding unnecessary work.
-   Managing **long-running processes** (e.g., backend servers, frontend development servers) with automatic restarts.
-   Offering **simplified configuration** for the entire development environment.
-   Improving **resource efficiency** by only acting on affected components.

## ⚙️ `multi.yaml` Configuration Structure

The core of `devloop`'s behavior is defined by `rules` in a `multi.yaml` file. Each rule specifies what files to `watch` and what `commands` to execute when those files change.

```yaml
# multi.yaml example

rules:
  - name: "Go Backend Build and Run" # A unique name for this rule, used for process management
    watch:
      - "**/*.go"
      - "go.mod"
      - "go.sum"
    commands:
      - "echo 'Building backend...'"
      - "go build -o ./bin/server ./cmd/server"
      - "./bin/server" # This is a long-running process that devloop will manage

  - name: "Frontend Assets Build" # Another rule for web assets
    watch:
      - "web/static/**/*.css"
      - "web/static/**/*.js"
    commands:
      - "echo 'Rebuilding frontend assets...'"
      - "npm run build --prefix web/" # This is a short-lived process

  # ... you can add more rules for WASM, documentation, etc.
```

### Explanation of Fields:

-   `name`: A unique identifier for the rule. Used internally for process management and logging.
-   `watch`: A list of glob patterns. When any file matching these patterns is created, modified, or deleted, the rule's commands are considered for execution.
-   `commands`: A list of shell commands to execute sequentially when the rule is triggered. These commands are run in a new process group, allowing `devloop` to manage their lifecycle.

## 📦 Installation

To install `devloop`, ensure you have Go (version 1.20 or higher) installed and your `$GOPATH/bin` (or `$GOBIN`) is in your system's PATH.

```bash
go install
```

This command will compile the `devloop` executable and place it in your Go binary directory, making it globally accessible.

Alternatively, to build the executable locally:

```bash
go build -o devloop
```

## 🚀 Usage

To run `devloop`, navigate to your project's root directory (where your `go.mod` and `multi.yaml` are located) and execute:

```bash
devloop -c multi.yaml
```

-   Use the `-c` flag to specify the path to your `multi.yaml` configuration file. If omitted, `devloop` will look for `multi.yaml` in the current directory.

`devloop` will start watching your files. When changes occur that match your defined rules, it will execute the corresponding commands. You will see log output indicating which rules are triggered and which commands are being run.

To stop `devloop` gracefully, press `Ctrl+C` (SIGINT). `devloop` will attempt to terminate any running child processes before exiting.

## 🛠️ Development Status & Roadmap

`devloop` is currently in active development. The core functionalities are implemented and tested:

-   Configuration loading and parsing.
-   File watching and glob matching.
-   Command execution and process management (including termination of previous processes).
-   Debouncing of rapid file change events.
-   CLI argument parsing.
-   Graceful shutdown handling (SIGINT/SIGTERM).

### Next Steps:

-   Further refine error handling and logging for user-friendliness.
-   Explore advanced process management features (e.g., process groups for better child process control).
-   Add more comprehensive documentation and examples.
-   Consider cross-platform compatibility testing.

## 🤝 Contributing

Contributions are welcome! Please feel free to open issues or submit pull requests.

## 📄 License

This project is licensed under the Apache License - see the [LICENSE](LICENSE) file for details. (Note: A LICENSE file is not yet present in the repository, but this is a placeholder for future inclusion.)
