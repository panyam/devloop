package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLROManager_ProcessTreeCleanup(t *testing.T) {
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Create test with command that spawns child processes
		configContent := `
settings:
  project_id: "process-tree-test"
  verbose: true

rules:
  - name: "server-with-children"
    lro: true
    skip_run_on_init: true
    commands:
      # This creates a process tree: bash -> python -> child processes
      - "bash -c \"python3 -c 'import time, subprocess; subprocess.Popen([\\\"sleep\\\", \\\"30\\\"]); time.sleep(30)'\""
    watch:
      - action: include
        patterns:
          - "*.txt"
`
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		orchestrator, err := NewOrchestrator(configPath)
		require.NoError(t, err)
		defer orchestrator.Stop()

		lroManager := orchestrator.lroManager
		rule := orchestrator.Config.Rules[0]

		// Start LRO process that creates children
		err = lroManager.RestartProcess(rule, "test")
		require.NoError(t, err)

		// Wait for process tree to establish
		time.Sleep(1 * time.Second)

		// Verify process is running
		assert.True(t, lroManager.IsProcessRunning("server-with-children"))
		processes := lroManager.GetRunningProcesses()
		require.Len(t, processes, 1)

		originalPID := processes["server-with-children"]
		t.Logf("Original process PID: %d", originalPID)

		// Test: Replace process (should kill entire tree)
		err = lroManager.RestartProcess(rule, "file_change")
		require.NoError(t, err)

		// Wait for replacement
		time.Sleep(1 * time.Second)

		// Verify new process started
		newProcesses := lroManager.GetRunningProcesses()
		require.Len(t, newProcesses, 1)
		newPID := newProcesses["server-with-children"]
		assert.NotEqual(t, originalPID, newPID, "Should have new PID after restart")

		// Test: Rapid restarts (stress test for race conditions)
		for i := 0; i < 3; i++ {
			err = lroManager.RestartProcess(rule, "rapid_test")
			assert.NoError(t, err)
			time.Sleep(100 * time.Millisecond) // Short delay between restarts
		}

		// After rapid restarts, should still have exactly 1 process
		finalProcesses := lroManager.GetRunningProcesses()
		assert.Len(t, finalProcesses, 1, "Should have exactly 1 process after rapid restarts")

		// Final cleanup
		err = lroManager.Stop()
		assert.NoError(t, err)

		// Verify complete cleanup
		time.Sleep(500 * time.Millisecond)
		assert.Empty(t, lroManager.GetRunningProcesses())
	})
}

func TestLROManager_ConcurrentOperations(t *testing.T) {
	testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		configContent := `
settings:
  project_id: "concurrent-test"
  verbose: true

rules:
  - name: "server-a"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 15"
    watch:
      - action: include
        patterns:
          - "*.go"
          
  - name: "server-b"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 15"
    watch:
      - action: include
        patterns:
          - "*.js"
`
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		orchestrator, err := NewOrchestrator(configPath)
		require.NoError(t, err)
		defer orchestrator.Stop()

		lroManager := orchestrator.lroManager

		// Start both processes concurrently
		ruleA := orchestrator.Config.Rules[0]
		ruleB := orchestrator.Config.Rules[1]

		// Simulate concurrent starts
		go func() {
			err := lroManager.RestartProcess(ruleA, "concurrent_a")
			assert.NoError(t, err)
		}()

		go func() {
			err := lroManager.RestartProcess(ruleB, "concurrent_b")
			assert.NoError(t, err)
		}()

		// Wait for both to start
		time.Sleep(1 * time.Second)

		// Verify both are running independently
		processes := lroManager.GetRunningProcesses()
		assert.Len(t, processes, 2)
		assert.Contains(t, processes, "server-a")
		assert.Contains(t, processes, "server-b")

		// Test concurrent restarts
		go func() {
			lroManager.RestartProcess(ruleA, "restart_a")
		}()

		go func() {
			lroManager.RestartProcess(ruleB, "restart_b")
		}()

		// Wait for restarts
		time.Sleep(1 * time.Second)

		// Should still have exactly 2 processes
		finalProcesses := lroManager.GetRunningProcesses()
		assert.Len(t, finalProcesses, 2)
	})
}
