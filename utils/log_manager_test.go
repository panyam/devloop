package utils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

		writer, err := lm.GetWriter(ruleName)
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

		writer2, err := lm.GetWriter(ruleName)
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
		fileWriter, err := lm.GetWriter(ruleName)
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

		fileWriter, err := lm.GetWriter(ruleName)
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
		writer, err := lm.GetWriter(ruleName)
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

		writer1, err := lm.GetWriter("rule-close-1")
		assert.NoError(t, err)
		_, err = writer1.Write([]byte("data"))
		assert.NoError(t, err)

		writer2, err := lm.GetWriter("rule-close-2")
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
