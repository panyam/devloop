# Devloop Release Notes

## [v0.1.3] - 2026-11-21

devloop v0.1.3 fixes `shell_files` on Linux and three ways shutdown could hang or leave processes behind.

### Fixed
- `shell_files` failed on Linux with exit 127. The source prefix used `source`, which `sh` lacks on Debian and Ubuntu. It now uses `.` (PR 14).
- Shutdown could hang when a file event arrived while devloop was stopping. The watcher blocked sending the event after the rule's event loop had exited (PR 14).
- Shutdown could deadlock when a matched file was removed, or a directory created, while the watcher was stopping (PR 16).
- A rule could start its next command after shutdown began, leaving an orphaned process (PR 14).

### Added
- CI runs `make test` on pushes and PRs, and govulncheck runs weekly and on each tag (PR 14).

### Dependencies
- The vue-components example moves to vitest ^4.1.11 (PR 15).

## [v0.1.2] - 2026-11-21

devloop v0.1.2 updates gRPC to fix open security advisories. Building devloop now requires Go 1.26.

### Security
- grpc v1.79.1 to v1.85.0. This fixes the `:path` authorization bypass, the HTTP/2 DATA frame memory exhaustion and the xDS advisories (PR 13).
- x/net v0.48.0 to v0.63.0, for the HTML parser DoS (PR 13).
- The examples pick up fixed versions of x/crypto, x/net, protobuf, gin-contrib/cors, requests, vite and vitest (PR 13).

### Changed
- The `go` directive moves from 1.24 to 1.26, because grpc v1.85.0 requires it.

### Fixed
- `TestGracefulShutdown` picks free ports instead of 8888 and 50051 (PR 13).

## [v0.1.1] - 2026-11-21

devloop v0.1.1 adds three views to `dagbump list`, and `make install` now builds dagbump alongside devloop.

### Added
- `dagbump list --pretty` draws a boxed table sized to the terminal. Each warning wraps inside the notes cell, so long warnings no longer break the table (PR 12).
- `dagbump list --html FILE` writes a self-contained page with the DAG in release waves. Hovering a unit traces its deps and dependents. Clicking it shows lagging deps, unreleased commits and warnings. Below the graph is a sortable, filterable table (PR 12).
- `dagbump list --svg FILE` writes the graph alone. `--open` opens the written file (PR 12).
- `make install` installs dagbump too.

## [v0.1.0] - 2026-11-09

devloop v0.1.0 removes the MCP server, moves the default ports, and adds dagbump, a command that cascades releases across dependent repos.

### Breaking
- The MCP server and the `--enable-mcp` flag are gone. Use the gRPC API or the HTTP gateway.
- The default ports moved from 5555 (gRPC) and 9999 (HTTP) to 19051 and 19080. Pass `--grpc-port 5555 --http-port 9999` to keep the old ones.

### Added
- `dagbump` (`go install github.com/panyam/devloop/cmd/dagbump@v0.1.0`). It finds the release DAG across local clones of Go modules and npm packages, writes a plan file, and applies dependency updates and releases wave by wave (PR 11).
- `disabled: true` on a rule turns it off without deleting it (issue 3).
- `append_on_restarts` keeps a rule's log file across re-runs.
- `shell_files` captures the environment from shell files once and passes it between a rule's commands.
- HTTP gateway routes for the gRPC API, and structured `LogEvent` messages in `StreamLogsResponse`.
- Log streaming fans out from one poller per rule.

### Fixed
- HTTP 404 on gateway routes, and a hang on Ctrl-C at shutdown.
- Six bugs in `RuleRunner` and its tests, and append-mode restarts with log streaming.
- `TerminateProcesses` only signals processes that are running.

### Dependencies
- gocurrent v0.0.10 to v0.1.2.

## v0.0.80 (2025-08-29)

### Simplified Architecture & Enhanced Status Tracking

**Major Architectural Simplification**
- **Replaced callback-based triggers with channel-based event loop** - Eliminated complex callback chains and multiple trigger methods
- **Removed scheduler and worker complexity** - Direct execution in RuleRunner for cleaner code paths
- **Channel-driven event handling** - File changes → channels → execution with proper Go concurrency patterns
- **Unified process management** - Single execution flow with proper status tracking

**Enhanced Status & Error Tracking**
- **Improved protobuf field names** for clarity:
  - `start_time` → `last_started` (when most recent execution started)
  - `last_build_time` → `last_finished` (when most recent execution finished) 
- **Added `last_error` field** to RuleStatus - Captures actual error messages from failed executions
- **Real-time status updates** - Proper "RUNNING" → "SUCCESS"/"FAILED" status transitions
- **Enhanced status display** - `devloop status` now shows detailed error information

**Event Flow Simplification**
```
Old: File Change → Callback → TriggerDebounced → Complex Timer Logic → Scheduler → Worker
New: File Change → fileChangeChan → handleFileChangeEvent → timerChan → executeNow
```

**Breaking Changes**
- Removed LRO (Long-Running Operations) distinction - all rules now execute directly
- Removed `lro: true/false` configuration flag - no longer needed
- Test interfaces changed from internal method access to channel-based behavior testing

## v0.0.70 (2025-08-20)

### LRO (Long-Running Operations) Architecture

**Architectural Changes**
- **Problem**: Long-running processes (dev servers, databases) were consuming worker semaphore slots indefinitely, blocking short-running jobs (builds, tests)
- **Solution**: Implemented dual-execution architecture with job routing based on `lro: true/false` flag
- **Result**: Dev servers can run indefinitely while builds/tests execute in parallel without resource starvation

**New Components**
- **Scheduler**: Event-driven routing component that receives TriggerEvents and routes to appropriate execution engines
- **LROManager**: Process lifecycle manager with process replacement and graceful termination
- **ProcessManager**: Process tree cleanup with SIGTERM to SIGKILL progression and process group verification
- **TriggerEvent**: Messaging system decoupling RuleRunner from execution concerns

**Configuration Changes**
```yaml
rules:
  - name: "build"
    lro: false  # Short-running - uses WorkerPool with semaphore
    commands:
      - "go build -o bin/server"
      
  - name: "dev-server"
    lro: true   # Long-running - uses LROManager, unlimited concurrency
    commands:
      - "./bin/server --dev"
```

**Implementation Details**
- LRO processes run with unlimited concurrency
- File changes trigger process kill and restart cycle with process tree cleanup
- RuleRunner handles watching/debouncing, Scheduler handles routing, Execution engines handle running
- Single source of truth for rule execution state via callback architecture
- Default `lro: false` maintains existing behavior

### Testing Infrastructure

**Test Coverage**
- 57% test coverage with LRO lifecycle and integration tests
- Process cleanup tests for process tree termination and race conditions
- Integration tests for scheduler routing and mixed workload scenarios

**Developer Tools**
- Makefile coverage targets: `make coverage-agent-html`, `make coverage-open`
- Test artifacts organized in `reports/` directory with HTML output
- Test categories: LRO tests, scheduler tests, stress tests

### Process Management

**Process Cleanup**
- Process group termination with proper SIGTERM to SIGKILL progression
- Mutex protection preventing multiple processes for same rule
- 5-second grace period before force termination
- Process verification ensuring complete process tree cleanup

## v0.0.60 (2025-08-01)

### 🚀 Enhanced Debouncing System

**Intelligent Build Consolidation**
- **Fixed**: Critical issue where multiple file edits during long builds would queue up multiple executions
- **Before**: 10 edits during 20s build = 10 queued builds (200s total)
- **After**: 10 edits during 20s build = 1 active + 1 pending (40s total)
- **Benefit**: ~80% reduction in wasted build time for rapidly changing files

**Smart Execution Management**
- Multiple triggers during rule execution now consolidate to single pending execution
- Only the most recent file changes trigger the final build
- No more wasteful intermediate builds from outdated file changes
- Thread-safe pending execution state management with proper mutex protection

**Enhanced Logging**
- Verbose mode shows when triggers are consolidated vs queued
- Clear visibility into debouncing decisions and pending execution replacements
- Better debugging for build timing and trigger behavior

### 🛠️ Bug Fixes & Improvements

**Configuration Parsing**
- Fixed `prefix_max_length` YAML parsing that was preventing proper prefix padding
- Prefix alignment now works correctly (e.g., `[prefix    ] output`)

**Test Infrastructure**
- Fixed `TestGracefulShutdown` by removing obsolete `--mode` CLI flag
- Fixed `TestPrefixing` with proper prefix length configuration parsing
- Added comprehensive test coverage for new debouncing behavior

### Configuration Example

```yaml
settings:
  default_debounce_delay: "500ms"  # Smart debouncing now prevents build queuing
  verbose: true                    # See debouncing decisions in logs
  
rules:
  - name: "frontend"
    commands:
      - "npm run build"  # Long builds no longer queue up!
```

**Result**: Developers experience dramatically faster feedback loops with no wasted build cycles.

---

## v0.0.59 (2025-08-01)

### 🎨 Enhanced Color Management

**Subprocess Color Preservation**
- **Fixed**: Devloop no longer suppresses colors from subprocess output
- **Before**: Tools like `npm build`, `go test`, etc. appeared in black and white
- **After**: Full color preservation - npm errors show in red, success messages in green
- **Benefit**: Significantly improved developer experience with native tool colors

**New Configuration Option**
- Added `suppress_subprocess_colors: false` (default) to settings
- Set to `true` to disable subprocess colors if preferred
- Backwards compatible - existing configs work unchanged

**Technical Improvements**
- Decoupled devloop prefix colors from subprocess color control
- Enhanced ANSI color code handling with proper regex patterns
- Added environment variables for subprocess color detection (`FORCE_COLOR`, `CLICOLOR_FORCE`, `COLORTERM`)
- Terminal output preserves colors, file logs remain clean without ANSI codes
- Comprehensive test coverage for color preservation functionality

### Configuration Example

```yaml
settings:
  color_logs: true                     # Enable devloop prefix colors
  suppress_subprocess_colors: false    # Preserve subprocess colors (default)
  
rules:
  - name: "frontend"
    commands:
      - "npm run build"    # Now shows colored output!
```

**Result**: Devloop prefixes stay colored (`[frontend]` in blue) while npm output shows its native colors (errors in red, success in green).

---

## Previous Releases

See [NEXTSTEPS.md](NEXTSTEPS.md) and [SUMMARY.md](SUMMARY.md) for complete feature history and technical details.