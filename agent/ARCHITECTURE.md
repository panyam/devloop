# Agent Architecture Documentation

This document describes the internal architecture of the devloop agent package.

## Overview

The agent package implements a simplified, channel-based architecture where RuleRunner instances handle direct command execution with proper debouncing and status tracking.

## Core Components

### 1. Orchestrator (`orchestrator.go`)
**Role**: Central coordinator and configuration manager

**Responsibilities**:
- Configuration loading and validation
- Component initialization and lifecycle management
- Cycle detection and prevention
- Project-level settings and defaults

**Key Features**:
- Manages RuleRunners with direct execution
- Advanced cycle detection with TriggerChain and FileModificationTracker
- Startup resilience with exponential backoff retry logic

### 2. RuleRunner (`rule_runner.go`) 
**Role**: File watching, debouncing, and direct command execution

**Responsibilities**:
- Per-rule file system monitoring via independent Watcher instances
- Channel-based event loop for clean concurrency
- Direct command execution with process management
- Status management and error tracking
- Rate limiting and cycle protection

**Key Features**:
- Independent fsnotify.Watcher per rule (no cross-rule interference)
- Channel-based event handling: fileChangeChan, timerChan, killChan, stopChan
- Status tracking: IsRunning, LastStarted, LastFinished, LastBuildStatus, LastError
- Direct process execution with proper status updates

### 3. Watcher (`watcher.go`)
**Role**: File system monitoring per rule

**Responsibilities**:
- Directory watching with inotify/FSEvents
- Pattern matching for include/exclude rules
- Event routing to RuleRunner via channels

**Key Features**:
- Per-rule file system isolation
- Interface-based design for extensibility
- Zero state management (pure routing)

### 4. WorkerPool (`workers.go`)
**Role**: Unified execution engine for all jobs (short and long-running)

**Responsibilities**:
- Job queuing with deduplication and process killing
- Global parallelism control via worker pool
- Process lifecycle management (start, monitor, terminate)
- Status callbacks to RuleRunner

**Key Features**:
- Configurable worker count via `max_parallel_rules`
- Job uniqueness (one instance per rule) with debounce-aware killing
- Process termination with SIGTERM → SIGKILL progression
- Cross-platform command execution
- Manual triggers bypass debounce for immediate restart

### 5. Watcher (`watcher.go`)
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
File Change → Watcher → fileChangeChan → RuleRunner.eventLoop → [Debounce] → timerChan → executeNow
                                          ↓                                        ↓
Manual Trigger → triggerExecution -------→                          Direct Command Execution
                                          ↓                                        ↓
Stop Signal → stopChan → handleStopEvent                               Status Updates
```

### Execution Flow

1. **Event Reception**: Channels receive file changes, timer expiration, kill signals, stop signals
2. **Debounce Handling**: File changes reset debounce timer, timer expiration triggers execution
3. **Direct Execution**: executeNow runs commands sequentially with status tracking
4. **Status Updates**: RUNNING → SUCCESS/FAILED with error messages
5. **Process Management**: Process termination with SIGTERM → SIGKILL progression

## Key Design Principles

### 1. Single Responsibility
Each component has one clear purpose:
- **RuleRunner**: "When and how to execute?" (file watching, debouncing, command execution)
- **Watcher**: "What files changed?" (file system monitoring per rule)
- **Orchestrator**: "Configuration and lifecycle?" (startup, shutdown, global settings)

### 2. Channel-Based Communication
Components communicate via Go channels for clean concurrency:
- Watcher → RuleRunner: File change events via fileChangeChan
- Timer → RuleRunner: Debounce expiration via timerChan
- Manual triggers → RuleRunner: Direct execution requests

### 3. Process Safety
- **Process Replacement**: Proper kill → wait → verify → start cycle for all jobs
- **Graceful Termination**: SIGTERM (5s) → SIGKILL with process group handling
- **Debounce-Aware Killing**: Respects debounce window unless manual trigger

### 4. Status Consistency
- **Single Source of Truth**: RuleRunner maintains authoritative rule status
- **Callback Pattern**: WorkerPool updates status via clean interface
- **Thread Safety**: All status operations protected by mutexes

## Configuration

### Simplified Configuration

```yaml
settings:
  max_parallel_rules: 5  # Global worker pool size for all jobs

rules:
  # All rules use the same unified execution model
  - name: "backend-build"
    commands:
      - "go build -o bin/server"
      - "go test ./..."
      
  - name: "dev-server"  # Long-running - will be killed/restarted on changes
    commands:
      - "./bin/server --dev --port 8080"
      
  - name: "frontend-dev"
    commands:
      - "npm run dev"  # Webpack dev server
```

### Process Management (All Rules)

**Unified Behavior**:
- Acquire worker from global pool
- Check if rule already running → kill if outside debounce window
- Execute commands sequentially  
- Monitor process and handle termination
- Release worker back to pool
- Mark as SUCCESS/FAILED (short jobs) or keep RUNNING (long jobs)

## Testing

### Test Structure

- `*_test.go`: Unit tests for individual components
- `*_integration_test.go`: Integration tests across components
- `testdata/configs/*.yaml`: Test configuration files

### Coverage Targets

```bash
make coverage-agent      # Quick coverage summary
make coverage-agent-html # Generate HTML report
make test-worker-pool   # Test WorkerPool process management
make test-scheduler     # Test Scheduler integration
```

### Key Test Scenarios

1. **Process Lifecycle**: Job start, killing, replacement, termination
2. **WorkerPool Execution**: Job queuing, parallel execution, status updates
3. **Scheduler Routing**: Unified routing to WorkerPool
4. **Status Callbacks**: WorkerPool updating RuleRunner status
5. **Debounce Logic**: Process killing respects debounce windows

## Performance Characteristics

- **Memory**: ~15-20MB base + ~2-5KB per rule watcher
- **CPU**: <1% idle, scales with file system events
- **Worker Efficiency**: Global pool shared across all job types
- **Process Management**: Intelligent killing with debounce protection
- **File Watch Latency**: <50ms from change to execution trigger

## Future Enhancements

- **Port Detection**: Automatic port extraction and cleanup verification
- **Health Monitoring**: Process health checks and automatic restart
- **Gateway Integration**: grpcrouter-based distributed mode
- **Advanced Routing**: Rule dependencies and conditional execution
