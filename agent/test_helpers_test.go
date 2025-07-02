package agent

import (
	"os"
	"testing"
	"time"
)

// withTestContext creates a temporary directory for a test, changes the CWD to it,
// and ensures everything is cleaned up afterward. It also enforces a timeout.
func withTestContext(t *testing.T, timeout time.Duration, testFunc func(t *testing.T, tmpDir string)) {
	t.Helper()

	// Create a temporary directory for the test.
	tmpDir, err := os.MkdirTemp("", "devloop_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a channel to signal when the test function is done.
	done := make(chan struct{})

	// Run the actual test logic in a goroutine.
	go func() {
		defer close(done)
		testFunc(t, tmpDir)
	}()

	if timeout <= 0 {
		timeout = 1000 * time.Second
	}

	// Wait for the test to finish or for the timeout to expire.
	select {
	case <-done:
		// Test completed within the timeout.
	case <-time.After(timeout):
		t.Fatalf("Test timed out after %v", timeout)
	}
}
