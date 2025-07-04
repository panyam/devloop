// Package testhelpers provides common testing utilities for devloop.
//
// This package contains shared testing functionality used across devloop's test suite:
// - Test context management with automatic cleanup
// - Network port allocation for testing
// - Temporary directory management
// - Test timeout enforcement
//
// # Test Context
//
// WithTestContext provides a standardized way to run tests with temporary directories:
//
//	func TestMyFunction(t *testing.T) {
//		testhelpers.WithTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
//			// Test code here runs in a temporary directory
//			// Automatic cleanup happens when test completes
//		})
//	}
//
// # Port Allocation
//
// FindAvailablePort helps with network testing:
//
//	port, err := testhelpers.FindAvailablePort()
//	if err != nil {
//		t.Fatal(err)
//	}
//	// Use port for test server
//
// # Features
//
// - Automatic temporary directory creation and cleanup
// - Test timeout enforcement to prevent hanging tests
// - Dynamic port allocation for network tests
// - Consistent test environment setup
package testhelpers

import (
	"net"
	"os"
	"testing"
	"time"
)

// WithTestContext creates a temporary directory for a test, changes the CWD to it,
// and ensures everything is cleaned up afterward. It also enforces a timeout.
func WithTestContext(t *testing.T, timeout time.Duration, testFunc func(t *testing.T, tmpDir string)) {
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

// FindAvailablePort finds an available port for testing purposes.
// Returns a port number that is currently available for binding.
func FindAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
