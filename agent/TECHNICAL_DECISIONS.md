# Technical Decisions Log

This document captures important technical decisions made during the development of devloop, including the rationale and trade-offs considered.

## 2025-07-02: Glob Pattern Library Migration

### Decision: Switch from gobwas/glob to bmatcuk/doublestar

**Context:**
- Users expect `src/**/*.go` to match both `src/main.go` AND `src/pkg/utils.go`
- The `gobwas/glob` library treats `**` as matching one or more directories
- This behavior differs from common tools like git, VS Code, Docker, etc.

**Options Considered:**
1. Keep gobwas/glob and rewrite patterns internally (`src/**/*.go` → `src/{*.go,**/*.go}`)
2. Keep gobwas/glob and add a compatibility flag
3. Switch to a library that follows conventions (doublestar)

**Decision:**
Switched to `bmatcuk/doublestar` because:
- It follows the widely-expected convention where `**` matches zero or more directories
- Simplifies the codebase (no pattern rewriting needed)
- Reduces cognitive load for users
- Well-maintained library with good performance

**Trade-offs:**
- Changed a dependency (minimal risk as the API is simple)
- Slightly different performance characteristics (acceptable)

## 2025-07-02: File Watcher Initialization

### Decision: Start file watcher from project root instead of current directory

**Context:**
- File watcher was starting from current working directory
- This caused issues when devloop was run from different directories
- Tests were failing because files weren't being watched properly

**Decision:**
Modified the `Start()` method to walk from `ProjectRoot()` (directory containing config file) instead of `"."`.

**Benefits:**
- Consistent behavior regardless of where devloop is executed
- Patterns in config file work as expected
- Better aligns with user mental model

## 2025-07-02: Code Organization

### Decision: Move core logic to agent/ directory

**Context:**
- All Go files were at the project root
- Made it harder to distinguish between entry point and core logic

**Decision:**
- Moved all Go files (except main.go) to `agent/` directory
- Created `utils/` directory for utility functions
- Kept `gateway/` separate for gateway-specific logic

**Benefits:**
- Clearer separation of concerns
- Easier to navigate codebase
- Better modularity for future enhancements

## Log Manager Channel Signaling

### Decision: Use channel closing instead of sending values

**Context:**
- Log manager was trying to signal rule start by sending to a channel
- This could block if no receiver was ready

**Decision:**
Changed from sending values to closing the channel to signal events.

**Benefits:**
- Non-blocking signal
- Can be checked multiple times
- Standard Go pattern for signaling events