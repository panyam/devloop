package utils

import (
	"hash/fnv"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
)

// ColorSettings represents the color configuration needed by ColorManager
type ColorSettings interface {
	GetColorLogs() bool
	GetColorScheme() string
	GetCustomColors() map[string]string
}

// ColorRule represents a rule that can have colors applied to it
type ColorRule interface {
	GetName() string
	GetColor() string
	GetPrefix() string
}

// isTerminal checks if we're outputting to a terminal
func isTerminal() bool {
	// Use a simple heuristic - check if stdout is a terminal
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// ColorManager handles color assignment and formatting for rule output
type ColorManager struct {
	enabled      bool
	scheme       string
	customColors map[string]string
	colorMap     map[string]*color.Color
	darkPalette  []*color.Color
	lightPalette []*color.Color
	mu           sync.RWMutex // Protects colorMap from concurrent access
}

// NewColorManager creates a new color manager with the given settings
func NewColorManager(settings ColorSettings) *ColorManager {
	cm := &ColorManager{
		enabled:      settings.GetColorLogs(),
		scheme:       settings.GetColorScheme(),
		customColors: settings.GetCustomColors(),
		colorMap:     make(map[string]*color.Color),
	}

	// If color scheme is not specified, default to "auto"
	if cm.scheme == "" {
		cm.scheme = "auto"
	}

	// Initialize color palettes
	cm.initializePalettes()

	// Check if colors should be enabled based on settings and terminal capability
	// We check if devloop itself is running in a TTY, not subprocesses
	colorCapable := cm.isTerminalColorCapable()

	if !cm.enabled || !colorCapable {
		cm.enabled = false
	}

	// Do NOT set color.NoColor globally - this would suppress colors from subprocesses
	// like npm, go test, etc. Let subprocesses control their own color decisions.
	// Devloop will control its own prefix colors through the ColorManager methods.

	return cm
}

// initializePalettes sets up color palettes for different terminal backgrounds
func (cm *ColorManager) initializePalettes() {
	// Dark background palette - bright colors for good contrast
	cm.darkPalette = []*color.Color{
		color.New(color.FgCyan, color.Bold),
		color.New(color.FgYellow, color.Bold),
		color.New(color.FgGreen, color.Bold),
		color.New(color.FgMagenta, color.Bold),
		color.New(color.FgBlue, color.Bold),
		color.New(color.FgRed, color.Bold),
		color.New(color.FgHiCyan),
		color.New(color.FgHiYellow),
		color.New(color.FgHiGreen),
		color.New(color.FgHiMagenta),
		color.New(color.FgHiBlue),
		color.New(color.FgHiRed),
	}

	// Light background palette - darker colors for good contrast
	cm.lightPalette = []*color.Color{
		color.New(color.FgBlue),
		color.New(color.FgRed),
		color.New(color.FgMagenta),
		color.New(color.FgCyan),
		color.New(color.FgYellow),
		color.New(color.FgGreen),
		color.New(color.FgHiBlue),
		color.New(color.FgHiRed),
		color.New(color.FgHiMagenta),
		color.New(color.FgHiCyan),
		color.New(color.FgHiYellow),
		color.New(color.FgHiGreen),
	}
}

// GetColorForRule returns the appropriate color for a given rule
func (cm *ColorManager) GetColorForRule(rule ColorRule) *color.Color {
	if !cm.enabled {
		return nil
	}

	ruleName := rule.GetName()
	if rule.GetPrefix() != "" {
		ruleName = rule.GetPrefix()
	}

	// Check if we already have a color assigned for this rule (read lock)
	cm.mu.RLock()
	if existingColor, exists := cm.colorMap[ruleName]; exists {
		cm.mu.RUnlock()
		return existingColor
	}
	cm.mu.RUnlock()

	var ruleColor *color.Color

	// 1. Check for rule-specific color override
	if rule.GetColor() != "" {
		ruleColor = cm.parseColorString(rule.GetColor())
	}

	// 2. Check for custom color mapping
	if ruleColor == nil && cm.customColors != nil {
		if customColor, exists := cm.customColors[ruleName]; exists {
			ruleColor = cm.parseColorString(customColor)
		}
	}

	// 3. Auto-assign from palette
	if ruleColor == nil {
		ruleColor = cm.assignColorFromPalette(ruleName)
	}

	// Cache the color assignment (write lock)
	cm.mu.Lock()
	// Double-check in case another goroutine added it while we were computing
	if existingColor, exists := cm.colorMap[ruleName]; exists {
		cm.mu.Unlock()
		return existingColor
	}
	cm.colorMap[ruleName] = ruleColor
	cm.mu.Unlock()
	return ruleColor
}

// assignColorFromPalette assigns a color from the appropriate palette based on rule name
func (cm *ColorManager) assignColorFromPalette(ruleName string) *color.Color {
	palette := cm.getActivePalette()
	if len(palette) == 0 {
		return nil
	}

	// Use hash of rule name to get consistent color assignment
	hash := fnv.New32a()
	hash.Write([]byte(ruleName))
	index := int(hash.Sum32()) % len(palette)

	return palette[index]
}

// getActivePalette returns the palette to use based on color scheme
func (cm *ColorManager) getActivePalette() []*color.Color {
	switch cm.scheme {
	case "dark":
		return cm.darkPalette
	case "light":
		return cm.lightPalette
	case "auto":
		return cm.detectTerminalPalette()
	default:
		return cm.darkPalette // Default fallback
	}
}

// detectTerminalPalette attempts to detect the appropriate palette for the terminal
func (cm *ColorManager) detectTerminalPalette() []*color.Color {
	// Check various environment variables that might indicate terminal theme
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	colorTerm := strings.ToLower(os.Getenv("COLORTERM"))

	// Some terminals set specific variables
	if strings.Contains(termProgram, "dark") || strings.Contains(colorTerm, "dark") {
		return cm.darkPalette
	}
	if strings.Contains(termProgram, "light") || strings.Contains(colorTerm, "light") {
		return cm.lightPalette
	}

	// Check for common dark terminals
	if termProgram == "iterm.app" || termProgram == "hyper" || termProgram == "wezterm" {
		return cm.darkPalette
	}

	// Default to dark palette (most terminals use dark backgrounds)
	return cm.darkPalette
}

// parseColorString converts a color string to a color.Color object
func (cm *ColorManager) parseColorString(colorStr string) *color.Color {
	colorStr = strings.ToLower(strings.TrimSpace(colorStr))

	switch colorStr {
	case "black":
		return color.New(color.FgBlack)
	case "red":
		return color.New(color.FgRed)
	case "green":
		return color.New(color.FgGreen)
	case "yellow":
		return color.New(color.FgYellow)
	case "blue":
		return color.New(color.FgBlue)
	case "magenta":
		return color.New(color.FgMagenta)
	case "cyan":
		return color.New(color.FgCyan)
	case "white":
		return color.New(color.FgWhite)
	case "hi-black", "bright-black":
		return color.New(color.FgHiBlack)
	case "hi-red", "bright-red":
		return color.New(color.FgHiRed)
	case "hi-green", "bright-green":
		return color.New(color.FgHiGreen)
	case "hi-yellow", "bright-yellow":
		return color.New(color.FgHiYellow)
	case "hi-blue", "bright-blue":
		return color.New(color.FgHiBlue)
	case "hi-magenta", "bright-magenta":
		return color.New(color.FgHiMagenta)
	case "hi-cyan", "bright-cyan":
		return color.New(color.FgHiCyan)
	case "hi-white", "bright-white":
		return color.New(color.FgHiWhite)
	case "bold-red":
		return color.New(color.FgRed, color.Bold)
	case "bold-green":
		return color.New(color.FgGreen, color.Bold)
	case "bold-yellow":
		return color.New(color.FgYellow, color.Bold)
	case "bold-blue":
		return color.New(color.FgBlue, color.Bold)
	case "bold-magenta":
		return color.New(color.FgMagenta, color.Bold)
	case "bold-cyan":
		return color.New(color.FgCyan, color.Bold)
	case "bold-white":
		return color.New(color.FgWhite, color.Bold)
	default:
		return nil // Unknown color
	}
}

// isTerminalColorCapable checks if the terminal supports colors
func (cm *ColorManager) isTerminalColorCapable() bool {
	// Check for explicit NO_COLOR environment variable (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Check TERM environment variable
	term := os.Getenv("TERM")
	if term == "dumb" {
		return false
	}

	// If COLORTERM is set, assume color support
	if os.Getenv("COLORTERM") != "" {
		return true
	}

	// If TERM indicates color support, enable it
	if strings.Contains(term, "color") || strings.Contains(term, "256") ||
		term == "xterm" || term == "screen" || term == "tmux" {
		return true
	}

	// Check if we're outputting to a terminal as fallback
	if isTerminal() {
		return true
	}

	// Default to false for unknown cases
	return false
}

// FormatPrefix applies color formatting to a prefix string
func (cm *ColorManager) FormatPrefix(prefix string, rule interface{}) string {
	if !cm.enabled {
		return prefix
	}

	var ruleColor *color.Color

	// Handle different rule types
	if colorRule, ok := rule.(ColorRule); ok {
		ruleColor = cm.GetColorForRule(colorRule)
	} else if ruleMap, ok := rule.(map[string]interface{}); ok {
		// Handle simple map case from logger
		if name, hasName := ruleMap["Name"].(string); hasName {
			// Create a simple rule-like object for color generation
			simpleRule := &SimpleColorRule{name: name}
			ruleColor = cm.GetColorForRule(simpleRule)
		}
	}

	if ruleColor == nil {
		return prefix
	}

	// Use the color only if this ColorManager is enabled
	// Don't rely on global color.NoColor as that affects subprocesses
	if cm.enabled {
		return ruleColor.Sprint(prefix)
	}
	return prefix
}

// SimpleColorRule is a basic implementation of ColorRule for simple cases
type SimpleColorRule struct {
	name   string
	color  string
	prefix string
}

func (r *SimpleColorRule) GetName() string   { return r.name }
func (r *SimpleColorRule) GetColor() string  { return r.color }
func (r *SimpleColorRule) GetPrefix() string { return r.prefix }

// IsEnabled returns whether color formatting is enabled
func (cm *ColorManager) IsEnabled() bool {
	return cm.enabled
}

// DisableColors explicitly disables color output for this ColorManager only
func (cm *ColorManager) DisableColors() {
	cm.enabled = false
}

// GetColoredString returns a colored version of the input string for the given rule
func (cm *ColorManager) GetColoredString(text string, rule ColorRule) string {
	if !cm.enabled {
		return text
	}

	ruleColor := cm.GetColorForRule(rule)
	if ruleColor == nil {
		return text
	}

	// Use the color only if this ColorManager is enabled
	// Don't rely on global color.NoColor as that affects subprocesses
	if cm.enabled {
		return ruleColor.Sprint(text)
	}
	return text
}
