package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"

	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
)

// mockStream is a mock implementation of the GatewayClientService_StreamLogsClientServer interface.
type mockStream struct {
	ctx    context.Context
	buffer *bytes.Buffer
}

func (m *mockStream) Send(logLine *pb.LogLine) error {
	_, err := m.buffer.WriteString(logLine.Line)
	return err
}

func (m *mockStream) SetHeader(md metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(md metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(md metadata.MD)       {}
func (m *mockStream) Context() context.Context        { return m.ctx }
func (m *mockStream) SendMsg(v interface{}) error     { return nil }
func (m *mockStream) RecvMsg(v interface{}) error     { return nil }

func newMockStream(ctx context.Context) *mockStream {
	return &mockStream{
		ctx:    ctx,
		buffer: &bytes.Buffer{},
	}
}

func TestLogManager_NewLogManager(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, lm)
		assert.DirExists(t, tmpDir)
	})
}

func TestLogManager_GetWriterAndSignalFinished(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
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

func TestLogManager_StreamLogs_Historical(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-historical"
		logFilePath := filepath.Join(tmpDir, fmt.Sprintf("%s.log", ruleName))

		initialContent := "historical line 1\nhistorical line 2\n"
		err = os.WriteFile(logFilePath, []byte(initialContent), 0644)
		assert.NoError(t, err)

		lm.mu.Lock()
		state := &ruleLogState{
			started:  make(chan struct{}),
			finished: make(chan struct{}),
		}
		lm.ruleStates[ruleName] = state
		close(state.started)
		close(state.finished)
		lm.mu.Unlock()

		stream := newMockStream(context.Background())
		err = lm.StreamLogs(ruleName, "", stream)
		assert.NoError(t, err)
		assert.Equal(t, initialContent, stream.buffer.String())
	})
}

func TestLogManager_StreamLogs_Realtime(t *testing.T) {
	withTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-realtime"
		writer, err := lm.GetWriter(ruleName)
		assert.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		stream := newMockStream(context.Background())
		go func() {
			defer wg.Done()
			err := lm.StreamLogs(ruleName, "", stream)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = writer.Write([]byte("realtime line 1\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		_, err = writer.Write([]byte("realtime line 2\n"))
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)

		lm.SignalFinished(ruleName)
		wg.Wait()

		assert.Equal(t, "realtime line 1\nrealtime line 2\n", stream.buffer.String())
	})
}

func TestLogManager_StreamLogs_Blocking(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-blocking"
		streamErrChan := make(chan error, 1)

		writer, err := lm.GetWriter(ruleName)
		assert.NoError(t, err)

		stream := newMockStream(context.Background())
		go func() {
			streamErrChan <- lm.StreamLogs(ruleName, "", stream)
		}()

		time.Sleep(100 * time.Millisecond)

		_, err = writer.Write([]byte("blocked line\n"))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

		err = <-streamErrChan
		assert.NoError(t, err)
		assert.Equal(t, "blocked line\n", stream.buffer.String())
	})
}

func TestLogManager_StreamLogs_Filtering(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		lm, err := NewLogManager(tmpDir)
		assert.NoError(t, err)

		ruleName := "test-rule-filter"
		writer, err := lm.GetWriter(ruleName)
		assert.NoError(t, err)

		contentToWrite := "line with filter keyword\nline without\nANOTHER LINE WITH FILTER KEYWORD\nfinal line\n"
		_, err = writer.Write([]byte(contentToWrite))
		assert.NoError(t, err)
		lm.SignalFinished(ruleName)

		stream := newMockStream(context.Background())
		err = lm.StreamLogs(ruleName, "filter keyword", stream)
		assert.NoError(t, err)

		expected := "line with filter keyword\nANOTHER LINE WITH FILTER KEYWORD\n"
		assert.Equal(t, expected, stream.buffer.String())
	})
}

func TestLogManager_Close(t *testing.T) {
	withTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
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

		_, err = writer1.Write([]byte("more data"))
		assert.Error(t, err)
		_, err = writer2.Write([]byte("more data"))
		assert.Error(t, err)
	})
}
