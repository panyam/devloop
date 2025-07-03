# Devloop Next Steps

This document outlines the immediate next steps for the `devloop` project.

## Recently Completed (2025-07-03)

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

- ✅ **Testing Infrastructure:**
  - Created factory pattern for testing both orchestrator implementations
  - Environment variable `DEVLOOP_ORCHESTRATOR_VERSION` selects v1 or v2
  - Added make targets: `testv1`, `testv2`, and `test` (runs both)
  - All tests pass for both implementations

- ✅ **Configuration Enhancements:**
  - Added rule-specific configuration options:
    - `debounce_delay: 500ms` - Per-rule debounce settings
    - `verbose: true` - Per-rule verbose logging
  - Added global defaults in settings:
    - `default_debounce_delay: 200ms`
    - `verbose: false`
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

- **Finalize Agent/Gateway Implementation:**
  - Implement the client logic in `agent` mode for registering with and sending data to the gateway
  - Implement the aggregation and proxying logic in the `gateway` mode to handle multiple agents
  - Add comprehensive tests for distributed mode operations

- **Add gRPC API Tests:** Create a new suite of tests specifically for the gRPC and HTTP endpoints to ensure the API is robust and reliable

## Medium Priority

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
  - The new gRPC-based architecture and the three operating modes
  - The glob pattern behavior with doublestar
  - The new `agent/` directory structure
  
- **User Experience:** 
  - Refine the CLI flags and output for the new modes to ensure they are clear and intuitive
  - Add better progress indicators for long-running commands
  - Improve error messages and debugging information