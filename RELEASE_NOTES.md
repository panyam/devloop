# Devloop Release Notes

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