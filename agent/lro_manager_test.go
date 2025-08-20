package agent

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLROManager_BasicLifecycle(t *testing.T) {
	testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Create test orchestrator
		configContent := `
settings:
  project_id: "lro-test"

rules:
  - name: "test-server"
    lro: true
    skip_run_on_init: true  # Don't auto-start, we'll start manually in test
    commands:
      - "sleep 30"  # Long-running command
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

		// Start orchestrator but not in background (no file watching needed for this test)
		// We'll manually trigger LRO processes

		lroManager := orchestrator.lroManager
		rule := orchestrator.Config.Rules[0] // "test-server" rule

		// Test: No processes running initially
		assert.False(t, lroManager.IsProcessRunning("test-server"))
		assert.Empty(t, lroManager.GetRunningProcesses())

		// Test: Start LRO process
		err = lroManager.RestartProcess(rule, "manual")
		assert.NoError(t, err)

		// Verify process is tracked
		assert.True(t, lroManager.IsProcessRunning("test-server"))
		processes := lroManager.GetRunningProcesses()
		assert.Len(t, processes, 1)
		assert.Contains(t, processes, "test-server")

		// Verify status callback worked
		ruleRunner := orchestrator.GetRuleRunner("test-server")
		require.NotNil(t, ruleRunner)
		assert.True(t, ruleRunner.IsRunning())
		status := ruleRunner.GetStatus()
		assert.Equal(t, "RUNNING", status.LastBuildStatus)

		// Test: Process replacement
		oldPID := processes["test-server"]

		// Restart the process (simulates file change)
		err = lroManager.RestartProcess(rule, "file_change")
		assert.NoError(t, err)

		// Verify old process was killed and new one started
		newProcesses := lroManager.GetRunningProcesses()
		assert.Len(t, newProcesses, 1)
		newPID := newProcesses["test-server"]
		assert.NotEqual(t, oldPID, newPID, "Process should be replaced with new PID")

		// Verify old process is actually dead
		err = syscall.Kill(oldPID, 0)
		assert.Error(t, err, "Old process should be terminated")

		// Test: Manual stop
		err = lroManager.Stop()
		assert.NoError(t, err)

		// Give some time for cleanup
		time.Sleep(100 * time.Millisecond)

		// Verify all processes stopped
		assert.False(t, lroManager.IsProcessRunning("test-server"))
		assert.Empty(t, lroManager.GetRunningProcesses())

		// Verify final status callback - should be false since process was manually stopped
		assert.False(t, ruleRunner.IsRunning(), "Rule should not be running after LRO manager stop")
	})
}

func TestLROManager_ProcessTermination(t *testing.T) {
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Create test orchestrator with a simple LRO command
		configContent := `
settings:
  project_id: "termination-test"

rules:
  - name: "heartbeat"
    lro: true
    skip_run_on_init: true
    commands:
      - "bash -c \"while true; do echo 'beat' >> heartbeat.log; sleep 0.1; done\""
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
		heartbeatFile := filepath.Join(tmpDir, "heartbeat.log")

		// Start LRO process
		err = lroManager.RestartProcess(rule, "startup")
		require.NoError(t, err)

		// Wait for process to start writing
		time.Sleep(500 * time.Millisecond)

		// Verify heartbeat file is being written
		_, err = os.Stat(heartbeatFile)
		assert.NoError(t, err, "Heartbeat file should exist")

		initialContent, err := os.ReadFile(heartbeatFile)
		require.NoError(t, err)
		initialSize := len(initialContent)
		assert.Greater(t, initialSize, 0, "Heartbeat should have written some content")

		// Test graceful termination
		processes := lroManager.GetRunningProcesses()
		require.Len(t, processes, 1)
		originalPID := processes["heartbeat"]

		// Kill the process manually (simulates restart)
		err = lroManager.killExistingProcess("heartbeat")
		assert.NoError(t, err)

		// Wait a bit to ensure process is fully terminated
		time.Sleep(200 * time.Millisecond)

		// Verify process is dead
		err = syscall.Kill(originalPID, 0)
		assert.Error(t, err, "Process should be terminated")

		// Verify heartbeat stops growing
		finalContent, err := os.ReadFile(heartbeatFile)
		require.NoError(t, err)

		// Wait a bit more and check that file stopped growing
		time.Sleep(500 * time.Millisecond)
		verifyContent, err := os.ReadFile(heartbeatFile)
		require.NoError(t, err)

		assert.Equal(t, len(finalContent), len(verifyContent),
			"Heartbeat file should stop growing after process termination")
	})
}

func TestLROManager_MultipleProcesses(t *testing.T) {
	testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Create test orchestrator with multiple LRO rules
		configContent := `
settings:
  project_id: "multi-lro-test"

rules:
  - name: "server1"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 20"
    watch:
      - action: include
        patterns:
          - "*.txt"
          
  - name: "server2" 
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 20"
    watch:
      - action: include
        patterns:
          - "*.txt"
          
  - name: "worker"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 20"
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

		// Start all LRO processes
		for _, rule := range orchestrator.Config.Rules {
			err = lroManager.RestartProcess(rule, "startup")
			assert.NoError(t, err, "Failed to start LRO process for rule %s", rule.Name)
		}

		// Verify all processes are running
		processes := lroManager.GetRunningProcesses()
		assert.Len(t, processes, 3)
		assert.Contains(t, processes, "server1")
		assert.Contains(t, processes, "server2")
		assert.Contains(t, processes, "worker")

		// Verify all PIDs are different
		pids := make(map[int]bool)
		for _, pid := range processes {
			assert.False(t, pids[pid], "PIDs should be unique")
			pids[pid] = true
		}

		// Test: Restart one process while others continue
		server1Rule := orchestrator.Config.Rules[0]
		originalPID := processes["server1"]

		err = lroManager.RestartProcess(server1Rule, "file_change")
		assert.NoError(t, err)

		// Verify server1 got new PID, others unchanged
		newProcesses := lroManager.GetRunningProcesses()
		assert.Len(t, newProcesses, 3)
		assert.NotEqual(t, originalPID, newProcesses["server1"], "server1 should have new PID")
		assert.Equal(t, processes["server2"], newProcesses["server2"], "server2 PID should be unchanged")
		assert.Equal(t, processes["worker"], newProcesses["worker"], "worker PID should be unchanged")

		// Test: Stop all processes
		err = lroManager.Stop()
		assert.NoError(t, err)

		// Verify all processes stopped
		assert.Empty(t, lroManager.GetRunningProcesses())
		assert.False(t, lroManager.IsProcessRunning("server1"))
		assert.False(t, lroManager.IsProcessRunning("server2"))
		assert.False(t, lroManager.IsProcessRunning("worker"))
	})
}

func TestLROManager_FailedStartup(t *testing.T) {
	testhelpers.WithTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		// Create test with command that will fail
		configContent := `
settings:
  project_id: "failure-test"

rules:
  - name: "failing-server"
    lro: true
    skip_run_on_init: true
    commands:
      - "nonexistent-command-that-will-fail"
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

		// Test: Failed startup (process starts but command fails quickly)
		err = lroManager.RestartProcess(rule, "startup")
		require.NoError(t, err, "RestartProcess should not fail immediately - bash starts successfully")

		// Wait for the bash process to fail when trying to execute nonexistent command
		time.Sleep(500 * time.Millisecond)

		// Verify process is no longer tracked (should have exited)
		assert.False(t, lroManager.IsProcessRunning("failing-server"))
		assert.Empty(t, lroManager.GetRunningProcesses())

		// Verify status was updated to FAILED by the monitor
		ruleRunner := orchestrator.GetRuleRunner("failing-server")
		require.NotNil(t, ruleRunner)
		status := ruleRunner.GetStatus()
		assert.Equal(t, "FAILED", status.LastBuildStatus)
		assert.False(t, ruleRunner.IsRunning())
	})
}

func TestLROManager_StatusCallbacks(t *testing.T) {
	testhelpers.WithTestContext(t, 8*time.Second, func(t *testing.T, tmpDir string) {
		// Create test orchestrator
		configContent := `
settings:
  project_id: "status-test"

rules:
  - name: "status-server"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 5"  # Will complete after 5 seconds
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
		ruleRunner := orchestrator.GetRuleRunner("status-server")
		require.NotNil(t, ruleRunner)

		// Initial state
		assert.False(t, ruleRunner.IsRunning())

		// Start LRO process
		err = lroManager.RestartProcess(rule, "manual")
		require.NoError(t, err)

		// Verify status updated to RUNNING
		assert.True(t, ruleRunner.IsRunning())
		status := ruleRunner.GetStatus()
		assert.Equal(t, "RUNNING", status.LastBuildStatus)
		assert.NotNil(t, status.StartTime)

		// Wait for process to complete (sleep 5)
		time.Sleep(6 * time.Second)

		// Verify status updated to COMPLETED
		assert.False(t, ruleRunner.IsRunning())
		finalStatus := ruleRunner.GetStatus()
		assert.Equal(t, "COMPLETED", finalStatus.LastBuildStatus)
		assert.NotNil(t, finalStatus.LastBuildTime)
		_ = finalStatus // Mark as used

		// Verify process is no longer tracked
		assert.False(t, lroManager.IsProcessRunning("status-server"))
		assert.Empty(t, lroManager.GetRunningProcesses())
	})
}
