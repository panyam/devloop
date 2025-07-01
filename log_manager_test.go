package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogManager_NewLogManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)
	assert.NotNil(t, lm)
	assert.DirExists(t, tmpDir)
}

func TestLogManager_GetWriterAndSignalFinished(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	ruleName := "test-rule-1"
	logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

	// Get writer and write some data
	writer, err := lm.GetWriter(ruleName)
	assert.NoError(t, err)
	assert.NotNil(t, writer)

	_, err = writer.Write([]byte("line 1\n"))
	assert.NoError(t, err)
	_, err = writer.Write([]byte("line 2\n"))
	assert.NoError(t, err)

	// Signal finished
	lm.SignalFinished(ruleName)

	// Verify file content
	content, err := os.ReadFile(logFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "line 1\nline 2\n", string(content))

	// Get writer again for a new run, should truncate
	writer2, err := lm.GetWriter(ruleName)
	assert.NoError(t, err)
	_, err = writer2.Write([]byte("new line 1\n"))
	assert.NoError(t, err)
	lm.SignalFinished(ruleName)

	content, err = os.ReadFile(logFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "new line 1\n", string(content))
}

func TestLogManager_StreamLogs_Historical(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	ruleName := "test-rule-historical"
	logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

	// Write some initial content to the log file
	initialContent := "historical line 1\nhistorical line 2\n"
	err = os.WriteFile(logFilePath, []byte(initialContent), 0644)
	assert.NoError(t, err)

	// Simulate a finished state for the rule
	lm.mu.Lock()
	state := &ruleLogState{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	lm.ruleStates[ruleName] = state
	close(state.started)  // Mark as started (but no active writer)
	close(state.finished) // Mark as finished
	lm.mu.Unlock()

	var buf bytes.Buffer
	err = lm.StreamLogs(ruleName, "", &buf)
	assert.NoError(t, err)
	assert.Equal(t, initialContent, buf.String())
}

func TestLogManager_StreamLogs_Realtime(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	ruleName := "test-rule-realtime"
	// logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

	var wg sync.WaitGroup
	wg.Add(1)

	// Goroutine to stream logs
	var streamedContent bytes.Buffer
	go func() {
		defer wg.Done()
		err := lm.StreamLogs(ruleName, "", &streamedContent)
		assert.NoError(t, err)
	}()

	// Give streamer time to start waiting
	time.Sleep(100 * time.Millisecond)

	// Get writer and write data
	writer, err := lm.GetWriter(ruleName)
	assert.NoError(t, err)

	_, err = writer.Write([]byte("realtime line 1\n"))
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond) // Give time for write to be processed

	_, err = writer.Write([]byte("realtime line 2\n"))
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Signal finished
	lm.SignalFinished(ruleName)
	wg.Wait()

	assert.Equal(t, "realtime line 1\nrealtime line 2\n", streamedContent.String())
}

func TestLogManager_StreamLogs_Blocking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	ruleName := "test-rule-blocking"
	var buf bytes.Buffer
	streamErrChan := make(chan error, 1)

	// Start streaming in a goroutine BEFORE getting the writer
	go func() {
		streamErrChan <- lm.StreamLogs(ruleName, "", &buf)
	}()

	// Give streamer time to start blocking
	time.Sleep(100 * time.Millisecond)

	// Get writer - this should unblock the streamer
	writer, err := lm.GetWriter(ruleName)
	assert.NoError(t, err)
	_, err = writer.Write([]byte("blocked line\n"))
	assert.NoError(t, err)
	lm.SignalFinished(ruleName)

	// Wait for streaming to finish
	assert.NoError(t, <-streamErrChan)
	assert.Equal(t, "blocked line\n", buf.String())
}

func TestLogManager_StreamLogs_Filtering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	ruleName := "test-rule-filter"

	// Write some content with and without the filter string
	contentToWrite := "line with filter keyword\nline without\nANOTHER LINE WITH FILTER KEYWORD\nfinal line\n"
	err = os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName)), []byte(contentToWrite), 0644)
	assert.NoError(t, err)

	// Simulate a finished state for the rule
	lm.mu.Lock()
	state := &ruleLogState{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	lm.ruleStates[ruleName] = state
	close(state.started)
	close(state.finished)
	lm.mu.Unlock()

	var buf bytes.Buffer
	err = lm.StreamLogs(ruleName, "filter keyword", &buf)
	assert.NoError(t, err)

	expected := "line with filter keyword\nANOTHER LINE WITH FILTER KEYWORD\n"
	assert.Equal(t, expected, buf.String())
}

func TestLogManager_Close(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_manager_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	lm, err := NewLogManager(tmpDir)
	assert.NoError(t, err)

	// Get writers for a few rules
	writer1, err := lm.GetWriter("rule-close-1")
	assert.NoError(t, err)
	_, err = writer1.Write([]byte("data"))
	assert.NoError(t, err)

	writer2, err := lm.GetWriter("rule-close-2")
	assert.NoError(t, err)
	_, err = writer2.Write([]byte("data"))
	assert.NoError(t, err)

	// Close the log manager
	err = lm.Close()
	assert.NoError(t, err)

	// Verify that files are closed (attempting to write should fail)
	_, err = writer1.Write([]byte("more data"))
	assert.Error(t, err) // Should be an error because the file is closed
	_, err = writer2.Write([]byte("more data"))
	assert.Error(t, err) // Should be an error because the file is closed
}
