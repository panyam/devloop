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

	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
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
	}, nil
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

	// Signal that the rule has started by closing the channel
	select {
	case <-state.started:
		// Already closed
	default:
		close(state.started)
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

// StreamLogs streams logs for a given rule to the provided gRPC stream.
func (lm *LogManager) StreamLogs(ruleName, filter string, stream pb.GatewayClientService_StreamLogsClientServer) error {
	lm.mu.Lock()
	state, ok := lm.ruleStates[ruleName]
	if !ok {
		lm.mu.Unlock()
		return fmt.Errorf("no state found for rule: %s", ruleName)
	}
	lm.mu.Unlock()

	// Wait for the rule to start. This is important for realtime streaming.
	select {
	case <-state.started:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}

	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	file, err := os.Open(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to open log file for streaming: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-state.finished:
			// Rule is finished. Drain any remaining content from the reader and exit.
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					if filter == "" || contains(line, filter) {
						if sendErr := stream.Send(&pb.LogLine{Line: line}); sendErr != nil {
							return sendErr
						}
					}
				}
				if err == io.EOF {
					return nil // Successfully drained and finished.
				}
				if err != nil {
					return err // Return other errors.
				}
			}
		default:
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				// If we're at the end of the file, check if the rule is finished.
				select {
				case <-state.finished:
					return nil
				default:
					// Rule is not finished, so wait for more content.
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}
			if err != nil {
				return err
			}
			if filter == "" || contains(line, filter) {
				if sendErr := stream.Send(&pb.LogLine{Line: line}); sendErr != nil {
					return sendErr
				}
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
