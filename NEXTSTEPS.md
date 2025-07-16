# Devloop Next Steps

This document outlines the immediate next steps for the `devloop` project.

## Recently Completed (2025-07-16)

- ✅ **Startup Resilience & Exponential Backoff Retry Logic:**
  - **Problem**: When devloop starts, if any rule fails the first time, devloop quits entirely - preventing development from continuing even during transient failures
  - **Solution**: Comprehensive startup retry system with exponential backoff that allows devloop to continue running while retrying failed rules
  - **Implementation**:
    - **New Rule Configuration Fields**:
      - `exit_on_failed_init: bool` (default: false) - Controls whether devloop exits when this rule fails startup
      - `max_init_retries: uint32` (default: 10) - Maximum retry attempts for failed startup  
      - `init_retry_backoff_base: uint64` (default: 3000ms) - Base backoff duration for exponential backoff
    - **Enhanced RuleRunner.Start()**: Added `executeWithRetry()` method with configurable exponential backoff
    - **Modified Orchestrator.Start()**: Collects startup failures instead of exiting immediately, only exits if critical rules fail
    - **Comprehensive Logging**: Detailed retry attempt logging with next retry time and success notifications
  - **Features Delivered**:
    - Exponential backoff retry logic (3s, 6s, 12s, 24s, etc.) with configurable base duration
    - Independent rule failure handling - rules fail independently without stopping devloop
    - Configurable exit behavior for critical rules via `exit_on_failed_init: true`
    - Graceful degradation - failed rules can still be triggered manually later
    - Backward compatibility - default behavior allows devloop to continue running despite startup failures
  - **Result**: Users can fix errors while devloop continues looping and watching for file changes
  - **Impact**: Major usability improvement - eliminates the frustration of devloop quitting on transient startup failures

## Previously Completed (2025-07-08)

- ✅ **Complete Cycle Detection Implementation:**
  - **Problem**: Devloop rules could create infinite cycles by watching files they modify, causing runaway resource consumption
  - **Solution**: Comprehensive cycle detection system with static validation and dynamic protection
  - **Implementation**:
    - **Phase 1 - Static Validation**: Added startup validation to detect self-referential patterns in rule configurations
    - **Phase 2 - Dynamic Rate Limiting**: Implemented TriggerTracker with frequency monitoring and exponential backoff
    - **Phase 3 - Advanced Dynamic Detection**: Added cross-rule cycle detection, file thrashing detection, and emergency breaks
    - **Config Parser Fix**: Fixed critical bug where YAML cycle_detection settings weren't being parsed into protobuf structs
  - **Features Delivered**:
    - Static self-reference detection with pattern overlap analysis relative to rule workdir
    - Rate limiting with configurable max_triggers_per_minute and exponential backoff
    - Cross-rule cycle detection using TriggerChain tracking with max_chain_depth limits
    - File thrashing detection with sliding window frequency analysis
    - Emergency cycle breaking with rule disabling and cycle resolution suggestions
    - Comprehensive configuration options in cycle_detection settings block
  - **Result**: Rules are prevented from creating infinite cycles while maintaining normal operation
  - **Impact**: Critical reliability improvement - prevents runaway processes and resource exhaustion

- ✅ **Watcher Robustness & Pattern Resolution Fix:**
  - **Problem**: Three critical watcher issues affecting reliability and intuitive behavior
    1. Patterns resolved relative to project root instead of rule's working directory
    2. Relative paths in patterns not properly honored
    3. Watcher flakiness when directories are added/removed
  - **Solution**: Complete overhaul of pattern resolution and directory watching logic
  - **Implementation**:
    - Modified `LoadConfig()` to preserve relative patterns instead of resolving to absolute paths
    - Created `resolvePattern()` helper for dynamic pattern resolution relative to rule's `workdir`
    - Enhanced `shouldWatchDirectory()` with pattern-based logic instead of hard-coded exclusions
    - Added dynamic directory watching for CREATE/DELETE events
    - Updated `RuleMatches()` to use runtime pattern resolution
  - **Result**: Patterns now work intuitively relative to each rule's working directory
  - **Impact**: Rules with different workdirs properly isolate their pattern matching, watcher handles dynamic filesystem changes

## Previously Completed (2025-07-07)

- ✅ **CLI Restructuring with Cobra Framework:**
  - **Problem**: Monolithic main.go with basic flag parsing and poor user experience
  - **Solution**: Complete refactoring to modern Cobra-based CLI with subcommands
  - **Implementation**: 
    - Created `cmd/` package with root, server, config, status, trigger, paths, convert commands
    - Extracted server logic to `server/` package with proper signal handling
    - Added `client/` package for gRPC client utilities
    - Default behavior: `devloop` starts server, subcommands act as client
  - **Result**: Professional CLI experience with help, subcommands, and client functionality
  - **Impact**: Users can now interact with running devloop servers via CLI commands

## Previously Completed (2025-07-07)

- ✅ **Architecture Simplification:**
  - **Problem**: Dual orchestrator implementations (V1/V2) created complexity
  - **Solution**: Simplified to single orchestrator implementation with Agent Service wrapper
  - **Implementation**: Removed factory pattern, consolidated to single agent/orchestrator.go
  - **Result**: Cleaner codebase, easier maintenance, and simplified testing
  - **Impact**: All tests passing with single implementation

- ✅ **Agent Service Integration:**
  - **Problem**: Complex gateway integration and API management
  - **Solution**: Created dedicated Agent Service (agent/service.go) providing gRPC API access
  - **Implementation**: Clean separation between file watching (Orchestrator) and API access (Agent Service)
  - **Result**: Better modularity and preparation for grpcrouter-based gateway
  - **API**: Provides GetConfig, GetRule, ListWatchedPaths, TriggerRule, StreamLogs endpoints

- ✅ **MCP Integration Simplification:**
  - **Root Cause**: Complex MCP mode with separate service management was hard to maintain
  - **Solution**: Simplified MCP to HTTP handler using existing Agent Service
  - **Implementation**: MCP runs on `/mcp` endpoint in startHttpServer method
  - **Result**: MCP enabled with `--enable-mcp` flag, uses same AgentServiceServer
  - **Benefits**: Easier maintenance, better integration with core functionality
  - **Compatibility**: Still uses StreamableHTTP transport (MCP 2025-03-26 spec) for universal compatibility

- ✅ **Default Port Configuration & Auto-Discovery:**
  - **Problem**: Default ports 8080 (HTTP) and 50051 (gRPC) cause frequent conflicts with other services
  - **Solution**: Updated defaults to 9999 (HTTP) and 5555 (gRPC) - less common ports
  - **Auto-Discovery**: Added `--auto-ports` flag for automatic port conflict resolution
  - **Implementation**: Port discovery logic in `runOrchestrator()` with fallback search
  - **User Experience**: `devloop` now starts without port conflicts in most scenarios

## Previously Completed (2025-07-03)

- ✅ **Complete Orchestrator Simplification:**
  - Deleted legacy dual orchestrator implementations and factory pattern
  - Simplified codebase to single orchestrator implementation
  - Updated all tests to use simplified architecture without version switching
  - Preserved modern process management and rule execution features

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

- ✅ **MCP (Model Context Protocol) Integration Simplification:**
  - ✅ **Simplified MCP as HTTP Handler:** Changed from separate mode to simple HTTP endpoint integration
  - ✅ Auto-generated MCP tools from protobuf definitions using protoc-gen-go-mcp
  - ✅ **StreamableHTTP Transport:** Uses modern MCP 2025-03-26 specification for universal compatibility
  - ✅ **Stateless Design:** No sessionId requirement for seamless Claude Code integration
  - ✅ **Agent Service Integration:** MCP uses same Agent Service as gRPC API for consistency
  - ✅ Enhanced protobuf documentation with comprehensive field descriptions and usage examples
  - ✅ Core MCP tools: GetConfig, GetRule, ListWatchedPaths, TriggerRule, StreamLogs
  - ✅ Updated integration approach using existing Agent Service architecture
  - ✅ Manual project ID configuration for consistent AI tool identification

- **Implement grpcrouter-based Gateway Mode:**
  - Research and integrate grpcrouter library for automatic gateway/proxy/reverse tunnel functionality
  - Implement simplified gateway mode using grpcrouter instead of custom implementation
  - Add client logic for agents to connect to grpcrouter-based gateway
  - Add comprehensive tests for distributed mode operations with new architecture

- **Add gRPC API Tests:** Create a new suite of tests specifically for the gRPC and HTTP endpoints to ensure the API is robust and reliable

## Medium Priority

- **MCP Enhancement Opportunities:**
  - Add streaming log support to MCP tools for real-time build/test monitoring
  - Consider implementing MCP resources for project files and configurations
  - Explore MCP prompts for common development workflows
  - Enhance MCP integration with additional Agent Service features

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
  - The simplified single-orchestrator architecture with Agent Service integration
  - The three core operating modes (standalone, agent, gateway) with MCP as HTTP handler
  - The glob pattern behavior with doublestar and proper action-based filtering
  - The new `agent/` directory structure and Agent Service architecture
  - Simplified MCP integration as HTTP handler using Agent Service
  
- **User Experience:** 
  - Refine the CLI flags and output for the new modes to ensure they are clear and intuitive
  - Add better progress indicators for long-running commands
  - Improve error messages and debugging information