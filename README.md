# devloop: Intelligent Development Workflow Orchestrator

![devloop Logo/Banner (Placeholder)](https://via.placeholder.com/1200x300?text=devloop+Logo)

`devloop` is a generic, multi-variant tool designed to streamline development workflows, particularly within Multi-Component Projects (MCPs) (no not *that* MCP). It combines functionalities inspired by live-reloading tools like `air` (for Go) and build automation tools like `make`, focusing on simple, configuration-driven orchestration of tasks based on file system changes.

## 🏗️ Architecture Overview

Devloop operates in three distinct modes to support different development scenarios:

### 1. Standalone Mode (Default)
The simplest mode for individual projects. Devloop runs as a single daemon process with an embedded gRPC server.

```
┌─────────────────────┐
│   Your Project      │
│  ┌───────────────┐  │
│  │  .devloop.yaml│  │
│  └───────┬───────┘  │
│          │          │
│  ┌───────▼───────┐  │
│  │  devloop      │  │
│  │  (daemon)     │  │
│  │ ┌───────────┐ │  │
│  │ │gRPC Server│ │  │
│  │ └───────────┘ │  │
│  └───────────────┘  │
└─────────────────────┘
```

**Use Case:** Single project development, simple setups

### 2. Agent Mode
Devloop connects to a central gateway, ideal for multi-component projects where you want centralized monitoring.

```
Project A                    Project B
┌──────────────┐            ┌──────────────┐
│.devloop.yaml │            │.devloop.yaml │
│              │            │              │
│  devloop     │            │  devloop     │
│  (agent)     │            │  (agent)     │
└──────┬───────┘            └──────┬───────┘
       │                           │
       └─────────┐   ┌─────────────┘
                 │   │
              ┌──▼───▼──┐
              │ Gateway │
              │         │
              │  :8080  │
              └─────────┘
```

**Use Case:** Microservices, monorepos, distributed development

### 3. Gateway Mode
Acts as a central hub accepting connections from multiple agents, providing a unified interface for monitoring and control.

```
devloop --mode gateway --gateway-port 8080
```

**Features:**
- Centralized logging and monitoring
- Unified HTTP/gRPC API for all connected projects
- Real-time status updates from all agents
- Cross-project orchestration capabilities

## 📋 Prerequisites

Before installing devloop, ensure you have:

- **Go 1.20 or higher** installed ([Download Go](https://go.dev/dl/))
- **Git** (for version control operations)
- **$GOPATH/bin** added to your system's PATH

### Supported Platforms

- **Linux** (amd64, arm64)
- **macOS** (Intel, Apple Silicon)
- **Windows** (amd64) - Note: Process management may have limitations on Windows

## 🚀 Getting Started

1.  **Install `devloop`**:
    ```bash
    go install github.com/panyam/devloop@latest
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
          - "./bin/server"    # This starts the server and is long running
    ```
3.  **Run `devloop`**:
    ```bash
    devloop -c .devloop.yaml
    ```

`devloop` will now watch your files and automatically rebuild and restart your backend server whenever you make changes to your Go code.

## 📚 Examples

### Example 1: Full-Stack Web Application

```yaml
# .devloop.yaml
settings:
  prefix_logs: true
  prefix_max_length: 12

rules:
  - name: "Go API Server"
    prefix: "api"
    watch:
      - action: "include"
        patterns:
          - "cmd/api/**/*.go"
          - "internal/**/*.go"
          - "go.mod"
          - "go.sum"
    commands:
      - "go build -o bin/api ./cmd/api"
      - "bin/api"

  - name: "React Frontend"
    prefix: "frontend"
    watch:
      - action: "include"
        patterns:
          - "web/src/**/*.{js,jsx,ts,tsx}"
          - "web/src/**/*.css"
          - "web/package.json"
    commands:
      - "cd web && npm run dev"

  - name: "Database Migrations"
    prefix: "db"
    watch:
      - action: "include"
        patterns:
          - "migrations/**/*.sql"
    commands:
      - "migrate -path ./migrations -database postgres://localhost/myapp up"

  - name: "API Documentation"
    prefix: "docs"
    watch:
      - action: "include"
        patterns:
          - "api/**/*.proto"
          - "docs/**/*.md"
    commands:
      - "protoc --doc_out=./docs --doc_opt=markdown,api.md api/*.proto"
```

### Example 2: Microservices with Agent/Gateway Mode

**Gateway Server** (central monitoring):
```bash
# Start the gateway on a dedicated server
devloop --mode gateway --gateway-port 8080
```

**Service A** (authentication service):
```yaml
# auth-service/.devloop.yaml
rules:
  - name: "Auth Service"
    prefix: "auth"
    watch:
      - action: "include"
        patterns:
          - "**/*.go"
          - "go.mod"
    commands:
      - "go build -o bin/auth ./cmd/auth"
      - "bin/auth --port 3001"
```

```bash
# Connect to gateway
devloop --mode agent --gateway-url gateway.local:8080 -c .devloop.yaml
```

**Service B** (user service):
```yaml
# user-service/.devloop.yaml
rules:
  - name: "User Service"
    prefix: "user"
    watch:
      - action: "include"
        patterns:
          - "**/*.go"
          - "go.mod"
    commands:
      - "go build -o bin/user ./cmd/user"
      - "bin/user --port 3002"
```

### Example 3: Python Data Science Project

```yaml
# .devloop.yaml
rules:
  - name: "Jupyter Lab"
    prefix: "jupyter"
    watch:
      - action: "include"
        patterns:
          - "notebooks/**/*.ipynb"
    commands:
      - "jupyter lab --no-browser --port=8888"

  - name: "Model Training"
    prefix: "train"
    watch:
      - action: "include"
        patterns:
          - "src/**/*.py"
          - "configs/**/*.yaml"
      - action: "exclude"
        patterns:
          - "**/__pycache__/**"
          - "**/*.pyc"
    commands:
      - "python src/train.py --config configs/model.yaml"

  - name: "Tests"
    prefix: "test"
    watch:
      - action: "include"
        patterns:
          - "src/**/*.py"
          - "tests/**/*.py"
    commands:
      - "pytest tests/ -v --color=yes"
```

### Example 4: Rust Project with WASM

```yaml
# .devloop.yaml
rules:
  - name: "Rust Backend"
    prefix: "rust"
    watch:
      - action: "include"
        patterns:
          - "src/**/*.rs"
          - "Cargo.toml"
          - "Cargo.lock"
    commands:
      - "cargo build --release"
      - "cargo run --release"

  - name: "WASM Build"
    prefix: "wasm"
    watch:
      - action: "include"
        patterns:
          - "wasm/**/*.rs"
          - "wasm/Cargo.toml"
    commands:
      - "cd wasm && wasm-pack build --target web --out-dir ../web/pkg"

  - name: "Web Server"
    prefix: "web"
    watch:
      - action: "include"
        patterns:
          - "web/**/*.{html,js,css}"
          - "web/pkg/**/*"
    commands:
      - "cd web && python -m http.server 8000"
```

### Example 5: Mobile App Development

```yaml
# .devloop.yaml
settings:
  prefix_logs: true

rules:
  - name: "React Native Metro"
    prefix: "metro"
    watch:
      - action: "include"
        patterns:
          - "src/**/*.{js,jsx,ts,tsx}"
          - "package.json"
    commands:
      - "npx react-native start --reset-cache"

  - name: "iOS Build"
    prefix: "ios"
    watch:
      - action: "include"
        patterns:
          - "ios/**/*.{m,h,swift}"
          - "ios/**/*.plist"
    commands:
      - "cd ios && pod install"
      - "npx react-native run-ios --simulator='iPhone 14'"

  - name: "Android Build"
    prefix: "android"
    watch:
      - action: "include"
        patterns:
          - "android/**/*.{java,kt,xml}"
          - "android/**/*.gradle"
    commands:
      - "cd android && ./gradlew clean"
      - "npx react-native run-android"
```

### Example 6: Docker Compose Integration

```yaml
# .devloop.yaml
rules:
  - name: "Docker Services"
    prefix: "docker"
    watch:
      - action: "include"
        patterns:
          - "docker-compose.yml"
          - "**/*.Dockerfile"
          - ".env"
    commands:
      - "docker-compose down"
      - "docker-compose build"
      - "docker-compose up"

  - name: "Go Service"
    prefix: "go"
    watch:
      - action: "include"
        patterns:
          - "services/api/**/*.go"
    commands:
      - "docker-compose restart api"
```

## 🔄 Comparison with Similar Tools

| Feature | devloop | air | nodemon | watchexec |
|---------|---------|-----|---------|-----------|
| **Language Focus** | Multi-language | Go | Node.js | Any |
| **Parallel Rules** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **Distributed Mode** | ✅ Agent/Gateway | ❌ No | ❌ No | ❌ No |
| **Process Groups** | ✅ Yes | ✅ Yes | ✅ Yes | ⚠️ Limited |
| **Log Prefixing** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **gRPC/HTTP API** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **Config Format** | YAML | TOML | JSON | CLI args |
| **Debouncing** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| **Exclude Patterns** | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| **Build Tool** | ❌ No | ✅ Yes | ❌ No | ❌ No |
| **Live Reload** | ✅ Via commands | ✅ Built-in | ❌ No | ❌ No |
| **Multi-Project** | ✅ Yes | ❌ No | ❌ No | ❌ No |

### When to Use devloop

- **Multi-component projects**: When you need to orchestrate multiple services/components
- **Microservices development**: Centralized monitoring with agent/gateway mode
- **Complex workflows**: When you need multiple parallel build/watch tasks
- **API monitoring**: Built-in gRPC/HTTP endpoints for integration
- **Cross-language projects**: Not tied to a specific language ecosystem

### When to Use Alternatives

- **air**: Go-only projects with built-in compilation and live-reload
- **nodemon**: Simple Node.js projects with minimal configuration
- **watchexec**: Single-command execution with complex file watching needs

## ✨ Key Features

-   **Parallel & Concurrent Task Running**: Define rules for different parts of your project (backend, frontend, etc.) and `devloop` will run them concurrently.
-   **Intelligent Change Detection**: Uses glob patterns to precisely match file changes, triggering only the necessary commands.
-   **Robust Process Management**: Automatically terminates old processes before starting new ones, preventing zombie processes and ensuring a clean state.
-   **Debounced Execution**: Rapid file changes trigger commands only once, preventing unnecessary builds and restarts.
-   **Command Log Prefixing**: Prepends a customizable prefix to each line of your command's output, making it easy to distinguish logs from different processes.
-   **.air.toml Converter**: Includes a built-in tool to convert your existing `.air.toml` configuration into a `devloop` rule.

## ⚙️ Configuration Reference

### Complete Configuration Structure

```yaml
# .devloop.yaml
settings:                    # Optional: Global settings
  prefix_logs: boolean       # Enable/disable log prefixing (default: true)
  prefix_max_length: number  # Max length for prefixes (default: unlimited)

rules:                       # Required: Array of rules
  - name: string            # Required: Unique rule identifier
    prefix: string          # Optional: Custom log prefix (defaults to name)
    workdir: string         # Optional: Working directory for commands
    env:                    # Optional: Environment variables
      KEY: "value"
    watch:                  # Required: File watch configuration
      - action: string      # Required: "include" or "exclude"
        patterns: [string]  # Required: Glob patterns
    commands: [string]      # Required: Shell commands to execute
```

### Settings Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `prefix_logs` | boolean | `true` | Prepend rule name/prefix to each output line |
| `prefix_max_length` | integer | unlimited | Truncate/pad prefixes to this length for alignment |

### Rule Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `name` | string | ✅ | Unique identifier for the rule |
| `prefix` | string | ❌ | Custom prefix for log output (overrides name) |
| `workdir` | string | ❌ | Working directory for command execution |
| `env` | map | ❌ | Additional environment variables |
| `watch` | array | ✅ | File patterns to monitor |
| `commands` | array | ✅ | Commands to execute when files change |

### Watch Configuration

Each watch entry consists of:

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `action` | string | `include`, `exclude` | Whether to trigger on or ignore matches |
| `patterns` | array | glob patterns | File patterns using doublestar syntax |

### Glob Pattern Syntax

- `*` - Matches any sequence of non-separator characters
- `**` - Matches any sequence of characters including path separators
- `?` - Matches any single non-separator character
- `[abc]` - Matches any character in the set
- `[a-z]` - Matches any character in the range
- `{a,b}` - Matches either pattern a or b

### Example with All Options

```yaml
settings:
  prefix_logs: true
  prefix_max_length: 12

rules:
  - name: "Backend API"
    prefix: "api"
    workdir: "./backend"
    env:
      NODE_ENV: "development"
      PORT: "3000"
      DATABASE_URL: "postgres://localhost/myapp"
    watch:
      - action: "exclude"
        patterns:
          - "**/vendor/**"
          - "**/*_test.go"
          - "**/.*"
      - action: "include"
        patterns:
          - "**/*.go"
          - "go.mod"
          - "go.sum"
    commands:
      - "echo 'Building API server...'"
      - "go mod tidy"
      - "go build -tags dev -o bin/api ./cmd/api"
      - "./bin/api --dev"
```

### Pattern Matching Examples

```yaml
# Match all Go files
"**/*.go"

# Match Go files only in src directory
"src/**/*.go"

# Match test files
"**/*_test.go"

# Match multiple extensions
"**/*.{js,jsx,ts,tsx}"

# Match specific directory
"cmd/server/**/*"

# Match top-level files only
"*.go"

# Match hidden files
"**/.*"
```

### Command Execution Behavior

1. **Sequential Execution**: Commands run in the order specified
2. **Shell Execution**: Each command runs via `bash -c`
3. **Process Groups**: Commands run in separate process groups for clean termination
4. **Environment Inheritance**: Commands inherit parent environment plus `env` variables
5. **Working Directory**: Commands execute in `workdir` (or current directory if not set)
6. **Process Termination**: Previous instances receive SIGTERM when rule re-triggers
7. **Error Handling**: Failed commands don't stop subsequent commands in the list

### Environment Variable Precedence

1. System environment variables
2. Variables from `env` configuration (overrides system vars)
3. devloop internal variables:
   - `DEVLOOP_RULE_NAME` - Current rule name
   - `DEVLOOP_TRIGGER_FILE` - File that triggered the rule

## 📦 Installation

### Via Go Install (Recommended)

```bash
go install github.com/panyam/devloop@latest
```

This command will compile the `devloop` executable and place it in your Go binary directory, making it globally accessible.

Alternatively, to build the executable locally:

```bash
go build -o devloop
```

## 🔌 API Reference

Devloop provides both gRPC and REST APIs for monitoring and control. The REST API is available via gRPC-Gateway.

### REST API Endpoints

Base URL: `http://localhost:8080` (default gateway port)

#### List All Projects
```http
GET /projects
```
Returns all registered devloop projects and their connection status.

**Response:**
```json
{
  "projects": [
    {
      "project_id": "auth-service",
      "project_root": "/path/to/auth-service",
      "connection_status": "CONNECTED"
    }
  ]
}
```

#### Get Project Configuration
```http
GET /projects/{projectId}/config
```
Returns the full configuration for a specific project.

**Response:**
```json
{
  "config_json": "{\"rules\":[{\"name\":\"backend\",\"commands\":[\"go run .\"]}]}"
}
```

#### Get Rule Status
```http
GET /projects/{projectId}/status/{ruleName}
```
Returns the current status of a specific rule.

**Response:**
```json
{
  "rule_name": "backend",
  "is_running": true,
  "started_at": "1704092400000",
  "last_build_time": "1704092400000",
  "last_build_status": "SUCCESS"
}
```

#### Trigger Rule Manually
```http
POST /projects/{projectId}/trigger/{ruleName}
```
Manually triggers a rule execution.

**Response:**
```json
{
  "success": true,
  "message": "Rule 'backend' triggered successfully"
}
```

#### List Watched Paths
```http
GET /projects/{projectId}/watched-paths
```
Returns all glob patterns being watched by the project.

**Response:**
```json
{
  "patterns": [
    "**/*.go",
    "go.mod",
    "go.sum"
  ]
}
```

#### Read File Content
```http
GET /projects/{projectId}/file-content?path={filePath}
```
Reads a file from the project directory.

**Response:**
```json
{
  "content": "package main\n\nfunc main() {\n    // ...\n}"
}
```

#### Stream Real-time Logs
```http
GET /projects/{projectId}/stream/logs/{ruleName}?filter={optional}
```
Server-sent events stream for real-time logs.

**Response (SSE):**
```
data: {"project_id":"backend","rule_name":"api","line":"Starting server...","timestamp":"1704092400000"}

data: {"project_id":"backend","rule_name":"api","line":"Server listening on :8080","timestamp":"1704092401000"}
```

#### Get Historical Logs
```http
GET /projects/{projectId}/historical-logs/{ruleName}?filter={optional}&startTime={ms}&endTime={ms}
```
Retrieve historical logs with optional time range.

**Parameters:**
- `filter`: Optional text filter
- `startTime`: Start timestamp in milliseconds
- `endTime`: End timestamp in milliseconds

**Response:**
```json
{
  "logs": [
    {
      "project_id": "backend",
      "rule_name": "api",
      "line": "Request processed",
      "timestamp": "1704092400000"
    }
  ]
}
```

### gRPC Interface

For direct gRPC access, use the following service definitions:

```protobuf
service GatewayClientService {
  rpc ListProjects(ListProjectsRequest) returns (ListProjectsResponse);
  rpc GetProjectConfig(GetProjectConfigRequest) returns (GetProjectConfigResponse);
  rpc GetRuleStatus(GetRuleStatusRequest) returns (GetRuleStatusResponse);
  rpc TriggerRule(TriggerRuleRequest) returns (TriggerRuleResponse);
  rpc ListWatchedPaths(ListWatchedPathsRequest) returns (ListWatchedPathsResponse);
  rpc ReadFileContent(ReadFileContentRequest) returns (ReadFileContentResponse);
  rpc StreamLogs(StreamLogsRequest) returns (stream LogLine);
  rpc GetHistoricalLogs(GetHistoricalLogsRequest) returns (GetHistoricalLogsResponse);
}
```

### Client Examples

#### JavaScript/TypeScript
```javascript
// Fetch all projects
const response = await fetch('http://localhost:8080/projects');
const data = await response.json();

// Stream logs using EventSource
const events = new EventSource('http://localhost:8080/projects/backend/stream/logs/api');
events.onmessage = (event) => {
  const log = JSON.parse(event.data);
  console.log(`[${log.rule_name}] ${log.line}`);
};
```

#### Python
```python
import requests
import sseclient

# Get rule status
response = requests.get('http://localhost:8080/projects/backend/status/api')
status = response.json()

# Stream logs
response = requests.get('http://localhost:8080/projects/backend/stream/logs/api', stream=True)
client = sseclient.SSEClient(response)
for event in client.events():
    log = json.loads(event.data)
    print(f"[{log['rule_name']}] {log['line']}")
```

#### Go
```go
// Using the generated gRPC client
conn, _ := grpc.Dial("localhost:8080", grpc.WithInsecure())
client := pb.NewGatewayClientServiceClient(conn)

// List projects
resp, _ := client.ListProjects(context.Background(), &pb.ListProjectsRequest{})
for _, project := range resp.Projects {
    fmt.Printf("Project: %s (%s)\n", project.ProjectId, project.ConnectionStatus)
}
```

## 🚀 Usage

### Running in Standalone Mode (Default)

Navigate to your project's root directory and execute:

```bash
devloop -c .devloop.yaml
```

-   Use the `-c` flag to specify the path to your `.devloop.yaml` configuration file. If omitted, `devloop` will look for `.devloop.yaml` in the current directory.

### Running in Agent Mode

To connect to a gateway:

```bash
devloop --mode agent --gateway-url localhost:8080 -c .devloop.yaml
```

### Running in Gateway Mode

To start a central gateway:

```bash
devloop --mode gateway --gateway-port 8080
```

The gateway will accept connections from agents and provide a unified interface at `http://localhost:8080`.

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

## 🔧 Troubleshooting

### Common Issues and Solutions

#### 1. Configuration File Not Found
**Error:** `Failed to read config file: open .devloop.yaml: no such file or directory`

**Solutions:**
- Ensure `.devloop.yaml` exists in your current directory
- Use `-c` flag to specify the config path: `devloop -c path/to/.devloop.yaml`
- Check file permissions: `ls -la .devloop.yaml`

#### 2. Commands Not Executing
**Symptoms:** File changes detected but commands don't run

**Solutions:**
- Verify glob patterns match your files:
  ```bash
  # Test pattern matching
  find . -name "*.go" | grep -E "pattern"
  ```
- Check command syntax - commands run via `bash -c`
- Ensure commands are in your PATH
- Add debug output to commands:
  ```yaml
  commands:
    - "echo 'Rule triggered for: $DEVLOOP_TRIGGER_FILE'"
    - "your-actual-command"
  ```

#### 3. Process Won't Terminate
**Symptoms:** Old processes keep running after file changes

**Solutions:**
- Ensure your process handles SIGTERM properly
- For servers, implement graceful shutdown:
  ```go
  // Go example
  sigChan := make(chan os.Signal, 1)
  signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
  <-sigChan
  server.Shutdown(context.Background())
  ```
- Use process managers that handle signals correctly

#### 4. High CPU Usage
**Symptoms:** devloop consuming excessive CPU

**Solutions:**
- Reduce watch scope - avoid watching `node_modules`, `.git`, etc.:
  ```yaml
  watch:
    - action: "exclude"
      patterns:
        - "node_modules/**"
        - ".git/**"
        - "*.log"
    - action: "include"
      patterns:
        - "src/**/*.js"
  ```
- Check for recursive file generation (logs writing to watched directories)
- Ensure proper debouncing is working

#### 5. Port Already in Use
**Error:** `listen tcp :8080: bind: address already in use`

**Solutions:**
- Kill existing processes: `lsof -ti:8080 | xargs kill -9`
- Use different ports for different rules
- Implement port checking in your startup scripts:
  ```bash
  commands:
    - "kill $(lsof -ti:8080) || true"
    - "npm start"
  ```

#### 6. Permission Denied Errors
**Error:** `permission denied`

**Solutions:**
- Check file permissions: `chmod +x your-script.sh`
- Run devloop with appropriate user permissions
- For privileged ports (<1024), use port forwarding or reverse proxy

#### 7. Log Output Issues
**Symptoms:** Missing or garbled log output

**Solutions:**
- Enable log prefixing for clarity:
  ```yaml
  settings:
    prefix_logs: true
    prefix_max_length: 10
  ```
- Check if commands buffer output (use unbuffered mode):
  ```yaml
  commands:
    - "python -u script.py"  # Unbuffered Python
    - "node --no-buffering app.js"  # Unbuffered Node.js
  ```

#### 8. Agent Can't Connect to Gateway
**Error:** `Failed to connect to gateway`

**Solutions:**
- Verify gateway is running: `curl http://gateway-host:8080/projects`
- Check network connectivity: `ping gateway-host`
- Ensure correct gateway URL format: `--gateway-url host:port`
- Check firewall rules allow connection

#### 9. File Changes Not Detected
**Symptoms:** Modifying files doesn't trigger rules

**Solutions:**
- Verify file system supports inotify (Linux) or FSEvents (macOS)
- Check if you're editing files via network mount (may not trigger events)
- Ensure patterns are correct - use `**` for recursive matching:
  ```yaml
  # Wrong
  patterns: ["*.go"]  # Only matches root directory
  
  # Correct
  patterns: ["**/*.go"]  # Matches all subdirectories
  ```

#### 10. Memory Leaks
**Symptoms:** Memory usage grows over time

**Solutions:**
- Ensure commands properly clean up resources
- Check for accumulating log files
- Monitor with: `ps aux | grep devloop`
- Restart devloop periodically if needed

### Debug Mode

To get more detailed output for troubleshooting:

```bash
# Run with verbose logging (when implemented)
devloop -v -c .devloop.yaml

# Check devloop version
devloop --version

# Validate configuration
devloop validate -c .devloop.yaml
```

### Getting Help

If you continue experiencing issues:

1. Check existing issues: https://github.com/panyam/devloop/issues
2. Create a minimal reproducible example
3. Include your `.devloop.yaml` configuration
4. Provide system information:
   ```bash
   go version
   uname -a
   devloop --version
   ```

### Log Interpretation

Understanding log prefixes:
- `[devloop]` - Internal devloop operations
- `[rule-name]` - Output from your rule's commands
- `ERROR` - Critical errors requiring attention
- `WARN` - Non-critical issues
- `INFO` - General information
- `DEBUG` - Detailed debugging information (verbose mode)

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

