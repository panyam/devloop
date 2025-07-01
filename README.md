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

-   `settings`: An optional top-level block for global settings that influence `devloop`'s behavior across all rules.
    -   `prefix_logs`: A boolean (`true` or `false`) to enable or disable log prefixing for all command outputs. When `true` (default), `devloop` prepends each line of output with the rule's `prefix` (or `name` if `prefix` is not set), making it easier to distinguish logs from concurrent processes.
    -   `prefix_max_length`: An integer that specifies the maximum length for all log prefixes. If a prefix is shorter than this length, it will be padded with spaces. If it's longer, it will be truncated. This helps in aligning log outputs for better readability, especially when multiple rules are running concurrently.
-   `rules`: A list of rules to execute.
    -   `name`: A unique identifier for the rule. Used internally for process management and logging.
    -   `prefix`: An optional string to use as the log prefix for this rule. If not set, the `name` is used.
    -   `watch`: A list of `Matcher` objects. Each `Matcher` has an `action` (`include` or `exclude`) and a list of `patterns`. The `watch` rules are evaluated in order, and the first one to match a file determines the action to take. This allows for fine-grained control over which files trigger a rule. For example, to include all `.go` files except those in a `vendor` directory:

        ```yaml
        watch:
          - action: "exclude"
            patterns:
              - "vendor/**/*.go"
          - action: "include"
            patterns:
              - "**/*.go"
        ```
        In this example, any `.go` file within the `vendor` directory will be excluded due to the first rule, while all other `.go` files will be included by the second rule.
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

### Subcommands

`devloop` also supports subcommands for specific utilities:

#### `convert`

This subcommand allows you to convert an existing `.air.toml` configuration file (used by the `air` live-reloading tool) into a `devloop`-compatible `.devloop.yaml` rule.

```bash
devloop convert -i .air.toml
```

-   Use the `-i` flag to specify the path to the `.air.toml` input file. If omitted, it defaults to `.air.toml` in the current directory. The converted output will be printed to standard output.

### Running the Orchestrator

`devloop` will start watching your files. When changes occur that match your defined rules, it will execute the corresponding commands. You will see log output indicating which rules are triggered and which commands are being run.

To stop `devloop` gracefully, press `Ctrl+C` (SIGINT). `devloop` will attempt to terminate any running child processes before exiting, ensuring a clean shutdown of your development environment.

## 💡 Troubleshooting and Logging

`devloop` provides detailed log output to help you understand its behavior and diagnose issues.

-   **Interpreting Logs**:
    -   `[devloop]` prefixed messages indicate internal `devloop` operations (e.g., "Starting orchestrator...", "Shutting down...").
    -   Messages prefixed with a rule's `name` or `prefix` (e.g., `[backend]`, `[frontend]`) are outputs from the commands executed by that specific rule.
    -   Look for `ERROR` or `FATAL` messages in the logs for critical issues.

-   **Common Issues**:
    -   **Commands not running**:
        -   Check your `.devloop.yaml` for correct `watch` patterns. Ensure the patterns accurately match the files you are changing.
        -   Verify that the `commands` themselves are correct and executable in your shell.
        -   Ensure `devloop` is running in the correct directory where your `.devloop.yaml` and watched files reside.
    -   **Processes not terminating**:
        -   If child processes are not shutting down gracefully, ensure your commands respond to `SIGTERM` (Ctrl+C). `devloop` sends `SIGTERM` to the process group.
    -   **Configuration errors**:
        -   `devloop` will log an error if your `.devloop.yaml` has syntax issues or incorrect field types. Review the "`.devloop.yaml` Configuration Structure" section carefully.

If you encounter persistent issues, consider running `devloop` with increased verbosity (if a future flag is added for it) or checking the `devloop` GitHub repository for known issues and community support.

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

