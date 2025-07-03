// Package utils provides common utilities for devloop.
//
// This package contains shared functionality used across the devloop ecosystem:
// - Logging infrastructure with colored prefix support
// - Color management for terminal output
// - Log management and streaming capabilities
// - Prefix writers for enhanced output formatting
//
// # Logging
//
// The package provides structured logging with prefix support:
//
//	logger := utils.NewDevloopLogger(true, 10, colorManager)
//	logger.LogWithPrefix("build", "Starting build process")
//	// Output: [build     ] Starting build process
//
// # Color Management
//
// Color management provides consistent terminal coloring:
//
//	colorManager := utils.NewColorManager(true)
//	coloredText := colorManager.FormatPrefix("[build]", rule)
//
// # Log Streaming
//
// Log streaming enables real-time log access:
//
//	logManager := utils.NewLogManager(logsDir)
//	err := logManager.StreamLogs("ruleName", "filter", stream)
//
// # Prefix Writers
//
// Enhanced output writers with prefix and color support:
//
//	writer := utils.NewColoredPrefixWriter(writers, "[prefix] ", colorManager, rule)
//	writer.Write([]byte("Hello World"))
//	// Output: [prefix] Hello World (with colors)
package utils

import (
	"fmt"
	"log"
	"strings"
)

// LoggerConfig holds configuration for consistent logging
type LoggerConfig struct {
	PrefixLogs      bool
	PrefixMaxLength int
	ColorManager    ColorFormatter
}

// ColorFormatter interface for formatting colored prefixes
type ColorFormatter interface {
	IsEnabled() bool
	FormatPrefix(prefix string, rule interface{}) string
}

// DevloopLogger provides consistent logging across all components
type DevloopLogger struct {
	config *LoggerConfig
}

// NewDevloopLogger creates a new logger with the given configuration
func NewDevloopLogger(prefixLogs bool, prefixMaxLength int, colorManager ColorFormatter) *DevloopLogger {
	return &DevloopLogger{
		config: &LoggerConfig{
			PrefixLogs:      prefixLogs,
			PrefixMaxLength: prefixMaxLength,
			ColorManager:    colorManager,
		},
	}
}

// LogWithPrefix logs a message with the specified prefix (e.g., "devloop", "gateway", "mcp")
func (dl *DevloopLogger) LogWithPrefix(prefix, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	if dl.config.PrefixLogs && dl.config.PrefixMaxLength > 0 {
		// Format with left-aligned prefix to match rule output format
		totalPadding := dl.config.PrefixMaxLength - len(prefix)
		leftAlignedPrefix := prefix + strings.Repeat(" ", totalPadding)
		prefixStr := "[" + leftAlignedPrefix + "] "

		// Add color if enabled
		if dl.config.ColorManager != nil && dl.config.ColorManager.IsEnabled() {
			// Create a simple map for the prefix to get consistent coloring
			rule := map[string]interface{}{"Name": prefix}
			coloredPrefix := dl.config.ColorManager.FormatPrefix(prefixStr, rule)
			fmt.Printf("%s%s\n", coloredPrefix, message)
		} else {
			fmt.Printf("%s%s\n", prefixStr, message)
		}
	} else {
		// Standard log format but with prefix color if available
		if dl.config.ColorManager != nil && dl.config.ColorManager.IsEnabled() {
			rule := map[string]interface{}{"Name": prefix}
			coloredPrefix := dl.config.ColorManager.FormatPrefix("["+prefix+"]", rule)
			log.Printf("%s %s", coloredPrefix, message)
		} else {
			log.Printf("[%s] %s", prefix, message)
		}
	}
}

// Global logger instance that can be used before orchestrator initialization
var globalLogger *DevloopLogger

// InitGlobalLogger initializes the global logger with configuration
func InitGlobalLogger(prefixLogs bool, prefixMaxLength int, colorManager ColorFormatter) {
	globalLogger = NewDevloopLogger(prefixLogs, prefixMaxLength, colorManager)
}

// LogDevloop logs a devloop message using the global logger
func LogDevloop(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.LogWithPrefix("devloop", format, args...)
	} else {
		log.Printf("[devloop] "+format, args...)
	}
}

// LogGateway logs a gateway message using the global logger
func LogGateway(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.LogWithPrefix("gateway", format, args...)
	} else {
		log.Printf("[gateway] "+format, args...)
	}
}

// LogMCP logs an MCP message using the global logger
func LogMCP(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.LogWithPrefix("mcp", format, args...)
	} else {
		log.Printf("[mcp] "+format, args...)
	}
}
