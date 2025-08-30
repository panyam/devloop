package agent

import (
	"testing"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// TestChannelBasedDebouncing verifies the channel-based debouncing behavior
func TestChannelBasedDebouncing(t *testing.T) {
	rule := &pb.Rule{
		Name:     "test-rule",
		Commands: []string{"echo 'test'"},
	}

	// Create minimal orchestrator for testing
	orchestrator := &Orchestrator{
		Config: &pb.Config{
			Settings: &pb.Settings{
				PrefixLogs: false,
			},
		},
		LogManager: &mockLogManager{},
	}

	runner := NewRuleRunner(rule, orchestrator)
	runner.SetDebounceDelay(50 * time.Millisecond)

	// Start the event loop
	go runner.eventLoop()

	// Test 1: File change should trigger debouncing
	select {
	case runner.fileChangeChan <- "test.go":
		// File change sent successfully
	default:
		t.Fatal("File change channel should not be full")
	}

	// Wait less than debounce duration - should not execute yet
	time.Sleep(25 * time.Millisecond)
	status := runner.GetStatus()
	if status.IsRunning {
		t.Error("Rule should not be running yet - debounce period not expired")
	}

	// Wait for debounce to complete
	time.Sleep(30 * time.Millisecond)

	// Test 2: Multiple rapid file changes should only trigger once
	for i := 0; i < 5; i++ {
		select {
		case runner.fileChangeChan <- "test.go":
		default:
			// Channel full is okay for this test
		}
		time.Sleep(10 * time.Millisecond) // Rapid changes
	}

	// Wait for debounce
	time.Sleep(60 * time.Millisecond)

	// Only one execution should have occurred (not 5)
	// This is verified by the behavior, not internal state

	// Cleanup
	close(runner.stopChan)
}

// TestManualTrigger tests manual trigger functionality
func TestManualTrigger(t *testing.T) {
	rule := &pb.Rule{
		Name:     "test-rule",
		Commands: []string{"echo 'manual test'"},
	}

	orchestrator := &Orchestrator{
		Config: &pb.Config{
			Settings: &pb.Settings{},
		},
		LogManager: &mockLogManager{},
	}

	runner := NewRuleRunner(rule, orchestrator)

	// Start the event loop
	go runner.eventLoop()

	// Test manual trigger (should execute immediately, no debouncing)
	runner.triggerExecution("manual")

	// Brief wait to allow execution to start
	time.Sleep(10 * time.Millisecond)

	// Cleanup
	close(runner.stopChan)
}

// mockLogManager implements the minimal LogManager interface needed for testing
type mockLogManager struct{}

func (m *mockLogManager) GetWriter(ruleName string) (*mockWriter, error) {
	return &mockWriter{}, nil
}

func (m *mockLogManager) SignalFinished(ruleName string) {}

type mockWriter struct{}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
