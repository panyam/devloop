# MCP Integration Guide

This guide explains how to integrate devloop with MCP (Model Context Protocol) clients for AI-powered development automation.

## Overview

The devloop MCP server is a **simplified HTTP handler** that runs alongside the gRPC API when enabled. It exposes development tools through the Model Context Protocol, allowing AI assistants to:
- Get project configuration and rule information
- Trigger rule execution manually
- List watched file paths
- Monitor rule status
- Stream logs (planned feature)

**Key Design Principles:** 
- MCP runs as an HTTP handler on the `/mcp` endpoint, not a separate mode
- Uses the same **Agent Service** that provides the gRPC API for consistency
- Uses modern **StreamableHTTP transport** (MCP 2025-03-26 spec) for universal client compatibility
- **Stateless design** eliminates sessionId requirements for seamless integration with Claude Code and other clients

## Quick Start

### 1. Enable MCP with devloop

```bash
# Enable MCP with standalone mode (requires both gRPC and HTTP ports)
devloop --grpc-port 5555 --http-port 9999 --enable-mcp -c /path/to/project/.devloop.yaml

# Enable MCP with automatic port discovery (avoids conflicts)
devloop --grpc-port 0 --http-port 0 --enable-mcp

# Enable MCP with custom ports
devloop --grpc-port 5000 --http-port 8080 --enable-mcp

# Or use default config location
cd /path/to/project
devloop --grpc-port 5555 --http-port 9999 --enable-mcp
```

**Note:** Gateway mode is temporarily removed and will be reimplemented using the grpcrouter library.

### 2. Configure MCP Client

#### Claude Code (HTTP Transport)
```bash
# Add devloop as HTTP MCP server for Claude Code
claude mcp add --transport http devloop http://localhost:9999/mcp/

# Test the connection
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":1}' \
  http://localhost:9999/mcp/
```

#### Other MCP Clients (stdio)
Add to your MCP client configuration:

```json
{
  "mcpServers": {
    "devloop": {
      "command": "devloop",
      "args": ["--grpc-port", "5555", "--http-port", "9999", "--enable-mcp", "-c", "/path/to/project/.devloop.yaml"],
      "env": {}
    }
  }
}
```

## Available MCP Tools

The following tools are auto-generated from devloop's Agent Service gRPC API definitions. Each tool includes comprehensive parameter validation and detailed documentation.

### Core Discovery Tools

#### `GetConfig`
- **Purpose**: Retrieve complete project configuration including available build rules, commands, and file watch patterns
- **Parameters**: None required (operates on the current project)
- **Returns**: Complete project configuration as JSON
- **Usage**: Understand available rules, commands, and file patterns

#### `ListWatchedPaths`
- **Purpose**: List all file glob patterns being monitored by the current devloop project
- **Parameters**: None required
- **Returns**: Array of glob patterns currently being watched
- **Usage**: Understand what file changes trigger builds

### Rule Management & Automation

#### `TriggerRule`
- **Purpose**: Manually execute a specific build/test rule to run commands immediately
- **Parameters**: 
  - `ruleName` (required): The name of the rule to execute (from project config)
- **Returns**: Success status and execution message
- **Usage**: "Run the backend tests", "Trigger the build rule"

#### `GetRule`
- **Purpose**: Get detailed information about a specific rule including its configuration
- **Parameters**: 
  - `ruleName` (required): The name of the rule to retrieve
- **Returns**: Rule configuration including watch patterns, commands, and settings
- **Usage**: Understand what a specific rule does and how it's configured

### Logging & Monitoring

#### `StreamLogs`
- **Purpose**: Stream logs for a specific rule (planned feature)
- **Parameters**: 
  - `ruleName` (required): The name of the rule to stream logs for
- **Returns**: Stream of log entries
- **Usage**: Monitor build/test progress in real-time
- **Status**: Currently a placeholder, implementation planned for future release

## Common Workflow Patterns

### 1. Project Discovery & Setup
```
1. GetConfig() → understand available rules, commands, and project structure
2. ListWatchedPaths() → see what file patterns trigger builds
3. GetRule(ruleName) → get detailed information about specific rules
```

### 2. Build & Test Automation
```
1. TriggerRule("backend-build") → start backend build
2. GetRule("backend-build") → check rule configuration
3. StreamLogs("backend-build") → monitor progress (when implemented)
```

### 3. Development Workflow
```
1. GetConfig() → see all available automation rules
2. TriggerRule("test-suite") → run tests manually
3. TriggerRule("lint-check") → run code quality checks
4. ListWatchedPaths() → understand what files are being monitored
```

### 4. Configuration Management
```
1. GetConfig() → review current project configuration
2. GetRule("failing-rule") → understand what the rule does
3. TriggerRule("failing-rule") → retry the rule manually
4. ListWatchedPaths() → check what files trigger the rule
```

## AI Assistant Integration

### Recommended Prompts

**For Build Automation:**
- "Run the backend tests and check the results"
- "Trigger the frontend build and monitor its progress"
- "Check if any builds are currently running"

**For Project Analysis:**
- "What build rules are available in this project?"
- "Show me the project configuration"
- "What files are being watched for changes?"

**For Debugging:**
- "Why did the last build fail?"
- "Read the build configuration and check for issues"
- "Show me the test results from the last run"

### Context Management

The MCP server maintains project context through:
- **Single Project Focus**: Operates on the current project only (no multi-project management)
- **Rule Names**: Specific build/test targets within the project
- **Configuration State**: Current project configuration and watch patterns
- **Execution Status**: Rule execution status and results

### Error Handling

The MCP server provides detailed error messages for:
- Invalid rule names
- Missing required parameters
- gRPC service unavailability
- Rule execution failures

## Transport Architecture

### StreamableHTTP Transport
Devloop uses the modern MCP StreamableHTTP transport (2025-03-26 spec) for:
- **Universal Compatibility**: Works with Claude Code, Python SDK, and other MCP clients
- **Stateless Operation**: No sessionId requirement simplifies client integration
- **HTTP/JSON**: Standard protocols for easy debugging and testing
- **Single Endpoint**: All MCP operations through `/mcp/` path
- **Agent Service Integration**: Uses the same service that provides gRPC API

### Implementation Details
The MCP server is implemented as a simple HTTP handler that:
- Runs alongside the gRPC server when `--enable-mcp` is specified
- Uses the Agent Service instance for all operations
- Requires both `--grpc-port` and `--http-port` to be specified
- Auto-generates tools from protobuf definitions
- Provides consistent API between gRPC and MCP interfaces
Previous versions used SSE transport requiring sessionId. This has been migrated to StreamableHTTP for better compatibility.

## Security Considerations

- **File Access**: Limited to project directory only
- **Path Traversal**: Blocked (no ../ allowed)
- **Command Execution**: Only through predefined devloop rules
- **Network Access**: HTTP on localhost only (default port 9999)

## Troubleshooting

### Common Issues

1. **"Missing sessionId" (Fixed)**: If using older devloop versions, upgrade to latest with StreamableHTTP transport
2. **"Connection refused"**: Ensure devloop is running with `--enable-mcp` flag and correct port (default 9999)
3. **"Project not found"**: Ensure devloop is running in agent mode for the project
4. **"Rule not found"**: Check rule names in project config with get_project_config
5. **"Permission denied"**: Ensure MCP server has read access to project files
6. **"Build failed"**: Check rule status and read relevant log files

### Debug Mode

Enable verbose logging:
```bash
devloop --enable-mcp --v --c /path/to/project/.devloop.yaml
```

### Integration Testing

Test MCP integration:
```bash
# Start devloop with MCP enabled
devloop --enable-mcp --c .devloop.yaml

# Test HTTP transport directly
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}},"id":1}' \
  http://localhost:9999/mcp/

# Test tools list
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}' \
  http://localhost:9999/mcp/

# Test with Claude Code
claude mcp add --transport http devloop http://localhost:9999/mcp/
```

## Advanced Usage

### Multiple Projects

To manage multiple projects, you have two options:

**Option 1: Separate MCP servers per project**
```json
{
  "mcpServers": {
    "devloop-backend": {
      "command": "devloop",
      "args": ["--enable-mcp", "--c", "/path/to/backend/.devloop.yaml"]
    },
    "devloop-frontend": {
      "command": "devloop", 
      "args": ["--enable-mcp", "--c", "/path/to/frontend/.devloop.yaml"]
    }
  }
}
```

**Option 2: Gateway mode with single MCP server (Recommended)**
```bash
# Start gateway with MCP
devloop --mode gateway --enable-mcp --grpc-port 50051

# Connect agents from different projects
cd /path/to/backend && devloop --mode agent --gateway-addr localhost:50051
cd /path/to/frontend && devloop --mode agent --gateway-addr localhost:50051
```

The gateway approach provides a unified view of all projects through a single MCP interface.

### Custom Rule Workflows

Create specialized rules for AI automation:

```yaml
# .devloop.yaml
rules:
  - name: "ai-test-suite"
    commands:
      - "npm test -- --reporter=json > test-results.json"
      - "echo 'Tests completed, results in test-results.json'"
    
  - name: "ai-build-report"
    commands:
      - "npm run build 2>&1 | tee build-output.log"
      - "echo 'Build completed, output in build-output.log'"
```

This allows AI assistants to trigger specific workflows designed for automation and result analysis.