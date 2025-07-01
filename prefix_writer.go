package main

import (
	"bytes"
	"io"
)

// PrefixWriter is an io.Writer that adds a prefix to each line of output.
type PrefixWriter struct {
	writer io.Writer
	prefix string
}

// Write implements the io.Writer interface.
func (pw *PrefixWriter) Write(p []byte) (n int, err error) {
	var output []byte
	lines := bytes.Split(p, []byte("\n"))
	for i, line := range lines {
		if len(line) > 0 {
			output = append(output, []byte(pw.prefix)...)
			output = append(output, line...)
			if i < len(lines)-1 {
				output = append(output, '\n')
			}
		}
	}
	_, err = pw.writer.Write(output)
	return len(p), err
}
