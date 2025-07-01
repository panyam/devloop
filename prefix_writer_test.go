package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixWriter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "Single line",
			input:    "hello",
			prefix:   "[test] ",
			expected: "[test] hello",
		},
		{
			name:     "Multiple lines",
			input:    "hello\nworld",
			prefix:   "[test] ",
			expected: "[test] hello\n[test] world",
		},
		{
			name:     "Multiple lines with trailing newline",
			input:    "hello\nworld\n",
			prefix:   "[test] ",
			expected: "[test] hello\n[test] world\n",
		},
		{
			name:     "Empty input",
			input:    "",
			prefix:   "[test] ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := &PrefixWriter{writers: []io.Writer{&buf}, prefix: tt.prefix}
			_, err := writer.Write([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}
