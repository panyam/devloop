# MCP Integration Guide

This guide explains how to integrate devloop with MCP (Model Context Protocol) clients for AI-powered development automation.

## Overview

The devloop MCP server is an **add-on capability** that can be enabled alongside any devloop operating mode (standalone, agent, or gateway). It exposes development tools through the Model Context Protocol, allowing AI assistants to:
- Discover and monitor development projects
- Trigger builds and tests  
- Read configuration and source files
- Monitor build status and logs
- Understand project structure and dependencies

**Key Design Principle:** MCP is not a separate mode but an additional interface that enhances existing devloop functionality.

## Quick Start

### 1. Enable MCP alongside any devloop mode

```bash
# Enable MCP with standalone mode (default)
devloop --enable-mcp --c /path/to/project/.devloop.yaml

# Enable MCP with gateway mode
devloop --mode gateway --enable-mcp --grpc-port 50051 --http-port 8080

# Enable MCP with agent mode
devloop --mode agent --enable-mcp --gateway-addr localhost:50051

# Or use default config location
cd /path/to/project
devloop --enable-mcp
```

### 2. Configure MCP Client

Add to your MCP client configuration:

```json
{
  "mcpServers": {
    "devloop": {
      "command": "devloop",
      "args": ["--enable-mcp", "--c", "/path/to/project/.devloop.yaml"],
      "env": {}
    }
  }
}
```

## Available MCP Tools

The following tools are auto-generated from devloop's gRPC API definitions. Each tool includes comprehensive parameter validation and detailed documentation.

### Core Discovery Tools

#### `devloop_gateway_v1_GatewayClientService_ListProjects`
- **Purpose**: Discover all available devloop projects for development automation and monitoring
- **Parameters**: None required
- **Returns**: Array of projects with IDs, paths, and connection status
- **Usage**: Start here to see what projects are available

#### `devloop_gateway_v1_GatewayClientService_GetConfig`
- **Purpose**: Retrieve complete project configuration including available build rules, commands, and file watch patterns
- **Parameters**: 
  - `project_id` (required): The unique identifier of the project (from ListProjects output)
- **Returns**: Complete project configuration as JSON string
- **Usage**: Understand available rules, commands, and file patterns

### Build & Test Automation

#### `devloop_gateway_v1_GatewayClientService_TriggerRuleClient`
- **Purpose**: Manually execute a specific build/test rule to run commands immediately
- **Parameters**: 
  - `project_id` (required): The unique identifier of the project
  - `rule_name` (required): The name of the rule to execute (from project config)
- **Returns**: Success status and execution message
- **Usage**: "Run the backend tests", "Trigger the build rule"

#### `devloop_gateway_v1_GatewayClientService_GetRuleStatus`
- **Purpose**: Check the current execution status of a specific build/test rule to monitor progress and results
- **Parameters**: 
  - `project_id` (required): The unique identifier of the project
  - `rule_name` (required): The name of the rule to check (from project config)
- **Returns**: Detailed status including execution state, timing, and results
- **Usage**: Check if builds are running, verify test results

### File & Configuration Access

#### `devloop_gateway_v1_GatewayClientService_ReadFileContent`
- **Purpose**: Read and return the content of a specific file within a devloop project
- **Parameters**: 
  - `project_id` (required): The unique identifier of the project
  - `path` (required): Relative path to the file within the project (no ../ allowed)
- **Returns**: Raw file content as bytes
- **Usage**: Read configs, source code, logs
- **Security**: Restricted to project directory, no path traversal allowed

#### `devloop_gateway_v1_GatewayClientService_ListWatchedPaths`
- **Purpose**: List all file glob patterns being monitored by a specific devloop project
- **Parameters**: 
  - `project_id` (required): The unique identifier of the project
- **Returns**: Array of glob patterns currently being watched
- **Usage**: Understand what file changes trigger builds

## Common Workflow Patterns

### 1. Project Discovery & Setup
```
1. devloop_gateway_v1_GatewayClientService_ListProjects() → see available projects
2. devloop_gateway_v1_GatewayClientService_GetConfig(project_id) → understand rules and commands
3. devloop_gateway_v1_GatewayClientService_ListWatchedPaths(project_id) → see what files are monitored
```

### 2. Build & Test Automation
```
1. devloop_gateway_v1_GatewayClientService_TriggerRuleClient(project_id, "backend") → start backend build
2. devloop_gateway_v1_GatewayClientService_GetRuleStatus(project_id, "backend") → monitor progress
3. devloop_gateway_v1_GatewayClientService_ReadFileContent(project_id, "build.log") → analyze results if needed
```

### 3. Development Debugging
```
1. devloop_gateway_v1_GatewayClientService_GetRuleStatus(project_id, rule_name) → check if build failed
2. devloop_gateway_v1_GatewayClientService_ReadFileContent(project_id, ".devloop.yaml") → review configuration
3. devloop_gateway_v1_GatewayClientService_ReadFileContent(project_id, "package.json") → check dependencies
4. devloop_gateway_v1_GatewayClientService_TriggerRuleClient(project_id, rule_name) → retry after fixes
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
- **Project IDs**: Unique identifiers for each project
- **Rule Names**: Specific build/test targets within projects
- **File Paths**: Relative paths within project boundaries
- **Status Tracking**: Real-time build/test execution status

### Error Handling

The MCP server provides detailed error messages for:
- Invalid project IDs or rule names
- Missing required parameters
- File access permissions
- Build/test execution failures

## Security Considerations

- **File Access**: Limited to project directory only
- **Path Traversal**: Blocked (no ../ allowed)
- **Command Execution**: Only through predefined devloop rules
- **Network Access**: Local stdio communication only

## Troubleshooting

### Common Issues

1. **"Project not found"**: Ensure devloop is running in agent mode for the project
2. **"Rule not found"**: Check rule names in project config with get_project_config
3. **"Permission denied"**: Ensure MCP server has read access to project files
4. **"Build failed"**: Check rule status and read relevant log files

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

# In another terminal, test with MCP client
# (specific commands depend on your MCP client)
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