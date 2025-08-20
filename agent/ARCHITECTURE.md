# Agent Architecture Documentation

This document describes the internal architecture of the devloop agent package.

## Overview

The agent package implements an event-driven, dual-execution architecture that intelligently handles both long-running operations (LRO) and short-running jobs through clean separation of concerns.

## Core Components

### 1. Orchestrator (`orchestrator.go`)
**Role**: Central coordinator and configuration manager

**Responsibilities**:
- Configuration loading and validation
- Component initialization and lifecycle management
- Cycle detection and prevention
- Project-level settings and defaults

**Key Features**:
- Manages RuleRunners, Scheduler, WorkerPool, and LROManager
- Advanced cycle detection with TriggerChain and FileModificationTracker
- Startup resilience with exponential backoff retry logic

### 2. RuleRunner (`rule_runner.go`) 
**Role**: File watching, debouncing, and trigger coordination

**Responsibilities**:
- Per-rule file system monitoring via independent Watcher instances
- Debouncing logic to prevent command storms
- Rate limiting and cycle protection
- Status management (single source of truth for rule execution state)
- Event emission to Scheduler

**Key Features**:
- Independent fsnotify.Watcher per rule (no cross-rule interference)
- Rule-specific debounce delays and verbose logging
- Status tracking: IsRunning, StartTime, LastBuildTime, LastBuildStatus
- TriggerEvent emission for execution requests

### 3. Scheduler (`scheduler.go`)
**Role**: Event-driven job routing

**Responsibilities**:
- Receive TriggerEvents from RuleRunners
- Route jobs based on `rule.lro` flag
- Stateless routing logic

**Key Features**:
- Clean separation between orchestration and execution
- Interface-based design for extensibility
- Zero state management (pure routing)

### 4. WorkerPool (`workers.go`)
**Role**: Semaphore-controlled execution of short-running jobs

**Responsibilities**:
- Job queuing with deduplication
- Semaphore-based parallelism control
- Sequential command execution
- Status callbacks to RuleRunner

**Key Features**:
- Configurable worker count via `max_parallel_rules`
- Job deduplication (replace pending jobs for same rule)
- Proper process termination with SIGTERM → SIGKILL progression
- Cross-platform command execution

### 5. LROManager (`lro_manager.go`)
**Role**: Long-running operation process lifecycle management

**Responsibilities**:
- Process replacement (kill old → start new)
- Graceful process termination
- Background process monitoring
- Status callbacks to RuleRunner

**Key Features**:
- Unlimited concurrency (no semaphore limits)
- Process replacement on file changes with proper cleanup
- 5-second graceful termination window
- Background monitoring for unexpected process exits

### 6. Watcher (`watcher.go`)
**Role**: Per-rule file system monitoring

**Responsibilities**:
- File system event detection via fsnotify
- Glob pattern matching
- Directory watching based on patterns
- Event forwarding to RuleRunner

**Key Features**:
- Independent watcher per rule
- Pattern-based directory inclusion/exclusion
- Dynamic directory addition/removal
- Resource-efficient (~2-5KB memory per watcher)

## Data Flow

```
File Change → Watcher → RuleRunner → [Debounce] → TriggerEvent → Scheduler
                                                                    ↓
Short-Running (lro: false) → WorkerPool → Worker → Status Callback ↗
Long-Running (lro: true)   → LROManager → Process → Status Callback ↗
```

## Key Design Principles

### 1. Single Responsibility
Each component has one clear purpose:
- **RuleRunner**: "When to execute?"
- **Scheduler**: "How to route?"  
- **WorkerPool**: "Execute short jobs"
- **LROManager**: "Manage long processes"

### 2. Event-Driven Communication
Components communicate via events, not direct method calls:
- RuleRunner → Scheduler: TriggerEvent
- Execution Engines → RuleRunner: Status callbacks

### 3. Process Safety
- **LRO Process Replacement**: Proper kill → wait → verify → start cycle
- **Graceful Termination**: SIGTERM (5s) → SIGKILL with process group handling
- **Resource Cleanup**: Port/socket release verification (planned)

### 4. Status Consistency
- **Single Source of Truth**: RuleRunner maintains authoritative rule status
- **Callback Pattern**: Execution engines update status via clean interface
- **Thread Safety**: All status operations protected by mutexes

## Configuration

### LRO Flag Usage

```yaml
rules:
  # Short-running jobs (builds, tests, linting)
  - name: "backend-build"
    lro: false  # Default - uses WorkerPool with semaphore control
    commands:
      - "go build -o bin/server"
      - "go test ./..."
      
  # Long-running services (servers, databases, watchers)  
  - name: "dev-server"
    lro: true   # Uses LROManager with unlimited concurrency
    commands:
      - "./bin/server --dev --port 8080"
      
  - name: "frontend-dev"
    lro: true
    commands:
      - "npm run dev"  # Webpack dev server
```

### Process Management

**Short-Running Rules**:
- Acquire semaphore slot
- Execute commands sequentially
- Wait for completion
- Release semaphore slot
- Mark as COMPLETED

**Long-Running Rules**:
- Start process immediately (no semaphore)
- Mark as RUNNING
- Monitor process in background
- On file change: kill existing → start new
- Never marked as COMPLETED (only RUNNING/FAILED)

## Testing

### Test Structure

- `*_test.go`: Unit tests for individual components
- `*_integration_test.go`: Integration tests across components
- `testdata/configs/*.yaml`: Test configuration files

### Coverage Targets

```bash
make coverage-agent      # Quick coverage summary
make coverage-agent-html # Generate HTML report
make test-lro           # Test LRO Manager specifically
make test-scheduler     # Test Scheduler integration
make test-new-architecture # Test all new architecture components
```

### Key Test Scenarios

1. **LRO Lifecycle**: Process start, replacement, termination
2. **WorkerPool Execution**: Job queuing, parallel execution, status updates
3. **Scheduler Routing**: Correct routing based on lro flag
4. **Status Callbacks**: Execution engines updating RuleRunner status
5. **Mixed Workloads**: LRO and short-running jobs executing simultaneously

## Performance Characteristics

- **Memory**: ~15-20MB base + ~2-5KB per rule watcher
- **CPU**: <1% idle, scales with file system events
- **Semaphore Efficiency**: Only short-running jobs consume semaphore slots
- **LRO Concurrency**: Unlimited long-running processes
- **File Watch Latency**: <50ms from change to execution trigger

## Future Enhancements

- **Port Detection**: Automatic port extraction and cleanup verification
- **Health Monitoring**: LRO process health checks and automatic restart
- **Gateway Integration**: grpcrouter-based distributed mode
- **Advanced Routing**: Rule dependencies and conditional execution
