package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_LROVsShortRunning(t *testing.T) {
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Create test with both LRO and short-running rules
		configContent := `
settings:
  project_id: "scheduler-test"
  max_parallel_rules: 2  # Limit short-running rules

rules:
  - name: "build"
    lro: false
    skip_run_on_init: true
    commands:
      - "echo 'Building...'"
      - "sleep 1"
      - "echo 'Build complete'"
    watch:
      - action: include
        patterns:
          - "*.go"
          
  - name: "test"
    lro: false  
    skip_run_on_init: true
    commands:
      - "echo 'Testing...'"
      - "sleep 1"
      - "echo 'Tests complete'"
    watch:
      - action: include
        patterns:
          - "*.go"
          
  - name: "dev-server"
    lro: true
    skip_run_on_init: true
    commands:
      - "bash -c \"echo 'Server starting...'; sleep 10; echo 'Server done'\""
    watch:
      - action: include
        patterns:
          - "*.go"
          
  - name: "worker-service" 
    lro: true
    skip_run_on_init: true
    commands:
      - "bash -c \"echo 'Worker starting...'; sleep 10; echo 'Worker done'\""
    watch:
      - action: include
        patterns:
          - "*.go"
`
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		orchestrator, err := NewOrchestrator(configPath)
		require.NoError(t, err)
		defer orchestrator.Stop()

		// Start orchestrator to initialize WorkerPool and other components
		go func() {
			if err := orchestrator.Start(); err != nil {
				t.Logf("Orchestrator start error: %v", err)
			}
		}()

		// Give time for components to start
		time.Sleep(200 * time.Millisecond)

		scheduler := orchestrator.scheduler

		// Test 1: Trigger short-running rules - should go to WorkerPool
		buildRule := orchestrator.Config.Rules[0] // "build"
		testRule := orchestrator.Config.Rules[1]  // "test"

		buildEvent := &TriggerEvent{
			Rule:        buildRule,
			TriggerType: "manual",
			Context:     context.Background(),
		}

		testEvent := &TriggerEvent{
			Rule:        testRule,
			TriggerType: "manual",
			Context:     context.Background(),
		}

		// Schedule both short-running jobs
		err = scheduler.ScheduleRule(buildEvent)
		assert.NoError(t, err)

		err = scheduler.ScheduleRule(testEvent)
		assert.NoError(t, err)

		// Wait for short jobs to complete
		time.Sleep(3 * time.Second)

		// Verify both completed successfully
		buildRunner := orchestrator.GetRuleRunner("build")
		testRunner := orchestrator.GetRuleRunner("test")

		// Debug status
		buildStatus := buildRunner.GetStatus()
		testStatus := testRunner.GetStatus()
		t.Logf("Build status: running=%t, status=%s", buildRunner.IsRunning(), buildStatus.LastBuildStatus)
		t.Logf("Test status: running=%t, status=%s", testRunner.IsRunning(), testStatus.LastBuildStatus)

		assert.False(t, buildRunner.IsRunning())
		assert.False(t, testRunner.IsRunning())

		assert.Equal(t, "SUCCESS", buildStatus.LastBuildStatus)
		assert.Equal(t, "SUCCESS", testStatus.LastBuildStatus)

		// Test 2: Trigger long-running rules - should go to WorkerPool
		devServerRule := orchestrator.Config.Rules[2] // "dev-server"
		workerRule := orchestrator.Config.Rules[3]    // "worker-service"

		devServerEvent := &TriggerEvent{
			Rule:        devServerRule,
			TriggerType: "file_change",
			Context:     context.Background(),
		}

		workerEvent := &TriggerEvent{
			Rule:        workerRule,
			TriggerType: "file_change",
			Context:     context.Background(),
		}

		// Schedule both LRO jobs
		err = scheduler.ScheduleRule(devServerEvent)
		assert.NoError(t, err)

		err = scheduler.ScheduleRule(workerEvent)
		assert.NoError(t, err)

		// Verify both LRO processes started and are running
		time.Sleep(500 * time.Millisecond)

		devServerRunner := orchestrator.GetRuleRunner("dev-server")
		workerRunner := orchestrator.GetRuleRunner("worker-service")

		assert.True(t, devServerRunner.IsRunning())
		assert.True(t, workerRunner.IsRunning())

		devServerStatus := devServerRunner.GetStatus()
		workerStatus := workerRunner.GetStatus()

		assert.Equal(t, "RUNNING", devServerStatus.LastBuildStatus)
		assert.Equal(t, "RUNNING", workerStatus.LastBuildStatus)

		// Verify long-running jobs are tracked in worker pool
		runningJobs := orchestrator.workerPool.GetExecutingRules()
		assert.Len(t, runningJobs, 2)
		assert.Contains(t, runningJobs, "dev-server")
		assert.Contains(t, runningJobs, "worker-service")

		// Test 3: Verify LRO processes don't block short-running jobs
		// Even with max_parallel_rules: 2, LRO processes shouldn't consume semaphore slots

		// Trigger another short job while LRO processes are running
		err = scheduler.ScheduleRule(buildEvent)
		assert.NoError(t, err)

		// This should complete quickly even though 2 LRO processes are running
		time.Sleep(2 * time.Second)

		buildStatus = buildRunner.GetStatus()
		assert.Equal(t, "SUCCESS", buildStatus.LastBuildStatus)
		assert.False(t, buildRunner.IsRunning())

		// LRO processes should still be running
		assert.True(t, devServerRunner.IsRunning())
		assert.True(t, workerRunner.IsRunning())
	})
}

func TestScheduler_RuleRouting(t *testing.T) {
	testhelpers.WithTestContext(t, 5*time.Second, func(t *testing.T, tmpDir string) {
		// Create minimal test setup
		configContent := `
settings:
  project_id: "routing-test"

rules:
  - name: "short-task"
    lro: false
    skip_run_on_init: true
    commands:
      - "echo 'short task'"
    watch:
      - action: include
        patterns:
          - "*.txt"
          
  - name: "long-service"
    lro: true
    skip_run_on_init: true
    commands:
      - "sleep 5"
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

		// Start orchestrator to initialize WorkerPool and other components
		go func() {
			if err := orchestrator.Start(); err != nil {
				t.Logf("Orchestrator start error: %v", err)
			}
		}()

		// Give time for components to start
		time.Sleep(200 * time.Millisecond)

		scheduler := orchestrator.scheduler
		shortRule := orchestrator.Config.Rules[0]
		longRule := orchestrator.Config.Rules[1]

		// Test: Short-running rule routing
		shortEvent := &TriggerEvent{
			Rule:        shortRule,
			TriggerType: "test",
			Context:     context.Background(),
		}

		err = scheduler.ScheduleRule(shortEvent)
		assert.NoError(t, err)

		// Wait for completion
		time.Sleep(1 * time.Second)

		shortRunner := orchestrator.GetRuleRunner("short-task")
		assert.False(t, shortRunner.IsRunning(), "Short task should complete quickly")
		assert.Equal(t, "SUCCESS", shortRunner.GetStatus().LastBuildStatus)

		// Test: Long-running rule routing
		longEvent := &TriggerEvent{
			Rule:        longRule,
			TriggerType: "test",
			Context:     context.Background(),
		}

		err = scheduler.ScheduleRule(longEvent)
		assert.NoError(t, err)

		// Check immediately - should be running
		time.Sleep(200 * time.Millisecond)

		longRunner := orchestrator.GetRuleRunner("long-service")
		assert.True(t, longRunner.IsRunning(), "Long service should be running")
		assert.Equal(t, "RUNNING", longRunner.GetStatus().LastBuildStatus)

		// Verify it's tracked as LRO process
		runningJobs := orchestrator.workerPool.GetExecutingRules()
		assert.Contains(t, runningJobs, "long-service")
	})
}
