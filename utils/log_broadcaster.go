package utils

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

const (
	defaultRingBufferCap  = 1000
	subscriberChanBufSize = 64
	pollInterval          = 100 * time.Millisecond
)

// Subscriber represents a client subscribed to log broadcasts.
type Subscriber struct {
	ID     string
	Filter string
	Ch     chan []*pb.LogLine // buffered, batched delivery
	Done   chan struct{}      // closed when broadcaster stops or rule finishes
}

// LogBroadcaster reads new lines from a LogSource once and fans out to all subscribers.
// One broadcaster per rule eliminates redundant file polling.
type LogBroadcaster struct {
	ruleName    string
	source      LogSource
	ringBuffer  *RingBuffer
	subscribers map[string]*Subscriber
	subMu       sync.RWMutex
	finished    atomic.Bool
	ctx         context.Context
	cancel      context.CancelFunc
	stopped     chan struct{} // closed when poller goroutine exits
}

// NewLogBroadcaster creates and starts a broadcaster for the given rule.
func NewLogBroadcaster(ruleName string, source LogSource) *LogBroadcaster {
	ctx, cancel := context.WithCancel(context.Background())
	b := &LogBroadcaster{
		ruleName:    ruleName,
		source:      source,
		ringBuffer:  NewRingBuffer(defaultRingBufferCap),
		subscribers: make(map[string]*Subscriber),
		ctx:         ctx,
		cancel:      cancel,
		stopped:     make(chan struct{}),
	}
	go b.pollLoop()
	return b
}

// Subscribe adds a subscriber that will receive new log lines.
// Returns the subscriber. The caller should select on sub.Ch and sub.Done.
func (b *LogBroadcaster) Subscribe(id, filter string) *Subscriber {
	sub := &Subscriber{
		ID:     id,
		Filter: filter,
		Ch:     make(chan []*pb.LogLine, subscriberChanBufSize),
		Done:   make(chan struct{}),
	}
	b.subMu.Lock()
	b.subscribers[id] = sub
	b.subMu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber by ID.
func (b *LogBroadcaster) Unsubscribe(id string) {
	b.subMu.Lock()
	delete(b.subscribers, id)
	b.subMu.Unlock()
}

// GetHistory returns the last n lines from the ring buffer.
func (b *LogBroadcaster) GetHistory(n int) []string {
	return b.ringBuffer.ReadLast(n)
}

// IsFinished returns whether the rule has finished.
func (b *LogBroadcaster) IsFinished() bool {
	return b.finished.Load()
}

// SignalFinished marks the rule as finished. The poller will do a final drain
// and then close all subscriber Done channels.
func (b *LogBroadcaster) SignalFinished() {
	b.finished.Store(true)
}

// SignalNewRun prepares the broadcaster for a new rule run.
// In truncate mode (appendMode=false): clears the ring buffer, resets the source
// to offset 0, and clears the finished flag.
// In append mode (appendMode=true): only clears the finished flag so the source
// continues from its current offset (picking up the separator + new output) and
// the ring buffer preserves history across runs.
func (b *LogBroadcaster) SignalNewRun(appendMode bool) {
	if !appendMode {
		b.ringBuffer.Clear()
		b.source.Reset()
	}
	b.finished.Store(false)
}

// Stop cancels the poller goroutine, closes all subscriber Done channels, and closes the source.
func (b *LogBroadcaster) Stop() {
	b.cancel()
	<-b.stopped // wait for poller to exit
	b.source.Close()
}

func (b *LogBroadcaster) pollLoop() {
	defer close(b.stopped)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.closeAllSubscribers()
			return
		case <-ticker.C:
			lines, err := b.source.ReadLines(b.ctx)
			if err != nil {
				LogDevloop("LogBroadcaster[%s]: error reading lines: %v", b.ruleName, err)
			}

			if len(lines) > 0 {
				b.ringBuffer.Write(lines)
				b.fanOut(lines)
			}

			if b.finished.Load() {
				// Final drain: read any remaining lines
				for {
					remaining, err := b.source.ReadLines(b.ctx)
					if err != nil || len(remaining) == 0 {
						break
					}
					b.ringBuffer.Write(remaining)
					b.fanOut(remaining)
				}
				b.closeAllSubscribers()
				return
			}
		}
	}
}

func (b *LogBroadcaster) fanOut(lines []string) {
	b.subMu.RLock()
	defer b.subMu.RUnlock()

	now := time.Now().UnixMilli()
	for _, sub := range b.subscribers {
		var filtered []*pb.LogLine
		for _, line := range lines {
			if sub.Filter == "" || containsInsensitive(line, sub.Filter) {
				filtered = append(filtered, &pb.LogLine{
					RuleName:  b.ruleName,
					Line:      line,
					Timestamp: now,
				})
			}
		}
		if len(filtered) == 0 {
			continue
		}
		// Non-blocking send — drop if subscriber is slow
		select {
		case sub.Ch <- filtered:
		default:
		}
	}
}

func (b *LogBroadcaster) closeAllSubscribers() {
	b.subMu.RLock()
	defer b.subMu.RUnlock()
	for _, sub := range b.subscribers {
		select {
		case <-sub.Done:
			// already closed
		default:
			close(sub.Done)
		}
	}
}

func containsInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
