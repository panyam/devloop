package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogManager manages log files and provides streaming capabilities.
type LogManager struct {
	logDir string
	// Mutex to protect access to ruleStates and file handles
	mu sync.Mutex
	// Map to track the state of each rule's logging
	ruleStates map[string]*ruleLogState
}

// ruleLogState holds the state for a single rule's logging
type ruleLogState struct {
	// Channel to signal when a rule's execution starts
	started chan struct{}
	// Channel to signal when a rule's execution finishes
	finished chan struct{}
	// The file handle for the current log file
	file *os.File
	// Mutex to protect access to the file handle and its offset
	fileMu sync.Mutex
}

// NewLogManager creates a new LogManager instance.
func NewLogManager(logDir string) (*LogManager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %q: %w", logDir, err)
	}
	return &LogManager{
			logDir:     logDir,
			ruleStates: make(map[string]*ruleLogState),
		},
		nil
}

// GetWriter returns an io.Writer for a specific rule's log file.
// It also signals that the rule's execution has started.
func (lm *LogManager) GetWriter(ruleName string) (io.Writer, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state, ok := lm.ruleStates[ruleName]
	if !ok {
		state = &ruleLogState{
			started:  make(chan struct{}),
			finished: make(chan struct{}),
		}
		lm.ruleStates[ruleName] = state
	}

	state.fileMu.Lock()
	if state.file != nil {
		state.file.Close()
	}

	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		state.fileMu.Unlock()
		return nil, fmt.Errorf("failed to open log file %q for rule %q: %w", logFilePath, ruleName, err)
	}
	state.file = file
	state.fileMu.Unlock()

	// Signal that the rule has started (non-blocking send)
	select {
	case state.started <- struct{}{}:
	default:
	}

	return file, nil
}

// SignalFinished signals that a rule's execution has finished.
func (lm *LogManager) SignalFinished(ruleName string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if state, ok := lm.ruleStates[ruleName]; ok {
		// Signal that the rule has finished by closing the channel
		select {
		case <-state.finished:
			// Already closed
		default:
			close(state.finished)
		}
		if state.file != nil {
			state.file.Close()
			state.file = nil
		}
	}
}

// StreamLogs streams log content for a given rule.
// It blocks until the rule starts execution if not already running.
func (lm *LogManager) StreamLogs(ruleName string, filter string, w io.Writer) error {
	lm.mu.Lock()
	state, ok := lm.ruleStates[ruleName]
	if !ok {
		state = &ruleLogState{
			started:  make(chan struct{}),
			finished: make(chan struct{}),
		}
		lm.ruleStates[ruleName] = state
	}
	lm.mu.Unlock()

	// Wait for the rule to start if it's not already running
	select {
	case <-state.started:
		// Rule has started or was already started
	case <-time.After(10 * time.Second): // Timeout for waiting
		return fmt.Errorf("timeout waiting for rule %q to start", ruleName)
	}

	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))

	// Open the file for reading
	file, err := os.OpenFile(logFilePath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %q for streaming: %w", logFilePath, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading log file: %w", err)
		}

		if filter == "" || (filter != "" && contains(line, filter)) {
			if _, writeErr := io.WriteString(w, line); writeErr != nil {
				return fmt.Errorf("error writing log stream: %w", writeErr)
			}
		}

		if err == io.EOF {
			// If rule is finished, we're done
			select {
			case <-state.finished:
				return nil
			default:
				// Rule is still running, wait for more data
				time.Sleep(100 * time.Millisecond) // Poll for new data
			}
		}
	}
}

// contains is a helper for basic string filtering.
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Close closes all open file handles managed by the LogManager.
func (lm *LogManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var firstErr error
	for _, state := range lm.ruleStates {
		if state.file != nil {
			if err := state.file.Close(); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				log.Printf("Error closing log file for rule: %v", err)
			}
		}
	}
	return firstErr
}
