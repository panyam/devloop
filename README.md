# devloop: Intelligent Development Workflow Orchestrator

![devloop Logo/Banner (Placeholder)](https://via.placeholder.com/1200x300?text=devloop+Logo)

`devloop` is a generic, multi-variant tool designed to streamline development workflows, particularly within Multi-Component Projects (MCPs). It combines functionalities inspired by live-reloading tools like `air` (for Go) and build automation tools like `make`, focusing on intelligent, configuration-driven orchestration of tasks based on file system changes.

## 🚀 Getting Started

1.  **Install `devloop`**:
    ```bash
    go install
    ```
2.  **Create a `.devloop.yaml` file in your project's root directory**:
    ```yaml
    rules:
      - name: "Go Backend Build and Run"
        watch:
          - action: "include"
            patterns:
              - "**/*.go"
              - "go.mod"
              - "go.sum"
        commands:
          - "echo 'Building backend...'"
          - "go build -o ./bin/server ./cmd/server"
          - "./bin/server"
    ```
3.  **Run `devloop`**:
    ```bash
    devloop -c .devloop.yaml
    ```

`devloop` will now watch your files and automatically rebuild and restart your backend server whenever you make changes to your Go code.

## ✨ Key Features

-   **Parallel & Concurrent Task Running**: Define rules for different parts of your project (backend, frontend, etc.) and `devloop` will run them concurrently.
-   **Intelligent Change Detection**: Uses glob patterns to precisely match file changes, triggering only the necessary commands.
-   **Robust Process Management**: Automatically terminates old processes before starting new ones, preventing zombie processes and ensuring a clean state.
-   **Debounced Execution**: Rapid file changes trigger commands only once, preventing unnecessary builds and restarts.
-   **Command Log Prefixing**: Prepends a customizable prefix to each line of your command's output, making it easy to distinguish logs from different processes.
-   **.air.toml Converter**: Includes a built-in tool to convert your existing `.air.toml` configuration into a `devloop` rule.

## ⚙️ `.devloop.yaml` Configuration Structure

The core of `devloop`'s behavior is defined by `rules` in a `.devloop.yaml` file. Each rule specifies what files to `watch` and what `commands` to execute when those files change.

```yaml
# .devloop.yaml example

settings:
  prefix_logs: true
  prefix_max_length: 15

rules:
  - name: "Go Backend"
    prefix: "backend"
    watch:
      - action: "include"
        patterns:
          - "**/*.go"
          - "go.mod"
          - "go.sum"
    commands:
      - "echo 'Building backend...'"
      - "go build -o ./bin/server ./cmd/server"
      - "./bin/server"

  - name: "Frontend Assets"
    prefix: "frontend"
    watch:
      - action: "include"
        patterns:
          - "web/static/**/*.css"
          - "web/static/**/*.js"
    commands:
      - "npm run build --prefix web/"

  - name: "Database Migrator"
    prefix: "db"
    watch:
      - action: "include"
        patterns:
          - "migrations/*.sql"
    commands:
      - "run-migrations.sh"
```

### Explanation of Fields:

-   `settings`: An optional top-level block for global settings.
    -   `prefix_logs`: A boolean to turn log prefixing on or off.
    -   `prefix_max_length`: An integer to pad or truncate all prefixes to a fixed length.
-   `rules`: A list of rules to execute.
    -   `name`: A unique identifier for the rule. Used internally for process management and logging.
    -   `prefix`: An optional string to use as the log prefix for this rule. If not set, the `name` is used.
    -   `watch`: A list of `Matcher` objects. Each `Matcher` has an `action` (`include` or `exclude`) and a list of `patterns`. The `watch` rules are evaluated in order, and the first one to match a file determines the action to take.
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

To run `devloop`, navigate to your project's root directory (where your `go.mod` and `.devloop.yaml` are located) and execute:

```bash
devloop -c .devloop.yaml
```

-   Use the `-c` flag to specify the path to your `.devloop.yaml` configuration file. If omitted, `devloop` will look for `.devloop.yaml` in the current directory.

`devloop` will start watching your files. When changes occur that match your defined rules, it will execute the corresponding commands. You will see log output indicating which rules are triggered and which commands are being run.

To stop `devloop` gracefully, press `Ctrl+C` (SIGINT). `devloop` will attempt to terminate any running child processes before exiting.

## 🛠️ Development Status & Roadmap

`devloop` is stable and ready for use. All core features are implemented and tested.

Future development will focus on:

-   **Enhanced User Experience**: Improving error messages, logging, and providing more detailed feedback.
-   **Advanced Configuration**: Exploring more powerful configuration options, such as rule dependencies or conditional execution.
-   **Plugin System**: A potential plugin system to allow for custom extensions and integrations.
-   **Broader Community Adoption**: Creating more examples and tutorials for different languages and frameworks.

## 🤝 Contributing

Contributions are welcome! Please feel free to open issues or submit pull requests.

## 📄 License

This project is licensed under the Apache License - see the [LICENSE](LICENSE) file for details.

