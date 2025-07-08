# Cycle Detection Demo

This example demonstrates devloop's comprehensive cycle detection capabilities, showcasing different types of cycles and how the system detects and warns about them.

## What's Included

- **Self-Referential Patterns**: Rules that watch files they create themselves
- **Cross-Rule Cycles**: Multiple rules that can trigger each other
- **Workdir-Relative Cycles**: Rules with different working directories that may overlap
- **Cycle Protection Configuration**: Examples of global and per-rule cycle protection settings
- **Static Detection**: Demonstrates static analysis at startup
- **Dynamic Protection**: Shows rate limiting and trigger chain detection (future phases)

## Prerequisites

- Go 1.20 or higher
- devloop installed (`go install github.com/panyam/devloop@latest`)

## Quick Start

1. Run the example:
   ```bash
   make run
   # Or directly: devloop -c .devloop.yaml
   ```

2. Observe the cycle detection warnings in the output

3. Test different configurations:
   ```bash
   # Test with cycle detection disabled
   devloop -c .devloop-no-cycles.yaml
   
   # Test with per-rule overrides
   devloop -c .devloop-mixed-protection.yaml
   ```

## Cycle Types Demonstrated

### 1. Self-Referential Cycles

**Problem**: A rule watches `**/*.log` but creates `output.log` in its working directory.

```yaml
rules:
  - name: "self-ref-logger"
    watch:
      - action: include
        patterns:
          - "**/*.log"
    commands:
      - "echo 'Processing logs...' >> output.log"
```

**Detection**: Static analysis detects pattern overlap with working directory.

### 2. Cross-Rule Cycles

**Problem**: Rule A watches files that Rule B creates, and Rule B watches files that Rule A creates.

```yaml
rules:
  - name: "config-generator"
    watch:
      - action: include
        patterns:
          - "input/*.yaml"
    commands:
      - "echo 'Generated config' > generated/config.json"
      
  - name: "config-processor"
    watch:
      - action: include
        patterns:
          - "generated/*.json"
    commands:
      - "echo 'Processed config' > input/processed.yaml"
```

**Detection**: ✅ **IMPLEMENTED** - Cross-rule cycle detection with trigger chain tracking.

### 3. Workdir-Relative Cycles

**Problem**: Different working directories but overlapping patterns.

```yaml
rules:
  - name: "backend-builder"
    workdir: "./backend"
    watch:
      - action: include
        patterns:
          - "src/**/*.go"
    commands:
      - "go build -o bin/server ./src"
      - "echo 'Built at' $(date) > src/build.log"
```

**Detection**: Pattern `src/**/*.go` matches `src/build.log` created by command.

### 4. Configuration Examples

#### Global Cycle Detection (Default)
```yaml
settings:
  cycle_detection:
    enabled: true
    static_validation: true
    dynamic_protection: false
    max_triggers_per_minute: 10
    max_chain_depth: 5
    file_thrash_window_seconds: 60
    file_thrash_threshold: 5
```

#### Per-Rule Cycle Protection Override
```yaml
rules:
  - name: "intentional-cycle"
    cycle_protection: false  # Disable protection for this rule
    watch:
      - action: include
        patterns:
          - "**/*.log"
    commands:
      - "echo 'Intentional cycle' >> cycle.log"
```

## What to Expect

### Static Detection Output

When you run `devloop server`, you'll see:

```
[devloop] Warning: Rule "self-ref-logger" may trigger itself - pattern "**/*.log" watches workdir "/path/to/examples/06-cycle-detection-demo"
[devloop] Warning: Rule "backend-builder" may trigger itself - pattern "src/**/*.go" watches workdir "/path/to/examples/06-cycle-detection-demo/backend"
[devloop] Starting with ports: HTTP=9999, gRPC=5555
[devloop] Starting orchestrator...
```

### Runtime Behavior with Dynamic Protection

With `dynamic_protection: true`, you'll see cycle prevention in action:

1. **Rate Limiting**: Rules exceeding `max_triggers_per_minute` enter backoff periods
2. **Cross-Rule Cycle Detection**: Trigger chains are tracked and cycles broken automatically
3. **File Thrashing Detection**: Rapid file modifications are detected and rules skipped
4. **Emergency Breaks**: Persistent cycles trigger emergency rule disabling

Example dynamic protection output:
```
[devloop] [simple-cycle] Rate limit exceeded (45.2 triggers/min), entering backoff (level 1)
[devloop] [simple-cycle] In backoff period (level 1), skipping execution
[devloop] Cross-rule cycle detected: config-generator -> config-processor, skipping execution
[devloop] File thrashing detected: test-output.log (8 modifications in 10s), skipping rules
```

## Testing Different Configurations

### 1. No Cycle Detection
```bash
devloop server -c .devloop-no-cycles.yaml
```
- No warnings at startup
- All cycles will run without protection

### 2. Rate Limiting Test
```bash
devloop server -c test-rate-limit.yaml
```
- Tests aggressive rate limiting with low thresholds
- Demonstrates dynamic protection in action

### 3. Full Cycle Demo
```bash
devloop server
```
- Uses default .devloop.yaml with comprehensive cycle scenarios
- Shows both static warnings and dynamic protection

## File Structure

```
06-cycle-detection-demo/
├── README.md
├── Makefile
├── .devloop.yaml                    # Default config with cycle detection
├── .devloop-no-cycles.yaml         # Cycle detection disabled
├── .devloop-mixed-protection.yaml  # Mixed per-rule settings
├── .devloop-strict.yaml            # Strict cycle detection
├── backend/
│   ├── src/
│   │   └── main.go
│   └── bin/
├── frontend/
│   └── src/
│       └── app.js
├── input/
│   └── sample.yaml
├── generated/
└── logs/
```

## Key Features Demonstrated

1. **Static Analysis**: Detects potential cycles before runtime
2. **Pattern Resolution**: Workdir-relative pattern matching
3. **Flexible Configuration**: Global and per-rule settings
4. **Backward Compatibility**: Existing configs work without changes
5. **Warning System**: Informative but non-blocking warnings

## Advanced Usage

### Custom Cycle Detection Settings

```yaml
settings:
  cycle_detection:
    enabled: true
    static_validation: true
    max_triggers_per_minute: 5      # More restrictive rate limiting
    file_thrash_threshold: 3        # Lower thrashing threshold
```

### Selective Rule Protection

```yaml
rules:
  - name: "build-system"
    cycle_protection: true          # Force protection even if global is off
    
  - name: "log-aggregator"
    cycle_protection: false         # Allow self-referential logging
```

## Troubleshooting

- **Too many warnings**: Adjust `cycle_detection.enabled` or use per-rule overrides
- **False positives**: Use `cycle_protection: false` for specific rules
- **Missing cycles**: Enable `static_validation` and check pattern resolution

## Next Steps

This example demonstrates Phase 1 (Static Detection). Future phases will add:
- **Dynamic Rate Limiting**: Prevent runaway triggers
- **Trigger Chain Analysis**: Detect cross-rule cycles at runtime
- **Advanced Thrashing Detection**: File modification frequency analysis
