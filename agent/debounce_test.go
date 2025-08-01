package agent

import (
	"testing"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// TestEnhancedDebouncingLogic verifies the enhanced debouncing logic behavior
func TestEnhancedDebouncingLogic(t *testing.T) {
	rule := &pb.Rule{Name: "test-rule"}
	orchestrator := &Orchestrator{Config: &pb.Config{Settings: &pb.Settings{}}}
	runner := NewRuleRunner(rule, orchestrator)
	runner.SetDebounceDelay(50 * time.Millisecond)

	// Test 1: When rule is not running, should use normal debouncing
	runner.TriggerDebounced()

	// Should have a debounce timer set
	runner.debounceMutex.Lock()
	hasTimer := runner.debounceTimer != nil
	runner.debounceMutex.Unlock()

	if !hasTimer {
		t.Error("Expected debounce timer to be set when rule is not running")
	}

	// Stop the timer to prevent execution
	runner.debounceMutex.Lock()
	if runner.debounceTimer != nil {
		runner.debounceTimer.Stop()
		runner.debounceTimer = nil
	}
	runner.debounceMutex.Unlock()

	// Test 2: When rule is running, should set pending execution instead of timer
	// Simulate running state
	runner.updateStatus(true, "RUNNING")

	// Trigger while running
	runner.TriggerDebounced()

	// Should have pending execution set, but no timer
	if !runner.hasPendingExecution() {
		t.Error("Expected pending execution to be set when rule is running")
	}

	runner.debounceMutex.Lock()
	hasTimer = runner.debounceTimer != nil
	runner.debounceMutex.Unlock()

	if hasTimer {
		t.Error("Expected no debounce timer when rule is running")
	}

	// Test 3: Multiple triggers while running should replace pending execution
	runner.TriggerDebounced() // Should still have pending execution
	runner.TriggerDebounced() // Should still have pending execution

	if !runner.hasPendingExecution() {
		t.Error("Expected pending execution to remain set after multiple triggers")
	}
}

// TestPendingExecutionHelpers tests the pending execution helper methods
func TestPendingExecutionHelpers(t *testing.T) {
	rule := &pb.Rule{Name: "test"}
	orchestrator := &Orchestrator{Config: &pb.Config{Settings: &pb.Settings{}}}
	runner := NewRuleRunner(rule, orchestrator)

	// Initially no pending execution
	if runner.hasPendingExecution() {
		t.Error("Expected no pending execution initially")
	}

	// Set pending execution
	runner.setPendingExecution(true)
	if !runner.hasPendingExecution() {
		t.Error("Expected pending execution after setting to true")
	}

	// Clear pending execution
	runner.setPendingExecution(false)
	if runner.hasPendingExecution() {
		t.Error("Expected no pending execution after setting to false")
	}
}
