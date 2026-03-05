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
			// Add newline to match expected test format (since LogManager strips them)
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

		lm.SignalFinished(ruleName)

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "line 1\nline 2\n", string(content))

		writer2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("new line 1\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

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
		lm.SignalFinished(ruleName)

		mockWriter := newMockWriter()
		writer := gocurrent.NewWriter(mockWriter.write)

		err = lm.StreamLogs(ruleName, "", 0, writer) // 0 timeout for finished rules
		assert.NoError(t, err)

		writer.Stop()

		expected := "historical line 1\nhistorical line 2\nRule 'test-rule-historical' execution completed\n"
		assert.Equal(t, expected, mockWriter.getContent())
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
			err := lm.StreamLogs(ruleName, "", 5, writer) // 5 second timeout for live logs
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = fileWriter.Write([]byte("realtime line 1\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		_, err = fileWriter.Write([]byte("realtime line 2\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		lm.SignalFinished(ruleName)
		wg.Wait()
		writer.Stop()

		expected := "realtime line 1\nrealtime line 2\nRule 'test-rule-realtime' execution completed\n"
		assert.Equal(t, expected, mockWriter.getContent())
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
			streamErrChan <- lm.StreamLogs(ruleName, "", 5, writer) // 5 second timeout
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = fileWriter.Write([]byte("blocked line\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

		err = <-streamErrChan
		assert.NoError(t, err)
		writer.Stop()

		expected := "blocked line\nRule 'test-rule-blocking' execution completed\n"
		assert.Equal(t, expected, mockWriter.getContent())
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
		lm.SignalFinished(ruleName)

		mockWriter := newMockWriter()
		gocurrentWriter := gocurrent.NewWriter(mockWriter.write)

		err = lm.StreamLogs(ruleName, "filter keyword", 0, gocurrentWriter) // 0 timeout for finished rules
		assert.NoError(t, err)

		gocurrentWriter.Stop()

		expected := "line with filter keyword\nANOTHER LINE WITH FILTER KEYWORD\nRule 'test-rule-filter' execution completed\n"
		assert.Equal(t, expected, mockWriter.getContent())
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
		lm.SignalFinished(ruleName)

		content, err := os.ReadFile(logFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "run1 line1\nrun1 line2\n", string(content))

		// Second run: append mode - previous content should be preserved
		writer2, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("run2 line1\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

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
		lm.SignalFinished(ruleName)

		// Second run: truncate mode - previous content should be gone
		writer2, err := lm.GetWriter(ruleName, false)
		assert.NoError(t, err)
		_, err = writer2.Write([]byte("run2 output\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

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
		lm.SignalFinished(ruleName)

		mock1 := newMockWriter()
		gw1 := gocurrent.NewWriter(mock1.write)
		err = lm.StreamLogs(ruleName, "", 0, gw1)
		assert.NoError(t, err)
		gw1.Stop()
		assert.Contains(t, mock1.getContent(), "run1 line")
		assert.Contains(t, mock1.getContent(), "execution completed")

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
			lm.StreamLogs(ruleName, "", 5, gw2)
		}()

		time.Sleep(100 * time.Millisecond)
		_, err = writer2.Write([]byte("run2 live line\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		lm.SignalFinished(ruleName)
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
		lm.SignalFinished(ruleName)

		// Run 2: append
		w2, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = w2.Write([]byte("run2\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

		// Run 3: append
		w3, err := lm.GetWriter(ruleName, true)
		assert.NoError(t, err)
		_, err = w3.Write([]byte("run3\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

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
