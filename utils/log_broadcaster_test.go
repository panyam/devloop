package utils

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/testhelpers"
)

// ============================================================
// RingBuffer tests
// ============================================================

func TestRingBuffer_WriteAndReadLast(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]string{"a", "b", "c", "d", "e"})
	got := rb.ReadLast(3)
	assert.Equal(t, []string{"c", "d", "e"}, got)
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(200)
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = string(rune('A' + i%26))
	}
	rb.Write(lines)
	got := rb.ReadLast(200)
	assert.Len(t, got, 200)
	// Should be the last 200 entries
	assert.Equal(t, lines[100:], got)
}

func TestRingBuffer_ReadMoreThanAvailable(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]string{"a", "b", "c"})
	got := rb.ReadLast(10)
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestRingBuffer_ReadAll(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]string{"x", "y", "z"})
	got := rb.ReadLast(-1)
	assert.Equal(t, []string{"x", "y", "z"}, got)
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]string{"a", "b"})
	rb.Clear()
	assert.Equal(t, 0, rb.Len())
	assert.Nil(t, rb.ReadLast(-1))
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(10)
	assert.Nil(t, rb.ReadLast(5))
	assert.Nil(t, rb.ReadLast(-1))
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.Write([]string{"line"})
			}
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.ReadLast(10)
			}
		}()
	}

	wg.Wait()
	// No panic or data race = pass
}

// ============================================================
// FileLogSource tests
// ============================================================

func TestFileLogSource_ReadNewContent(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "test.log")
		err := os.WriteFile(path, []byte("line1\nline2\n"), 0644)
		require.NoError(t, err)

		src := NewFileLogSource(path)
		defer src.Close()

		lines, err := src.ReadLines(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"line1", "line2"}, lines)

		// Second read with no new content
		lines, err = src.ReadLines(context.Background())
		require.NoError(t, err)
		assert.Empty(t, lines)

		// Append more content
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString("line3\n")
		f.Close()

		lines, err = src.ReadLines(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"line3"}, lines)
	})
}

func TestFileLogSource_NoContent(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "empty.log")
		err := os.WriteFile(path, []byte{}, 0644)
		require.NoError(t, err)

		src := NewFileLogSource(path)
		defer src.Close()

		lines, err := src.ReadLines(context.Background())
		require.NoError(t, err)
		assert.Empty(t, lines)
	})
}

func TestFileLogSource_FileNotExists(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "missing.log")
		src := NewFileLogSource(path)
		defer src.Close()

		lines, err := src.ReadLines(context.Background())
		assert.NoError(t, err)
		assert.Nil(t, lines)
	})
}

func TestFileLogSource_Reset(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "reset.log")
		err := os.WriteFile(path, []byte("aaa\nbbb\n"), 0644)
		require.NoError(t, err)

		src := NewFileLogSource(path)
		defer src.Close()

		lines, _ := src.ReadLines(context.Background())
		assert.Equal(t, []string{"aaa", "bbb"}, lines)

		src.Reset()

		lines, _ = src.ReadLines(context.Background())
		assert.Equal(t, []string{"aaa", "bbb"}, lines)
	})
}

func TestFileLogSource_DetectsTruncation(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "trunc.log")
		err := os.WriteFile(path, []byte("long line content here\n"), 0644)
		require.NoError(t, err)

		src := NewFileLogSource(path)
		defer src.Close()

		lines, _ := src.ReadLines(context.Background())
		assert.Equal(t, []string{"long line content here"}, lines)

		// Truncate and write shorter content
		err = os.WriteFile(path, []byte("short\n"), 0644)
		require.NoError(t, err)

		lines, err = src.ReadLines(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"short"}, lines)
	})
}

func TestFileLogSource_Close(t *testing.T) {
	testhelpers.WithTestContext(t, 1*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "close.log")
		err := os.WriteFile(path, []byte("data\n"), 0644)
		require.NoError(t, err)

		src := NewFileLogSource(path)
		src.ReadLines(context.Background())
		err = src.Close()
		assert.NoError(t, err)

		// Double close should not error
		err = src.Close()
		assert.NoError(t, err)
	})
}

// ============================================================
// LogBroadcaster tests
// ============================================================

func TestBroadcaster_SingleSubscriber(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		sub := b.Subscribe("client-1", "")

		// Write content
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString("hello\nworld\n")
		f.Close()

		got := collectLines(t, sub, 2, 2*time.Second)
		assert.Len(t, got, 2)
		assert.Equal(t, "hello", got[0].Line)
		assert.Equal(t, "world", got[1].Line)
	})
}

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		sub1 := b.Subscribe("client-1", "")
		sub2 := b.Subscribe("client-2", "")

		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString("broadcast\n")
		f.Close()

		got1 := collectLines(t, sub1, 1, 2*time.Second)
		got2 := collectLines(t, sub2, 1, 2*time.Second)
		assert.Equal(t, "broadcast", got1[0].Line)
		assert.Equal(t, "broadcast", got2[0].Line)
	})
}

func TestBroadcaster_SubscriberWithFilter(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		sub := b.Subscribe("client-1", "error")

		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString("info: all good\nerror: something broke\nwarn: maybe\n")
		f.Close()

		got := collectLines(t, sub, 1, 2*time.Second)
		assert.Len(t, got, 1)
		assert.Equal(t, "error: something broke", got[0].Line)
	})
}

func TestBroadcaster_Unsubscribe(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		sub := b.Subscribe("client-1", "")
		b.Unsubscribe("client-1")

		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString("after-unsub\n")
		f.Close()

		// Give poller time to read
		time.Sleep(300 * time.Millisecond)

		// Channel should be empty (no delivery after unsubscribe)
		select {
		case <-sub.Ch:
			t.Fatal("should not receive after unsubscribe")
		default:
		}
	})
}

func TestBroadcaster_History(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		// Wait for poller to read the initial content
		time.Sleep(300 * time.Millisecond)

		history := b.GetHistory(3)
		assert.Equal(t, []string{"c", "d", "e"}, history)

		allHistory := b.GetHistory(-1)
		assert.Equal(t, []string{"a", "b", "c", "d", "e"}, allHistory)
	})
}

func TestBroadcaster_SignalFinished(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte("line\n"), 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)

		sub := b.Subscribe("client-1", "")

		// Wait for poller to read
		time.Sleep(300 * time.Millisecond)

		b.SignalFinished()
		assert.True(t, b.IsFinished())

		// Done channel should close
		select {
		case <-sub.Done:
			// expected
		case <-time.After(2 * time.Second):
			t.Fatal("Done channel should have closed")
		}
	})
}

func TestBroadcaster_SignalNewRun_Truncate(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte("old\n"), 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		time.Sleep(300 * time.Millisecond)
		assert.Equal(t, 1, b.ringBuffer.Len())

		b.SignalNewRun(false)

		assert.False(t, b.IsFinished())
		assert.Equal(t, 0, b.ringBuffer.Len())
	})
}

func TestBroadcaster_SignalNewRun_Append(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte("line1\nline2\n"), 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		// Wait for poller to read initial content
		time.Sleep(300 * time.Millisecond)
		assert.Equal(t, 2, b.ringBuffer.Len())

		b.SignalFinished()
		time.Sleep(50 * time.Millisecond)

		b.SignalNewRun(true)

		// Ring buffer should be preserved (not cleared)
		assert.Equal(t, 2, b.ringBuffer.Len())
		assert.Equal(t, []string{"line1", "line2"}, b.GetHistory(-1))
		// Finished flag should be cleared
		assert.False(t, b.IsFinished())
	})
}

func TestBroadcaster_Stop(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)

		sub := b.Subscribe("client-1", "")
		b.Stop()

		select {
		case <-sub.Done:
			// expected
		case <-time.After(1 * time.Second):
			t.Fatal("Done channel should have closed after Stop")
		}
	})
}

func TestBroadcaster_SlowSubscriber(t *testing.T) {
	testhelpers.WithTestContext(t, 3*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		// Subscribe but never read — simulates a slow consumer
		_ = b.Subscribe("slow-client", "")
		// Also subscribe a fast client
		fast := b.Subscribe("fast-client", "")

		// Write enough to overflow the slow subscriber's channel buffer
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		for i := 0; i < 200; i++ {
			f.WriteString("line\n")
		}
		f.Close()

		// Fast client should still receive data without deadlock
		got := collectLines(t, fast, 1, 2*time.Second)
		assert.NotEmpty(t, got)
	})
}

func TestBroadcaster_BroadcastEvent(t *testing.T) {
	testhelpers.WithTestContext(t, 2*time.Second, func(t *testing.T, tmpDir string) {
		path := filepath.Join(tmpDir, "rule.log")
		os.WriteFile(path, []byte{}, 0644)

		src := NewFileLogSource(path)
		b := NewLogBroadcaster("test-rule", src)
		defer b.Stop()

		sub1 := b.Subscribe("client-1", "")
		sub2 := b.Subscribe("client-2", "")

		event := &pb.LogEvent{
			RuleName:  "test-rule",
			Type:      pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED,
			Timestamp: time.Now().UnixMilli(),
			Message:   "done",
		}
		b.BroadcastEvent(event)

		// Both subscribers should receive the event
		select {
		case got := <-sub1.Events:
			assert.Equal(t, pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED, got.Type)
			assert.Equal(t, "done", got.Message)
		case <-time.After(1 * time.Second):
			t.Fatal("sub1 did not receive event")
		}

		select {
		case got := <-sub2.Events:
			assert.Equal(t, pb.LogEventType_LOG_EVENT_TYPE_RUN_COMPLETED, got.Type)
		case <-time.After(1 * time.Second):
			t.Fatal("sub2 did not receive event")
		}
	})
}

// collectLines reads from a subscriber until it has at least `want` lines or times out.
func collectLines(t *testing.T, sub *Subscriber, want int, timeout time.Duration) []*pb.LogLine {
	t.Helper()
	var all []*pb.LogLine
	deadline := time.After(timeout)
	for {
		select {
		case batch := <-sub.Ch:
			all = append(all, batch...)
			if len(all) >= want {
				return all
			}
		case <-sub.Done:
			return all
		case <-deadline:
			t.Fatalf("timed out waiting for %d lines, got %d", want, len(all))
			return all
		}
	}
}
