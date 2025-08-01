package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestStripColorCodes verifies that ANSI color codes are properly stripped
func TestStripColorCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "red text",
			input:    "\x1b[31mError: Something went wrong\x1b[0m",
			expected: "Error: Something went wrong",
		},
		{
			name:     "green text with bold",
			input:    "\x1b[32m\x1b[1mSuccess!\x1b[0m",
			expected: "Success!",
		},
		{
			name:     "npm-style colored output",
			input:    "\x1b[32mnpm\x1b[0m \x1b[37mWARN\x1b[0m \x1b[35mdeprecated\x1b[0m some-package@1.0.0",
			expected: "npm WARN deprecated some-package@1.0.0",
		},
		{
			name:     "mixed content",
			input:    "Normal text \x1b[31mred error\x1b[0m more normal text",
			expected: "Normal text red error more normal text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripColorCodes(tt.input)
			if result != tt.expected {
				t.Errorf("StripColorCodes() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestStripANSIFromBytes verifies that the byte version works correctly
func TestStripANSIFromBytes(t *testing.T) {
	input := []byte("\x1b[31mError: Something went wrong\x1b[0m")
	expected := []byte("Error: Something went wrong")
	
	result := stripANSIFromBytes(input)
	if !bytes.Equal(result, expected) {
		t.Errorf("stripANSIFromBytes() = %q, want %q", result, expected)
	}
}

// Mock rule for testing
type mockRule struct {
	name   string
	prefix string
	color  string
}

func (r *mockRule) GetName() string   { return r.name }
func (r *mockRule) GetPrefix() string { return r.prefix }
func (r *mockRule) GetColor() string  { return r.color }

// TestColoredPrefixWriter_PreservesColorsForTerminal verifies that colors are preserved for terminal output
func TestColoredPrefixWriter_PreservesColorsForTerminal(t *testing.T) {
	var terminalBuf bytes.Buffer
	var fileBuf bytes.Buffer
	
	rule := &mockRule{name: "test", prefix: "test", color: "red"}
	
	// Create a simple color manager that's disabled (no colors for prefixes)
	colorManager := &ColorManager{enabled: false}
	
	// Create the writer manually to specify which writers are terminals vs files
	cpw := &ColoredPrefixWriter{
		terminalWriters: []io.Writer{&terminalBuf}, // This should preserve colors
		fileWriters:     []io.Writer{&fileBuf},     // This should strip colors
		prefix:          "[test] ",
		coloredPrefix:   "[test] ",
		colorManager:    colorManager,
		rule:            rule,
	}
	
	// Write some colored content (simulating npm output)
	coloredContent := "\x1b[32mnpm\x1b[0m install completed \x1b[31mwith errors\x1b[0m"
	cpw.Write([]byte(coloredContent))
	
	terminalOutput := terminalBuf.String()
	fileOutput := fileBuf.String()
	
	// Terminal output should preserve colors
	if !strings.Contains(terminalOutput, "\x1b[32mnpm\x1b[0m") || !strings.Contains(terminalOutput, "\x1b[31mwith errors\x1b[0m") {
		t.Errorf("Terminal output should preserve ANSI colors, got: %q", terminalOutput)
	}
	
	// File output should strip colors
	if strings.Contains(fileOutput, "\x1b[") {
		t.Errorf("File output should not contain ANSI codes, got: %q", fileOutput)
	}
	
	// File output should contain the clean text
	expectedCleanText := "npm install completed with errors"
	if !strings.Contains(fileOutput, expectedCleanText) {
		t.Errorf("File output should contain clean text %q, got: %q", expectedCleanText, fileOutput)
	}
}