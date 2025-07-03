# Devloop Next Steps

This document outlines the immediate next steps for the `devloop` project.

## Recently Completed (2025-07-03)

- ✅ **Complete OrchestratorV1 Removal:**
  - Deleted legacy V1 orchestrator and factory pattern implementations
  - Simplified codebase to single OrchestratorV2 implementation
  - Updated all tests to use V2 directly without version switching
  - Backed up legacy cleanup files while preserving modern process management

- ✅ **Critical Rule Matching Bug Fix:**
  - **Root Cause**: Orchestrator was ignoring `Action` field in matcher patterns
  - **Impact**: Exclude patterns matched but still triggered rule execution
  - **Solution**: Implemented proper action-based filtering logic
  - **Result**: SDL project's `web/**` exclusions now work correctly

- ✅ **Enhanced Configuration System:**
  - Added `default_action` field to Rule struct for per-rule defaults
  - Added `default_watch_action` field to Settings struct for global defaults
  - Implemented proper precedence: rule-specific → global → hardcoded fallback

- ✅ **Architecture Refactoring:**
  - Separated file watching (Orchestrator) from command execution (RuleRunner)
  - Implemented OrchestratorV2 with cleaner separation of concerns
  - Each rule now has its own RuleRunner managing its lifecycle
  - Fixed various race conditions in process management

- ✅ **Process Management Improvements:**
  - Fixed zombie process issues when devloop is killed
  - Implemented platform-specific process management (Pdeathsig on Linux, Setpgid on Darwin)
  - Commands now execute sequentially with proper failure propagation (like GNU Make)
  - Added process existence checks before termination to avoid errors

- ✅ **Cross-Platform Command Execution:**
  - Fixed hardcoded `bash -c` commands that failed on Windows
  - Implemented `createCrossPlatformCommand()` using `cmd /c` on Windows, `bash -c` on Unix
  - Added fallback to `sh -c` for POSIX compatibility when bash is unavailable
  - Fixed WorkDir behavior to default to config file directory instead of current working directory

- ✅ **Color-Coded Rule Output:**
  - Added comprehensive color support using `github.com/fatih/color` library
  - Implemented configurable color schemes (auto, dark, light, custom)
  - Hash-based consistent color assignment ensures same rule always gets same color
  - Separate handling for terminal (colored) vs file (plain) output
  - Configuration options: `color_logs`, `color_scheme`, `custom_colors`
  - Per-rule color overrides via `color` field in rule configuration
  - Automatic terminal detection with sensible defaults

- ✅ **Testing Infrastructure:**
  - Created factory pattern for testing both orchestrator implementations
  - Environment variable `DEVLOOP_ORCHESTRATOR_VERSION` selects v1 or v2
  - Added make targets: `testv1`, `testv2`, and `test` (runs both)
  - All tests pass for both implementations

- ✅ **Configuration Enhancements:**
  - Added rule-specific configuration options:
    - `debounce_delay: 500ms` - Per-rule debounce settings
    - `verbose: true` - Per-rule verbose logging
    - `color: "blue"` - Per-rule color overrides
  - Added global defaults in settings:
    - `default_debounce_delay: 200ms`
    - `verbose: false`
    - `color_logs: true` - Enable colored output
    - `color_scheme: "auto"` - Auto-detect terminal theme
  - Rule-specific settings override global defaults

## Previously Completed (2025-07-02)

- ✅ **Fixed All Failing Tests:** Successfully resolved all test failures
- ✅ **Improved Glob Pattern Matching:** Switched to `bmatcuk/doublestar` library
- ✅ **Code Reorganization:** Moved files into `agent/` directory

## High Priority

- ✅ **Complete OrchestratorV2 Gateway Integration:**
  - ✅ Port missing gateway communication methods from v1 to v2
  - ✅ Implement handleGatewayStreamRecv methods (GetConfig, GetRuleStatus, TriggerRule, etc.)
  - ✅ Test gateway integration with both orchestrator versions (all tests passing)

- ✅ **Production Readiness:**
  - ✅ Switch default orchestrator to v2 after thorough testing
  - Add migration guide for users
  - Performance benchmarking between v1 and v2

- ✅ **MCP (Model Context Protocol) Integration:**
  - ✅ **Redesigned MCP as Add-On Capability:** Changed from exclusive mode to optional feature alongside core modes
  - ✅ Auto-generated MCP tools from protobuf definitions using protoc-gen-go-mcp
  - ✅ Enhanced protobuf documentation with comprehensive field descriptions and usage examples
  - ✅ Implemented six core MCP tools: ListProjects, GetConfig, GetRuleStatus, TriggerRuleClient, ReadFileContent, ListWatchedPaths
  - ✅ Created complete integration guide (MCP_INTEGRATION.md) with workflow patterns and usage examples
  - ✅ Added manual project ID configuration for consistent AI tool identification
  - ✅ Clean separation of MCP functionality in `internal/mcp/` package using adapter pattern

- **Finalize Agent/Gateway Implementation:**
  - Implement the client logic in `agent` mode for registering with and sending data to the gateway
  - Implement the aggregation and proxying logic in the `gateway` mode to handle multiple agents
  - Add comprehensive tests for distributed mode operations

- **Add gRPC API Tests:** Create a new suite of tests specifically for the gRPC and HTTP endpoints to ensure the API is robust and reliable

## Medium Priority

- **MCP Enhancement Opportunities:**
  - Add streaming log support to MCP tools for real-time build/test monitoring
  - Consider implementing MCP resources for project files and configurations
  - Explore MCP prompts for common development workflows
  - Add support for multiple project instances in single MCP server

- **Performance Optimization:**
  - Profile the file watching and command execution pipeline
  - Optimize debouncing logic for large numbers of file changes
  - Consider implementing command queuing for better resource management

- **Enhanced Configuration:**
  - Add support for environment-specific configurations
  - Implement configuration validation and better error messages
  - Consider adding a configuration wizard for new users

## Ongoing

- **Documentation:** Update all project documentation (README, etc.) to reflect:
  - The simplified single-orchestrator architecture
  - The three core operating modes (standalone, agent, gateway) + MCP as add-on
  - The glob pattern behavior with doublestar and proper action-based filtering
  - The new `agent/` directory structure
  - MCP integration capabilities and AI assistant workflows
  
- **User Experience:** 
  - Refine the CLI flags and output for the new modes to ensure they are clear and intuitive
  - Add better progress indicators for long-running commands
  - Improve error messages and debugging information