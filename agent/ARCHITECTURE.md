# Agent Architecture Documentation

This document describes the internal architecture of the devloop agent package.

## Overview

The agent package implements an event-driven, unified execution architecture that handles all types of jobs (short and long-running) through a single WorkerPool with intelligent process management.

## Core Components

### 1. Orchestrator (`orchestrator.go`)
**Role**: Central coordinator and configuration manager

**Responsibilities**:
- Configuration loading and validation
- Component initialization and lifecycle management
- Cycle detection and prevention
- Project-level settings and defaults

**Key Features**:
- Manages RuleRunners, Scheduler, and WorkerPool
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
- Route all jobs to WorkerPool (unified execution)
- Stateless routing logic

**Key Features**:
- Clean separation between orchestration and execution
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
File Change → Watcher → RuleRunner → [Debounce] → TriggerEvent → Scheduler
                                                                    ↓
All Rules → WorkerPool → Worker → Process Management → Status Callback ↗
```

### Process Management Logic

1. **Job Enqueueing**: Check if rule already running
2. **Debounce Check**: Manual triggers or time > debounce window → kill existing
3. **Process Termination**: SIGTERM (5s) → SIGKILL with process group handling  
4. **Job Execution**: Start new process with proper monitoring
5. **Status Updates**: Running → Success/Failed via callbacks

## Key Design Principles

### 1. Single Responsibility
Each component has one clear purpose:
- **RuleRunner**: "When to execute?" (file watching, debouncing)
- **Scheduler**: "How to route?" (simple WorkerPool routing)
- **WorkerPool**: "Execute all jobs" (process management, killing, status)

### 2. Event-Driven Communication
Components communicate via events, not direct method calls:
- RuleRunner → Scheduler: TriggerEvent
- WorkerPool → RuleRunner: Status callbacks

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
