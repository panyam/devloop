package utils

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"sync"
)

// LogSource provides an interface for reading new log lines incrementally.
type LogSource interface {
	// ReadLines returns new lines since the last call. Returns nil, nil if no new content or file doesn't exist yet.
	ReadLines(ctx context.Context) ([]string, error)
	// Reset seeks back to the beginning (e.g. after file truncation).
	Reset() error
	// Close releases the underlying resource.
	Close() error
}

// FileLogSource reads new lines from a log file, detecting truncation.
type FileLogSource struct {
	path   string
	file   *os.File
	reader *bufio.Reader
	offset int64
	mu     sync.Mutex
}

// NewFileLogSource creates a FileLogSource for the given path.
// The file is opened lazily on the first ReadLines call.
func NewFileLogSource(path string) *FileLogSource {
	return &FileLogSource{path: path}
}

// ReadLines returns new complete lines since the last call.
// Returns nil, nil if the file doesn't exist yet or has no new content.
func (fs *FileLogSource) ReadLines(ctx context.Context) ([]string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Lazy open
	if fs.file == nil {
		f, err := os.Open(fs.path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		fs.file = f
		fs.reader = bufio.NewReader(f)
		fs.offset = 0
	}

	// Detect truncation: current file size < our offset
	info, err := os.Stat(fs.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was removed — close handle and return nil
			fs.closeInternal()
			return nil, nil
		}
		return nil, err
	}
	if info.Size() < fs.offset {
		// File was truncated — reset
		if err := fs.resetInternal(); err != nil {
			return nil, err
		}
	}

	var lines []string
	for {
		if ctx.Err() != nil {
			break
		}
		line, err := fs.reader.ReadString('\n')
		if len(line) > 0 {
			// Only emit complete lines (ending with \n)
			if strings.HasSuffix(line, "\n") {
				fs.offset += int64(len(line))
				lines = append(lines, strings.TrimSuffix(line, "\n"))
			} else {
				// Incomplete line — seek back so we re-read it next time
				fs.file.Seek(fs.offset, io.SeekStart)
				fs.reader.Reset(fs.file)
				break
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return lines, err
		}
	}
	return lines, nil
}

// Reset re-opens the file from the beginning.
func (fs *FileLogSource) Reset() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.resetInternal()
}

func (fs *FileLogSource) resetInternal() error {
	if fs.file != nil {
		fs.file.Seek(0, io.SeekStart)
		fs.reader.Reset(fs.file)
	}
	fs.offset = 0
	return nil
}

// Close releases the file handle.
func (fs *FileLogSource) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.closeInternal()
}

func (fs *FileLogSource) closeInternal() error {
	if fs.file != nil {
		err := fs.file.Close()
		fs.file = nil
		fs.reader = nil
		fs.offset = 0
		return err
	}
	return nil
}
