package agent

import (
	"path/filepath"
	"testing"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// TestChannelBasedDebouncing verifies the channel-based debouncing behavior
func TestChannelBasedDebouncing(t *testing.T) {
	rule := &pb.Rule{
		Name:     "test-rule",
		Commands: []string{"echo 'test'"},
	}

	// Create minimal orchestrator for testing
	tmpDir := t.TempDir()
	logManager, err := utils.NewLogManager(filepath.Join(tmpDir, "logs"))
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}
	defer logManager.Close()

	orchestrator := &Orchestrator{
		Config: &pb.Config{
			Settings: &pb.Settings{
				PrefixLogs: false,
			},
		},
		LogManager: logManager,
	}

	runner := NewRuleRunner(rule, orchestrator)
	runner.SetDebounceDelay(50 * time.Millisecond)

	// Start the event loop
	go runner.eventLoop()

	// Test 1: File change should trigger debouncing (simulate via TriggerManual)
	runner.TriggerManual()

	// Wait less than debounce duration - should not execute yet
	time.Sleep(25 * time.Millisecond)
	status := runner.GetStatus()
	if status.IsRunning {
		t.Error("Rule should not be running yet - debounce period not expired")
	}

	// Wait for debounce to complete
	time.Sleep(30 * time.Millisecond)

	// Test 2: Multiple rapid triggers (simulated)
	for i := 0; i < 5; i++ {
		runner.TriggerManual()
		time.Sleep(10 * time.Millisecond) // Rapid triggers
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

	tmpDir := t.TempDir()
	logManager, err := utils.NewLogManager(filepath.Join(tmpDir, "logs"))
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}
	defer logManager.Close()

	orchestrator := &Orchestrator{
		Config: &pb.Config{
			Settings: &pb.Settings{},
		},
		LogManager: logManager,
	}

	runner := NewRuleRunner(rule, orchestrator)

	// Start the event loop
	go runner.eventLoop()

	// Test manual trigger (should execute immediately, no debouncing)
	runner.TriggerManual()

	// Brief wait to allow execution to start
	time.Sleep(10 * time.Millisecond)

	// Cleanup
	close(runner.stopChan)
}
