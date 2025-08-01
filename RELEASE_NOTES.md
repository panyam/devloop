# Devloop Release Notes

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