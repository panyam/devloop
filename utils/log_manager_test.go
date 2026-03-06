package utils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/testhelpers"
	"github.com/panyam/gocurrent"
)

// mockWriter collects StreamLogsResponse messages for testing
type mockWriter struct {
	buffer   *bytes.Buffer
	mu       sync.Mutex
	messages []*pb.StreamLogsResponse
}

func newMockWriter() *mockWriter {
	return &mockWriter{
		buffer:   &bytes.Buffer{},
		messages: make([]*pb.StreamLogsResponse, 0),
	}
}

func (m *mockWriter) write(response *pb.StreamLogsResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, response)
	if response.Lines != nil {
		for _, logLine := range response.Lines {
			m.buffer.WriteString(logLine.Line)
			m.buffer.WriteString("\n")
		}
	}
	return nil
}

func (m *mockWriter) getContent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buffer.String()
}

func (m *mockWriter) getEvents() []*pb.LogEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	var events []*pb.LogEvent
	for _, msg := range m.messages {
		if msg.Event != nil {
			events = append(events, msg.Event)
		}
	}
	return events
}

func (m *mockWriter) hasEventType(t pb.LogEventType) bool {
	for _, e := range m.getEvents() {
		if e.Type == t {
			return true
		}
	}
	return false
}

// TestLogManager_NewLogManager verifies that a new LogManager can be created
// with a valid directory and initializes the logs directory structure correctly.
func TestLogManager_NewLogManager(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, lm)
		assert.DirExists(t, tmpDir)
	})
}

// TestLogManager_GetWriterAndSignalFinished tests that the LogManager correctly
// provides writers for rules and handles the finished signal to close log files properly.
func TestLogManager_GetWriterAndSignalFinished(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-1"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		writer, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		assert.NotNil(t, writer)

		_, err = writer.Write([]byte("line 1\n"))
		assert.NoError(t, err)
		_, err = writer.Write([]byte("line 2\n"))
		assert.NoError(t, err)

		lm.SignalFinished(ruleName, true, "")

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "line 1\nline 2\n", string(content))

		writer2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("new line 1\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		content, err = os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "new line 1\n", string(content))
	})
}

// TestLogManager_StreamLogs_Historical verifies that historical logs can be streamed
// correctly from existing log files for a specific rule.
func TestLogManager_StreamLogs_Historical(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-historical"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		initialContent := "historical line 1\nhistorical line 2\n"
		err = os.WriteFile(logFilePath, []byte(initialContent), 0644)
		assert.NoError(t, err)

		// Mark the rule as finished so it streams historical content
		lm.SignalFinished(ruleName, true, "")

		mockWriter := newMockWriter()
		writer := gocurrent.NewWriter(mockWriter.write)

		err = lm.StreamLogs(ruleName, "", 0, 0, writer) // 0 timeout for finished rules
		assert.NoError(t, err)

		writer.Stop()

		assert.Contains(t, mockWriter.getContent(), "historical line 1")
		assert.Contains(t, mockWriter.getContent(), "historical line 2")
		assert.True(t, mockWriter.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED))
	})
}

// TestLogManager_StreamLogs_Realtime tests that real-time log streaming works
// correctly, delivering new log lines as they are written to active rules.
func TestLogManager_StreamLogs_Realtime(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-realtime"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		mockWriter := newMockWriter()
		writer := gocurrent.NewWriter(mockWriter.write)

		go func() {
			defer wg.Done()
			err := lm.StreamLogs(ruleName, "", 5, 0, writer) // 5 second timeout for live logs
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = fileWriter.Write([]byte("realtime line 1\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		_, err = fileWriter.Write([]byte("realtime line 2\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		writer.Stop()

		assert.Contains(t, mockWriter.getContent(), "realtime line 1")
		assert.Contains(t, mockWriter.getContent(), "realtime line 2")
		assert.True(t, mockWriter.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED))
	})
}

// TestLogManager_StreamLogs_Blocking verifies that log streaming can be properly
// cancelled/interrupted without hanging the system or causing resource leaks.
func TestLogManager_StreamLogs_Blocking(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-blocking"
		streamErrChan := make(chan error, 1)

		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		mockWriter := newMockWriter()
		writer := gocurrent.NewWriter(mockWriter.write)

		go func() {
			streamErrChan <- lm.StreamLogs(ruleName, "", 5, 0, writer) // 5 second timeout
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = fileWriter.Write([]byte("blocked line\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		err = <-streamErrChan
		assert.NoError(t, err)
		writer.Stop()

		assert.Contains(t, mockWriter.getContent(), "blocked line")
		assert.True(t, mockWriter.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED))
	})
}

// TestLogManager_StreamLogs_Filtering tests that log streaming respects filter
// parameters to only return log lines containing the specified filter text.
func TestLogManager_StreamLogs_Filtering(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-filter"
		writer, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		contentToWrite := "line with filter keyword\nline without\nANOTHER LINE WITH FILTER KEYWORD\nfinal line\n"
		_, err = writer.Write([]byte(contentToWrite))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		mockWriter := newMockWriter()
		gocurrentWriter := gocurrent.NewWriter(mockWriter.write)

		err = lm.StreamLogs(ruleName, "filter keyword", 0, 0, gocurrentWriter) // 0 timeout for finished rules
		assert.NoError(t, err)

		gocurrentWriter.Stop()

		assert.Contains(t, mockWriter.getContent(), "line with filter keyword")
		assert.Contains(t, mockWriter.getContent(), "ANOTHER LINE WITH FILTER KEYWORD")
		assert.True(t, mockWriter.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED))
	})
}

// TestLogManager_Close verifies that the LogManager can be properly closed,
// releasing all resources and stopping all background operations cleanly.
func TestLogManager_Close(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		writer1, err := lm.GetWriter("rule-close-1", false)
		assert.NoError(t, err)
		_, err = writer1.Write([]byte("data"))
		assert.NoError(t, err)

		writer2, err := lm.GetWriter("rule-close-2", false)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("data"))
		assert.NoError(t, err)

		err = lm.Close()
		assert.NoError(t, err)

		// In the new simplified LogManager, Close() doesn't prevent further writes
		// since each GetWriter() returns a new file handle. This test just verifies
		// that Close() can be called without error.
		assert.NoError(t, err)
	})
}

// TestLogManager_GetWriter_AppendMode tests that GetWriter in append mode
// preserves previous content and writes a separator line.
func TestLogManager_GetWriter_AppendMode(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-append"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		// First run: truncate mode (default)
		writer1, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer1.Write([]byte("run1 line1\nrun1 line2\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "run1 line1\nrun1 line2\n", string(content))

		// Second run: append mode - previous content should be preserved
		writer2, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("run2 line1\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		content, err = os.ReadFile(logFilePath)
		assert.NoError(t, err)
		// Should contain run1 content + separator + run2 content
		assert.Contains(t, string(content), "run1 line1\nrun1 line2\n")
		assert.Contains(t, string(content), "--- [rule: test-rule-append] run started at")
		assert.Contains(t, string(content), "run2 line1\n")
	})
}

// TestLogManager_GetWriter_TruncateMode tests that GetWriter in truncate mode
// replaces previous content entirely.
func TestLogManager_GetWriter_TruncateMode(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-truncate"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		// First run
		writer1, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer1.Write([]byte("run1 output\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		// Second run: truncate mode - previous content should be gone
		writer2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("run2 output\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "run2 output\n", string(content))
		assert.NotContains(t, string(content), "run1")
	})
}

// TestLogManager_GetWriter_ClearsFinishedState tests that GetWriter clears
// the finishedRules state so subsequent StreamLogs calls use the live path.
func TestLogManager_GetWriter_ClearsFinishedState(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-finished-clear"

		// First run: write, finish, and verify StreamLogs returns completed content
		writer1, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer1.Write([]byte("run1 line\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		mock1 := newMockWriter()
		gw1 := gocurrent.NewWriter(mock1.write)
		err = lm.StreamLogs(ruleName, "", 0, 0, gw1)
		assert.NoError(t, err)
		gw1.Stop()
		assert.Contains(t, mock1.getContent(), "run1 line")
		assert.True(t, mock1.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED))

		// Second run: GetWriter should clear finished state
		writer2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		// Now stream in background - should use live path (not finished path)
		var wg sync.WaitGroup
		wg.Add(1)
		mock2 := newMockWriter()
		gw2 := gocurrent.NewWriter(mock2.write)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw2)
		}()

		time.Sleep(100 * time.Millisecond)
		_, err = writer2.Write([]byte("run2 live line\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw2.Stop()

		// Should have the live content from run2, not just a "completed" message
		assert.Contains(t, mock2.getContent(), "run2 live line")
	})
}

// TestLogManager_AppendMode_MultipleRuns tests that multiple append-mode runs
// accumulate content with separator lines between each run.
func TestLogManager_AppendMode_MultipleRuns(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-multi-append"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		// Run 1: truncate (initial run)
		w1, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = w1.Write([]byte("run1\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		// Run 2: append
		w2, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = w2.Write([]byte("run2\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		// Run 3: append
		w3, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = w3.Write([]byte("run3\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		contentStr := string(content)

		// All three runs should be present
		assert.Contains(t, contentStr, "run1\n")
		assert.Contains(t, contentStr, "run2\n")
		assert.Contains(t, contentStr, "run3\n")

		// Should have two separator lines (between run1->run2 and run2->run3)
		separatorCount := strings.Count(contentStr, "--- [rule: test-rule-multi-append] run started at")
		assert.Equal(t, 2, separatorCount)
	})
}

// TestLogManager_StreamLogs_MultipleClients verifies that two concurrent streams
// both receive the same log lines from a single rule.
func TestLogManager_StreamLogs_MultipleClients(t *testing.T) {
	testhelpers.WithTestContext(t, 3*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-multi-client"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		var wg sync.WaitGroup

		mock1 := newMockWriter()
		gw1 := gocurrent.NewWriter(mock1.write)
		mock2 := newMockWriter()
		gw2 := gocurrent.NewWriter(mock2.write)

		wg.Add(2)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw1)
		}()
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw2)
		}()

		time.Sleep(200 * time.Millisecond)

		_, err = fileWriter.Write([]byte("shared line 1\nshared line 2\n"))
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw1.Stop()
		gw2.Stop()

		assert.Contains(t, mock1.getContent(), "shared line 1")
		assert.Contains(t, mock1.getContent(), "shared line 2")
		assert.Contains(t, mock2.getContent(), "shared line 1")
		assert.Contains(t, mock2.getContent(), "shared line 2")
	})
}

// TestLogManager_StreamLogs_LastNLines verifies that history lines are sent before live lines.
func TestLogManager_StreamLogs_LastNLines(t *testing.T) {
	testhelpers.WithTestContext(t, 3*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-history"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		// Write some initial content
		_, err = fileWriter.Write([]byte("line1\nline2\nline3\nline4\nline5\n"))
		assert.NoError(t, err)

		// Give broadcaster time to ingest
		time.Sleep(300 * time.Millisecond)

		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 3, gw) // last 3 lines of history
		}()

		time.Sleep(200 * time.Millisecond)

		// Write a live line
		_, err = fileWriter.Write([]byte("live line\n"))
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw.Stop()

		content := mock.getContent()
		// Should contain history lines
		assert.Contains(t, content, "line3")
		assert.Contains(t, content, "line4")
		assert.Contains(t, content, "line5")
		// And live line
		assert.Contains(t, content, "live line")
	})
}

// TestLogManager_StreamLogs_RuleRestart_Truncate verifies that after a rule restarts
// with truncation, a connected client gets the new run's output.
func TestLogManager_StreamLogs_RuleRestart_Truncate(t *testing.T) {
	testhelpers.WithTestContext(t, 3*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-restart-trunc"

		// First run
		w1, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = w1.Write([]byte("run1 output\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		// Second run (truncate)
		w2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw)
		}()

		time.Sleep(200 * time.Millisecond)
		_, err = w2.Write([]byte("run2 output\n"))
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw.Stop()

		content := mock.getContent()
		assert.Contains(t, content, "run2 output")
	})
}

// TestLogManager_StreamLogs_RuleRestart_Append verifies that after a rule restarts
// in append mode, a subscriber sees new content without duplicated old content.
func TestLogManager_StreamLogs_RuleRestart_Append(t *testing.T) {
	testhelpers.WithTestContext(t, 3*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-restart-append"

		// First run (append mode)
		w1, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = w1.Write([]byte("run1 output\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName, true, "")

		// Second run (append mode) — file now has run1 content + separator
		w2, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw)
		}()

		time.Sleep(200 * time.Millisecond)
		_, err = w2.Write([]byte("run2 output\n"))
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw.Stop()

		content := mock.getContent()
		assert.Contains(t, content, "run2 output")
		// run1 output should appear at most once (from initial file read),
		// NOT duplicated by a source reset
		count := strings.Count(content, "run1 output")
		assert.LessOrEqual(t, count, 1, "run1 output should not be duplicated")
	})
}

// TestLogManager_Close_StopsBroadcasters verifies that Close stops all broadcasters.
func TestLogManager_Close_StopsBroadcasters(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-close-bc"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, _ = fileWriter.Write([]byte("data\n"))

		// Force broadcaster creation by calling StreamLogs in background
		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", -1, 0, gw) // infinite timeout
		}()

		time.Sleep(200 * time.Millisecond)

		// Close should stop the broadcaster and unblock StreamLogs
		err = lm.Close()
		assert.NoError(t, err)

		wg.Wait()
		gw.Stop()
	})
}

// TestLogManager_StreamLogs_CompletedEvent verifies that a RUN_COMPLETED event
// is sent when a rule finishes execution.
func TestLogManager_StreamLogs_CompletedEvent(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-event-completed"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw)
		}()

		time.Sleep(100 * time.Millisecond)
		_, _ = fileWriter.Write([]byte("output\n"))
		time.Sleep(100 * time.Millisecond)

		lm.SignalFinished(ruleName, true, "")
		wg.Wait()
		gw.Stop()

		events := mock.getEvents()
		assert.NotEmpty(t, events)
		lastEvent := events[len(events)-1]
		assert.Equal(t, pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED, lastEvent.Type)
		assert.Equal(t, ruleName, lastEvent.RuleName)
	})
}

// TestLogManager_StreamLogs_FailedEvent verifies that a RUN_FAILED event
// is sent when a rule finishes with an error.
func TestLogManager_StreamLogs_FailedEvent(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-event-failed"
		fileWriter, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.StreamLogs(ruleName, "", 5, 0, gw)
		}()

		time.Sleep(100 * time.Millisecond)
		_, _ = fileWriter.Write([]byte("output\n"))
		time.Sleep(100 * time.Millisecond)

		lm.SignalFinished(ruleName, false, "exit status 1")
		wg.Wait()
		gw.Stop()

		events := mock.getEvents()
		assert.NotEmpty(t, events)
		lastEvent := events[len(events)-1]
		assert.Equal(t, pb.LogEventType_LOG_EVENT_TYPE_RUN_FAILED, lastEvent.Type)
		assert.Equal(t, "exit status 1", lastEvent.Message)
	})
}

// TestLogManager_StreamLogs_TimeoutEvent verifies that a TIMEOUT event
// is sent when the stream times out.
func TestLogManager_StreamLogs_TimeoutEvent(t *testing.T) {
	testhelpers.WithTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-event-timeout"
		_, err = lm.GetWriter(ruleName, false)
		assert.NoError(t, err)

		mock := newMockWriter()
		gw := gocurrent.NewWriter(mock.write)

		err = lm.StreamLogs(ruleName, "", 1, 0, gw) // 1 second timeout
		assert.NoError(t, err)
		gw.Stop()

		assert.True(t, mock.hasEventType(pb.LogEventType_LOG_EVENT_TYPE_TIMEOUT))
	})
}
