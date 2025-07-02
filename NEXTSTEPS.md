# Devloop Next Steps

This document outlines the immediate next steps for the `devloop` project.

## High Priority

- **Fix Failing Tests:** The top priority is to resolve the remaining test failures in `graceful_shutdown_test.go` and `log_manager_test.go`. The recent architectural refactoring has introduced issues with path resolution and test timeouts that must be addressed.
- **Finalize Agent/Gateway Implementation:**
  - Implement the client logic in `agent` mode for registering with and sending data to the gateway.
  - Implement the aggregation and proxying logic in the `gateway` mode to handle multiple agents.
- **Add gRPC API Tests:** Create a new suite of tests specifically for the gRPC and HTTP endpoints to ensure the API is robust and reliable.

## Ongoing

- **Documentation:** Update all project documentation (README, etc.) to reflect the new gRPC-based architecture and the three operating modes.
- **User Experience:** Refine the CLI flags and output for the new modes to ensure they are clear and intuitive.