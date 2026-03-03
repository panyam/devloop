# Streaming Logs: Design & Implementation Plan

This document describes three improvements to the log streaming system, to be implemented sequentially. It also documents a bug discovered during the analysis.

## Bug: `finishedRules` Never Cleared on Re-Run

**Location**: `utils/log_manager.go`

When a rule finishes, `SignalFinished()` sets `finishedRules[ruleName] = true` (line 51). When the rule re-runs (file change or manual trigger), `GetWriter()` truncates the log file (line 40) but **never clears `finishedRules[ruleName]`**.

After the first completed run, every subsequent `StreamLogs` call takes the `streamFinishedLogs` path (line 68-70) — reads the truncated (empty/partial) file, hits EOF, sends a "completed" message, and exits. It never follows the new run's live output.

**Fix**: Clear `finishedRules[ruleName]` at the start of `GetWriter()` (or in a new `SignalRunStarted()` method introduced in Task 3 below).

---

## Task 1: Single-Poller-Per-Rule with Fan-Out to Clients

### Problem

The current `StreamLogs` implementation opens a new file reader per client connection (`streamLiveLogs`). Each client independently polls the log file with `time.Sleep(100ms)`. With N clients watching the same rule, that's N independent read loops all doing the same work.

### Design

Separate "reading new lines from the file" (one poller per rule) from "delivering to clients" (fan-out with per-client filtering/preferences).

**Components:**

```
LogBroadcaster (one per rule, lazily created)
├── filePoller goroutine
│     └── reads new lines from the log file
│     └── pushes raw lines to all subscribers
├── subscribers map[subscriberID]*Subscriber
│     └── each has its own channel + filter settings
└── ringBuffer (last N lines, configurable, default 200)
```

**LogBroadcaster lifecycle:**
- Created lazily on first `Subscribe()` call for a rule
- Starts its file poller goroutine on creation
- Stops poller and cleans up when last subscriber unsubscribes
- Stored in a `map[string]*LogBroadcaster` on LogManager, keyed by rule name

**Subscriber struct:**
```go
type Subscriber struct {
    ID     string
    Filter string                                     // per-client filter
    Ch     chan *pb.StreamLogsResponse                 // delivery channel
    Done   chan struct{}                               // closed when subscriber disconnects
}
```

**Key design point — extensibility for custom pollers later:**
The `LogBroadcaster` accepts a `LogSource` interface:

```go
type LogSource interface {
    // ReadLines blocks until new lines are available, then returns them.
    // Returns error on permanent failure.
    ReadLines(ctx context.Context) ([]string, error)
    // Reset is called when the log file is truncated (new run starts).
    Reset() error
    Close() error
}
```

Default implementation: `FileLogSource` (reads from the log file with polling). This allows swapping in a different source later (e.g., a client that wants to read from a different offset, or a pipe-based source) without changing the broadcaster.

**Fan-out behavior:**
1. Poller reads raw lines from the file
2. Lines are added to the ring buffer
3. For each subscriber: apply filter, send matching lines to subscriber's channel
4. If a subscriber's channel is full, drop oldest undelivered batch (bounded channel, never block the poller)

**Client join flow:**
1. Client calls `StreamLogs` with optional `last_n_lines` parameter
2. LogManager gets or creates the `LogBroadcaster` for the rule
3. If `last_n_lines > 0`: read that many lines from ring buffer, send immediately
4. Subscribe to live broadcast going forward
5. On client disconnect: unsubscribe, broadcaster cleans up if no subscribers remain

### Files to Change

| File | Change |
|------|--------|
| `utils/log_manager.go` | Add `LogBroadcaster`, `Subscriber`, `LogSource` interface, `FileLogSource` |
| `utils/log_manager.go` | Refactor `StreamLogs` to use broadcaster subscribe/unsubscribe |
| `utils/log_manager.go` | Add ring buffer (simple circular `[]string`) |
| `utils/log_manager_test.go` | Add tests for broadcaster fan-out, ring buffer, lazy start/stop |

### Proto Changes

```protobuf
message StreamLogsRequest {
  string rule_name = 1;
  string filter = 2;
  int64 timeout = 3;

  // Number of historical lines to send before switching to live streaming.
  // 0 = no history, just live. -1 = send all available history.
  int32 last_n_lines = 4;
}
```

---

## Task 2: Configurable Truncate vs Append on Re-Run

### Problem

`GetWriter()` always opens the log file with `O_TRUNC`. Every re-run of a rule replaces the previous log output entirely. For debugging flaky tests or tracking output across restarts, users want to preserve history.

### Design

Add a per-rule config option `append_on_restarts` (default: `false`, meaning truncate — current behavior). When `true`, consecutive runs append to the same log file instead of replacing.

**Config file (`.devloop.yaml`):**
```yaml
rules:
  - name: backend
    append_on_restarts: false    # default: each run starts fresh
    commands:
      - go build ./...

  - name: tests
    append_on_restarts: true     # keep all output across re-runs
    commands:
      - go test ./...
```

**Behavior matrix:**

| `append_on_restarts` | On re-run | Log file | Poller behavior |
|---|---|---|---|
| `false` (default) | Truncate file | Fresh content | Reset read offset to 0, clear ring buffer |
| `true` | Append separator + continue | Accumulated content | Continue reading from current offset |

**Separator line (append mode):**
```
--- [rule: backend] run started at 2026-03-03T10:15:00 (trigger: file_change) ---
```

This is a plain text separator that both humans and parsers can recognize. The structured `LogEvent` (Task 3) provides the machine-readable signal.

### Files to Change

| File | Change |
|------|--------|
| `protos/devloop/v1/models.proto` | Add `bool append_on_restarts = 18` to `Rule` message |
| `agent/common.go` | Parse `append_on_restarts` from YAML config |
| `utils/log_manager.go` | `GetWriter()` accepts append mode, chooses `O_TRUNC` vs `O_APPEND` |
| `utils/log_manager.go` | In append mode, write separator line on re-run |
| `utils/log_manager.go` | Broadcaster: on truncate, reset offset + ring buffer; on append, continue |
| `utils/log_manager_test.go` | Tests for both modes |

### Proto Change

```protobuf
message Rule {
  // ... existing fields ...

  // Whether to append logs across rule re-runs or truncate on each run.
  // false (default): each run starts with a clean log file.
  // true: consecutive runs append, preserving history.
  bool append_on_restarts = 18;
}
```

---

## Task 3: Structured Log Events for Run Lifecycle

### Problem

When a rule re-runs, connected streaming clients have no way to know "the log was just truncated" or "a new run started." They need a signal to decide whether to clear their view, insert a separator, or take other action.

Currently, control information is sent as plain text in `LogLine.line` (e.g., `"Rule 'backend' execution completed"`). This is fragile — clients can't reliably parse it, and it mixes control signals with actual log output.

### Design

Add a typed `LogEvent` to `StreamLogsResponse` alongside the existing `LogLine` messages. Events are control-plane signals; log lines are data-plane output.

**Proto changes:**

```protobuf
enum LogEventType {
  LOG_EVENT_TYPE_UNSPECIFIED = 0;
  LOG_EVENT_TYPE_RUN_STARTED = 1;     // new run began
  LOG_EVENT_TYPE_RUN_COMPLETED = 2;   // run finished successfully
  LOG_EVENT_TYPE_RUN_FAILED = 3;      // run finished with error
  LOG_EVENT_TYPE_TIMEOUT = 4;         // stream timed out waiting for content
}

message LogEvent {
  string rule_name = 1;
  LogEventType type = 2;
  int64 timestamp = 3;
  string message = 4;                  // human-readable context

  // Whether the log file was truncated (client should clear its view)
  // Only meaningful for RUN_STARTED events.
  bool truncated = 5;
}

message StreamLogsResponse {
  repeated LogLine lines = 1;

  // Control events (run started, completed, failed, timeout).
  // Sent alongside or instead of log lines.
  LogEvent event = 2;
}
```

**Lifecycle flow in LogManager:**

```
executeNow() called
    │
    ├── LogManager.SignalRunStarted(ruleName, appendMode)
    │     ├── clear finishedRules[ruleName]          ← FIXES THE BUG
    │     ├── if truncate: truncate file, broadcaster resets offset + ring buffer
    │     ├── if append: write separator line to file
    │     └── broadcast LogEvent{RUN_STARTED, truncated: !appendMode} to subscribers
    │
    ├── subprocess writes to log file
    │     └── poller reads new lines → broadcast to subscribers as LogLine
    │
    └── SignalFinished(ruleName, success bool, errMsg string)
          ├── set finishedRules[ruleName] = true
          └── broadcast LogEvent{RUN_COMPLETED or RUN_FAILED, message: errMsg}
```

**New LogManager methods:**

```go
// SignalRunStarted is called at the start of rule execution.
// It clears the finished state, handles truncate/append, and
// notifies subscribers with a RUN_STARTED event.
func (lm *LogManager) SignalRunStarted(ruleName string, appendMode bool) error

// SignalFinished is updated to accept success/error info and
// broadcast a RUN_COMPLETED or RUN_FAILED event.
func (lm *LogManager) SignalFinished(ruleName string, success bool, errMsg string)
```

**Client behavior (examples):**

| Client type | On RUN_STARTED (truncated=true) | On RUN_STARTED (truncated=false) |
|---|---|---|
| Terminal/CLI | Clear screen or print separator | Print separator |
| Web dashboard | Reset log panel | Insert visual divider |
| Vim plugin | Clear buffer | Append divider line |
| MCP/AI tool | Start fresh context | Continue with context |

### Files to Change

| File | Change |
|------|--------|
| `protos/devloop/v1/models.proto` | Add `LogEventType` enum, `LogEvent` message |
| `protos/devloop/v1/agents.proto` | Add `LogEvent event = 2` to `StreamLogsResponse` |
| `utils/log_manager.go` | Add `SignalRunStarted()`, update `SignalFinished()` signature |
| `utils/log_manager.go` | Broadcaster sends `LogEvent` messages to subscribers |
| `agent/rule_runner.go` | Call `SignalRunStarted()` before execution, pass success/error to `SignalFinished()` |
| `agent/service.go` | Forward events in the `StreamLogs` RPC handler |
| `utils/log_manager_test.go` | Tests for event broadcasting, truncation signaling |

---

## Implementation Order

1. **Task 1: Single-poller broadcaster** — Foundation for everything else. Fix the `finishedRules` bug as part of this since `SignalRunStarted` lives here.
2. **Task 2: Append vs truncate** — Builds on the broadcaster's ability to reset or continue.
3. **Task 3: Structured events** — Builds on both the broadcaster (delivery mechanism) and append/truncate (the `truncated` flag in `RUN_STARTED`).

Each task is independently shippable — Task 1 alone fixes the bug and improves scalability. Task 2 adds config flexibility. Task 3 adds the protocol-level signals.
