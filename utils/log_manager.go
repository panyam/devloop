package utils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/panyam/gocurrent"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// LogManager manages log files and provides streaming capabilities.
type LogManager struct {
	logDir string
	// Mutex to protect access to finishedRules map
	mu sync.Mutex
	// Map to track which rules have finished execution
	finishedRules map[string]bool
}

// NewLogManager creates a new LogManager instance.
func NewLogManager(logDir string) (*LogManager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %q: %w", logDir, err)
	}
	return &LogManager{
		logDir:        logDir,
		finishedRules: make(map[string]bool),
	}, nil
}

// GetWriter returns an io.Writer for a specific rule's log file.
func (lm *LogManager) GetWriter(ruleName string) (io.Writer, error) {
	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %q for rule %q: %w", logFilePath, ruleName, err)
	}
	return file, nil
}

// SignalFinished signals that a rule's execution has finished.
func (lm *LogManager) SignalFinished(ruleName string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.finishedRules[ruleName] = true
}

// StreamLogs streams logs for a given rule to the provided gocurrent Writer.
func (lm *LogManager) StreamLogs(ruleName, filter string, writer *gocurrent.Writer[*pb.StreamLogsResponse]) error {
	// Check if log file exists
	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	_, err := os.Stat(logFilePath)
	if err != nil {
		return fmt.Errorf("log file not found for rule %q", ruleName)
	}

	// Check if rule has finished
	lm.mu.Lock()
	ruleFinished := lm.finishedRules[ruleName]
	lm.mu.Unlock()

	if ruleFinished {
		// Rule has finished - stream all content and exit
		return lm.streamFinishedLogs(logFilePath, ruleName, filter, writer)
	} else {
		// Rule is still running - stream in real-time
		return lm.streamLiveLogs(logFilePath, ruleName, filter, writer)
	}
}

// streamFinishedLogs streams all logs for a rule that has finished execution
func (lm *LogManager) streamFinishedLogs(logFilePath, ruleName, filter string, writer *gocurrent.Writer[*pb.StreamLogsResponse]) error {
	file, err := os.Open(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to open log file for streaming: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n") // Remove trailing newline
			if filter == "" || contains(line, filter) {
				response := &pb.StreamLogsResponse{
					Lines: []*pb.LogLine{
						{
							RuleName:  ruleName,
							Line:      line,
							Timestamp: time.Now().UnixMilli(),
						},
					},
				}
				if !writer.Send(response) {
					return fmt.Errorf("failed to send log line")
				}
			}
		}
		if err == io.EOF {
			// Send final completion message
			finalMsg := &pb.StreamLogsResponse{
				Lines: []*pb.LogLine{
					{
						RuleName:  ruleName,
						Line:      fmt.Sprintf("Rule '%s' execution completed", ruleName),
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}
			writer.Send(finalMsg)
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// streamLiveLogs streams logs for a rule that is currently running
func (lm *LogManager) streamLiveLogs(logFilePath, ruleName, filter string, writer *gocurrent.Writer[*pb.StreamLogsResponse]) error {
	file, err := os.Open(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to open log file for streaming: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	for {
		// Check if rule has finished (without blocking)
		lm.mu.Lock()
		ruleFinished := lm.finishedRules[ruleName]
		lm.mu.Unlock()

		if ruleFinished {
			// Rule finished while we were streaming - drain remaining content
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					line = strings.TrimSuffix(line, "\n")
					if filter == "" || contains(line, filter) {
						response := &pb.StreamLogsResponse{
							Lines: []*pb.LogLine{
								{
									RuleName:  ruleName,
									Line:      line,
									Timestamp: time.Now().UnixMilli(),
								},
							},
						}
						if !writer.Send(response) {
							return fmt.Errorf("failed to send log line")
						}
					}
				}
				if err == io.EOF {
					// Send completion message and exit
					finalMsg := &pb.StreamLogsResponse{
						Lines: []*pb.LogLine{
							{
								RuleName:  ruleName,
								Line:      fmt.Sprintf("Rule '%s' execution completed", ruleName),
								Timestamp: time.Now().UnixMilli(),
							},
						},
					}
					writer.Send(finalMsg)
					return nil
				}
				if err != nil {
					return err
				}
			}
		}

		// Try to read a line
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			if filter == "" || contains(line, filter) {
				response := &pb.StreamLogsResponse{
					Lines: []*pb.LogLine{
						{
							RuleName:  ruleName,
							Line:      line,
							Timestamp: time.Now().UnixMilli(),
						},
					},
				}
				if !writer.Send(response) {
					return fmt.Errorf("failed to send log line")
				}
			}
		}
		if err == io.EOF {
			// At end of file but rule is still running - wait and try again
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}

// contains is a helper for basic string filtering.
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Close closes the LogManager and cleans up resources.
func (lm *LogManager) Close() error {
	// With simplified design, we don't manage open file handles
	// Individual file operations open/close their own handles
	return nil
}
