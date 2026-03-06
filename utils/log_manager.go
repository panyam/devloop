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

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/gocurrent"
)

// LogManager manages log files and provides streaming capabilities.
// It uses a single LogBroadcaster per rule to fan out log lines
// to all subscribers instead of each client polling independently.
type LogManager struct {
	logDir string
	mu     sync.Mutex
	// Map to track which rules have finished execution
	finishedRules map[string]bool
	// One broadcaster per rule — lazy-created
	broadcasters map[string]*LogBroadcaster
}

// NewLogManager creates a new LogManager instance.
func NewLogManager(logDir string) (*LogManager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %q: %w", logDir, err)
	}
	return &LogManager{
		logDir:        logDir,
		finishedRules: make(map[string]bool),
		broadcasters:  make(map[string]*LogBroadcaster),
	}, nil
}

// GetWriter returns an io.Writer for a specific rule's log file.
// When appendOnRestart is true, the log file is opened in append mode and a
// separator line is written to mark the start of a new run. When false (default),
// the file is truncated on each run.
func (lm *LogManager) GetWriter(ruleName string, appendOnRestart bool) (io.Writer, error) {
	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))

	flags := os.O_CREATE | os.O_WRONLY
	if appendOnRestart {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(logFilePath, flags, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %q for rule %q: %w", logFilePath, ruleName, err)
	}

	// In append mode, write a separator line to mark the new run
	if appendOnRestart {
		separator := fmt.Sprintf("\n--- [rule: %s] run started at %s ---\n", ruleName, time.Now().Format(time.RFC3339))
		if _, err := file.WriteString(separator); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write separator to log file %q: %w", logFilePath, err)
		}
	}

	// Clear finished state so StreamLogs uses the live path for this new run
	lm.mu.Lock()
	delete(lm.finishedRules, ruleName)
	// Signal broadcaster for new run if one exists
	if b, ok := lm.broadcasters[ruleName]; ok {
		b.SignalNewRun(appendOnRestart)
		b.BroadcastEvent(&pb.LogEvent{
			RuleName:  ruleName,
			Type:      pb.LogEventType_LOG_EVENT_TYPE_RUN_STARTED,
			Timestamp: time.Now().UnixMilli(),
			Truncated: !appendOnRestart,
		})
	}
	lm.mu.Unlock()

	return file, nil
}

// SignalFinished signals that a rule's execution has finished.
// success indicates whether the run completed successfully.
// errMsg is an optional error message when success is false.
func (lm *LogManager) SignalFinished(ruleName string, success bool, errMsg string) {
	lm.mu.Lock()
	lm.finishedRules[ruleName] = true
	b := lm.broadcasters[ruleName]
	lm.mu.Unlock()

	if b != nil {
		eventType := pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED
		if !success {
			eventType = pb.LogEventType_LOG_EVENT_TYPE_RUN_FAILED
		}
		b.BroadcastEvent(&pb.LogEvent{
			RuleName:  ruleName,
			Type:      eventType,
			Timestamp: time.Now().UnixMilli(),
			Message:   errMsg,
		})
		b.SignalFinished()
	}
}

// getOrCreateBroadcaster returns the broadcaster for a rule, creating one if needed.
// Caller must NOT hold lm.mu.
func (lm *LogManager) getOrCreateBroadcaster(ruleName string) *LogBroadcaster {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if b, ok := lm.broadcasters[ruleName]; ok {
		return b
	}

	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	source := NewFileLogSource(logFilePath)
	b := NewLogBroadcaster(ruleName, source)
	lm.broadcasters[ruleName] = b
	return b
}

// StreamLogs streams logs for a given rule to the provided gocurrent Writer.
// lastNLines controls history replay: 0 = no history, negative = all available.
func (lm *LogManager) StreamLogs(ruleName, filter string, timeoutSeconds int64, lastNLines int32, writer *gocurrent.Writer[*pb.StreamLogsResponse]) error {
	// Check if log file exists
	logFilePath := filepath.Join(lm.logDir, fmt.Sprintf("%s.log", ruleName))
	if _, err := os.Stat(logFilePath); err != nil {
		return fmt.Errorf("log file not found for rule %q", ruleName)
	}

	// If the rule is already finished, read the file directly and return.
	// This handles the case where content was written before a broadcaster existed.
	lm.mu.Lock()
	ruleFinished := lm.finishedRules[ruleName]
	lm.mu.Unlock()

	if ruleFinished {
		return lm.streamFinishedLogs(logFilePath, ruleName, filter, writer)
	}

	b := lm.getOrCreateBroadcaster(ruleName)

	// Send history if requested
	if lastNLines != 0 {
		history := b.GetHistory(int(lastNLines))
		if len(history) > 0 {
			now := time.Now().UnixMilli()
			for _, line := range history {
				if filter != "" && !contains(line, filter) {
					continue
				}
				resp := &pb.StreamLogsResponse{
					Lines: []*pb.LogLine{{
						RuleName:  ruleName,
						Line:      line,
						Timestamp: now,
					}},
				}
				if !writer.Send(resp) {
					return fmt.Errorf("failed to send history line")
				}
			}
		}
	}

	// Subscribe for live updates
	subID := fmt.Sprintf("%s-%d", ruleName, time.Now().UnixNano())
	sub := b.Subscribe(subID, filter)
	defer b.Unsubscribe(subID)

	// Setup timeout
	var timeoutDuration time.Duration
	var timeoutTimer *time.Timer
	var timeoutChan <-chan time.Time

	if timeoutSeconds < 0 {
		timeoutChan = nil
	} else {
		if timeoutSeconds == 0 {
			timeoutSeconds = 3
		}
		timeoutDuration = time.Duration(timeoutSeconds) * time.Second
		timeoutTimer = time.NewTimer(timeoutDuration)
		defer timeoutTimer.Stop()
		timeoutChan = timeoutTimer.C
	}

	for {
		select {
		case batch := <-sub.Ch:
			for _, logLine := range batch {
				resp := &pb.StreamLogsResponse{
					Lines: []*pb.LogLine{logLine},
				}
				if !writer.Send(resp) {
					return fmt.Errorf("failed to send log line")
				}
			}
			// Reset timeout on new content
			if timeoutTimer != nil {
				if !timeoutTimer.Stop() {
					select {
					case <-timeoutTimer.C:
					default:
					}
				}
				timeoutTimer.Reset(timeoutDuration)
			}

		case event := <-sub.Events:
			writer.Send(&pb.StreamLogsResponse{Event: event})

		case <-sub.Done:
			// Drain remaining batches and events from channels
			for {
				select {
				case batch := <-sub.Ch:
					for _, logLine := range batch {
						writer.Send(&pb.StreamLogsResponse{
							Lines: []*pb.LogLine{logLine},
						})
					}
				case event := <-sub.Events:
					writer.Send(&pb.StreamLogsResponse{Event: event})
				default:
					goto drained
				}
			}
		drained:
			return nil

		case <-timeoutChan:
			writer.Send(&pb.StreamLogsResponse{
				Event: &pb.LogEvent{
					RuleName:  ruleName,
					Type:      pb.LogEventType_LOG_EVENT_TYPE_TIMEOUT,
					Timestamp: time.Now().UnixMilli(),
					Message:   fmt.Sprintf("No new content for %d seconds", timeoutSeconds),
				},
			})
			return nil
		}
	}
}

// streamFinishedLogs reads a completed rule's log file and sends all lines.
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
			line = strings.TrimSuffix(line, "\n")
			if filter == "" || contains(line, filter) {
				resp := &pb.StreamLogsResponse{
					Lines: []*pb.LogLine{{
						RuleName:  ruleName,
						Line:      line,
						Timestamp: time.Now().UnixMilli(),
					}},
				}
				if !writer.Send(resp) {
					return fmt.Errorf("failed to send log line")
				}
			}
		}
		if err == io.EOF {
			writer.Send(&pb.StreamLogsResponse{
				Event: &pb.LogEvent{
					RuleName:  ruleName,
					Type:      pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED,
					Timestamp: time.Now().UnixMilli(),
				},
			})
			return nil
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

// Close closes the LogManager and stops all broadcasters.
func (lm *LogManager) Close() error {
	lm.mu.Lock()
	broadcasters := make(map[string]*LogBroadcaster, len(lm.broadcasters))
	for k, v := range lm.broadcasters {
		broadcasters[k] = v
	}
	lm.broadcasters = make(map[string]*LogBroadcaster)
	lm.mu.Unlock()

	for _, b := range broadcasters {
		b.Stop()
	}
	return nil
}
