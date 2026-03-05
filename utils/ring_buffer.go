package utils

import "sync"

// RingBuffer is a thread-safe circular buffer of strings.
// We use a simple slice-based approach instead of container/ring because:
//   - container/ring is a linked list without fixed-capacity overwrite semantics
//   - No built-in "read last N" — would require O(n) linked-list traversal
//   - Per-element heap allocations vs a single contiguous slice here
//   - Uses any/interface{} — loses type safety
//
// TODO: If we need a generic version, consider making this RingBuffer[T any].
type RingBuffer struct {
	buf   []string
	head  int // next write position
	count int
	cap   int
	mu    sync.Mutex
}

// NewRingBuffer creates a new RingBuffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]string, capacity),
		cap: capacity,
	}
}

// Write appends lines to the ring buffer, overwriting oldest entries if full.
func (rb *RingBuffer) Write(lines []string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, line := range lines {
		rb.buf[rb.head] = line
		rb.head = (rb.head + 1) % rb.cap
		if rb.count < rb.cap {
			rb.count++
		}
	}
}

// ReadLast returns the last n lines from the buffer.
// If n < 0, returns all available lines.
// If n > available, returns all available lines.
func (rb *RingBuffer) ReadLast(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	if n < 0 || n > rb.count {
		n = rb.count
	}

	result := make([]string, n)
	// Start position: head - count gives the oldest entry
	// We want the last n entries, so start at head - n
	start := (rb.head - n + rb.cap) % rb.cap
	for i := 0; i < n; i++ {
		result[i] = rb.buf[(start+i)%rb.cap]
	}
	return result
}

// Clear resets the buffer to empty.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.count = 0
}

// Len returns the number of lines currently in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
