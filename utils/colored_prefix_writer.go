package utils

import (
	"bytes"
	"io"
	"os"
	"regexp"
)

// Nameable represents anything that has a name (used for color generation)
type Nameable interface {
	GetName() string
}

// ColoredPrefixWriter is an enhanced io.Writer that adds colored prefixes to each line of output
// It separates terminal output (with colors) from file output (without colors)
type ColoredPrefixWriter struct {
	terminalWriters []io.Writer // Writers that support colors (like os.Stdout)
	fileWriters     []io.Writer // Writers that should not have colors (like log files)
	prefix          string
	coloredPrefix   string
	colorManager    *ColorManager
	rule            Nameable
}

// NewColoredPrefixWriter creates a new ColoredPrefixWriter
func NewColoredPrefixWriter(writers []io.Writer, prefix string, colorManager *ColorManager, rule Nameable) *ColoredPrefixWriter {
	// Separate terminal writers from file writers
	var terminalWriters, fileWriters []io.Writer

	for _, writer := range writers {
		if writer == os.Stdout || writer == os.Stderr {
			terminalWriters = append(terminalWriters, writer)
		} else {
			fileWriters = append(fileWriters, writer)
		}
	}

	// Create colored prefix for terminal output
	coloredPrefix := prefix
	if colorManager != nil && colorManager.IsEnabled() {
		coloredPrefix = colorManager.FormatPrefix(prefix, rule)
	}

	return &ColoredPrefixWriter{
		terminalWriters: terminalWriters,
		fileWriters:     fileWriters,
		prefix:          prefix,
		coloredPrefix:   coloredPrefix,
		colorManager:    colorManager,
		rule:            rule,
	}
}

// Write implements the io.Writer interface with color support
func (cpw *ColoredPrefixWriter) Write(p []byte) (n int, err error) {
	lines := bytes.Split(p, []byte("\n"))

	// Write colored output to terminal writers
	if len(cpw.terminalWriters) > 0 {
		var coloredOutput []byte
		for i, line := range lines {
			if len(line) > 0 {
				coloredOutput = append(coloredOutput, []byte(cpw.coloredPrefix)...)
				coloredOutput = append(coloredOutput, line...)
				if i < len(lines)-1 {
					coloredOutput = append(coloredOutput, '\n')
				}
			}
		}

		for _, w := range cpw.terminalWriters {
			_, err = w.Write(coloredOutput)
			if err != nil {
				return n, err
			}
		}
	}

	// Write plain output to file writers (strip ANSI codes for clean logs)
	if len(cpw.fileWriters) > 0 {
		var plainOutput []byte
		for i, line := range lines {
			if len(line) > 0 {
				// Add the plain prefix (without colors)
				plainOutput = append(plainOutput, []byte(cpw.prefix)...)
				// Strip ANSI codes from the subprocess output for clean file logs
				cleanLine := stripANSIFromBytes(line)
				plainOutput = append(plainOutput, cleanLine...)
				if i < len(lines)-1 {
					plainOutput = append(plainOutput, '\n')
				}
			}
		}

		for _, w := range cpw.fileWriters {
			_, err = w.Write(plainOutput)
			if err != nil {
				return n, err
			}
		}
	}

	return len(p), nil
}

// UpdatePrefix updates the prefix and regenerates the colored version
func (cpw *ColoredPrefixWriter) UpdatePrefix(newPrefix string) {
	cpw.prefix = newPrefix
	if cpw.colorManager != nil && cpw.colorManager.IsEnabled() {
		cpw.coloredPrefix = cpw.colorManager.FormatPrefix(newPrefix, cpw.rule)
	} else {
		cpw.coloredPrefix = newPrefix
	}
}

// ANSI escape sequence regex pattern - matches color codes, cursor movements, etc.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripColorCodes removes ANSI color codes from a string using regex
func StripColorCodes(input string) string {
	return ansiRegex.ReplaceAllString(input, "")
}

// stripANSIFromBytes removes ANSI escape sequences from byte slice
func stripANSIFromBytes(input []byte) []byte {
	return ansiRegex.ReplaceAll(input, []byte{})
}

// WriteToFileOnly writes output only to file writers (without colors)
func (cpw *ColoredPrefixWriter) WriteToFileOnly(p []byte) (n int, err error) {
	if len(cpw.fileWriters) == 0 {
		return len(p), nil
	}

	lines := bytes.Split(p, []byte("\n"))
	var plainOutput []byte

	for i, line := range lines {
		if len(line) > 0 {
			plainOutput = append(plainOutput, []byte(cpw.prefix)...)
			// Strip ANSI codes from subprocess output for clean file logs
			cleanLine := stripANSIFromBytes(line)
			plainOutput = append(plainOutput, cleanLine...)
			if i < len(lines)-1 {
				plainOutput = append(plainOutput, '\n')
			}
		}
	}

	for _, w := range cpw.fileWriters {
		_, err = w.Write(plainOutput)
		if err != nil {
			return n, err
		}
	}

	return len(p), nil
}

// WriteToTerminalOnly writes output only to terminal writers (with colors)
func (cpw *ColoredPrefixWriter) WriteToTerminalOnly(p []byte) (n int, err error) {
	if len(cpw.terminalWriters) == 0 {
		return len(p), nil
	}

	lines := bytes.Split(p, []byte("\n"))
	var coloredOutput []byte

	for i, line := range lines {
		if len(line) > 0 {
			coloredOutput = append(coloredOutput, []byte(cpw.coloredPrefix)...)
			coloredOutput = append(coloredOutput, line...)
			if i < len(lines)-1 {
				coloredOutput = append(coloredOutput, '\n')
			}
		}
	}

	for _, w := range cpw.terminalWriters {
		_, err = w.Write(coloredOutput)
		if err != nil {
			return n, err
		}
	}

	return len(p), nil
}
