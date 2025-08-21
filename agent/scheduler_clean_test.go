package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestScheduler_CleanAPI demonstrates the new clean testing pattern
func TestScheduler_CleanAPI(t *testing.T) {
	helper := NewTestHelper(t, 10*time.Second)

	// Define test configuration using builder pattern
	config := NewTestConfig("scheduler-clean-test").
		WithMaxWorkers(2).
		AddShortRunningRule("build", []string{"echo 'Building'", "sleep 1", "echo 'Done'"}).
		AddShortRunningRule("test", []string{"echo 'Testing'", "sleep 1", "echo 'Tested'"}).
		AddLRORule("dev-server", []string{"bash -c \"echo 'Server up'; sleep 10\""}).
		AddLRORule("worker", []string{"bash -c \"echo 'Worker up'; sleep 10\""})

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		scheduler := orchestrator.scheduler

		// Test 1: Short-running job routing
		t.Run("ShortRunningJobs", func(t *testing.T) {
			// Trigger build and test jobs
			buildEvent := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[0], // build
				TriggerType: "manual",
				Context:     context.Background(),
			}

			testEvent := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[1], // test
				TriggerType: "manual",
				Context:     context.Background(),
			}

			// Schedule jobs
			err := scheduler.ScheduleRule(buildEvent)
			require.NoError(t, err)

			err = scheduler.ScheduleRule(testEvent)
			require.NoError(t, err)

			// Wait for completion
			err = orchestrator.WaitForRuleCompletion("build", 5*time.Second)
			require.NoError(t, err, "Build should complete")

			err = orchestrator.WaitForRuleCompletion("test", 5*time.Second)
			require.NoError(t, err, "Test should complete")

			// Assert final states
			helper.AssertRuleStatus(orchestrator.Orchestrator, "build", false, "SUCCESS")
			helper.AssertRuleStatus(orchestrator.Orchestrator, "test", false, "SUCCESS")
		})

		// Test 2: LRO job routing
		t.Run("LROJobs", func(t *testing.T) {
			// Trigger LRO jobs
			serverEvent := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[2], // dev-server
				TriggerType: "file_change",
				Context:     context.Background(),
			}

			workerEvent := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[3], // worker
				TriggerType: "file_change",
				Context:     context.Background(),
			}

			// Schedule LRO jobs
			err := scheduler.ScheduleRule(serverEvent)
			require.NoError(t, err)

			err = scheduler.ScheduleRule(workerEvent)
			require.NoError(t, err)

			// Wait for LRO processes to start
			time.Sleep(500 * time.Millisecond)

			// Assert LRO states
			helper.AssertRuleStatus(orchestrator.Orchestrator, "dev-server", true, "RUNNING")
			helper.AssertRuleStatus(orchestrator.Orchestrator, "worker", true, "RUNNING")
			helper.AssertRunningJobCount(orchestrator.workerPool, 2)
		})

		// Test 3: Mixed workload (LRO processes don't block short jobs)
		t.Run("MixedWorkload", func(t *testing.T) {
			// LRO processes should already be running from previous test
			helper.AssertRunningJobCount(orchestrator.workerPool, 2)

			// Trigger another build while LRO processes are running
			buildEvent := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[0], // build
				TriggerType: "file_change",
				Context:     context.Background(),
			}

			err := scheduler.ScheduleRule(buildEvent)
			require.NoError(t, err)

			// Build should complete quickly despite LRO processes running
			err = orchestrator.WaitForRuleCompletion("build", 3*time.Second)
			require.NoError(t, err, "Build should complete even with LRO processes running")

			helper.AssertRuleStatus(orchestrator.Orchestrator, "build", false, "SUCCESS")

			// LRO processes should still be running
			helper.AssertRuleStatus(orchestrator.Orchestrator, "dev-server", true, "RUNNING")
			helper.AssertRuleStatus(orchestrator.Orchestrator, "worker", true, "RUNNING")
		})
	})
}

// TestWorkerPool_OnDemandScaling tests the new on-demand worker creation
func TestWorkerPool_OnDemandScaling(t *testing.T) {
	helper := NewTestHelper(t, 8*time.Second)

	config := NewTestConfig("worker-scaling-test").
		WithMaxWorkers(3). // Limit to 3 workers
		AddShortRunningRule("quick1", []string{"echo 'Quick 1'", "sleep 1"}).
		AddShortRunningRule("quick2", []string{"echo 'Quick 2'", "sleep 1"}).
		AddShortRunningRule("quick3", []string{"echo 'Quick 3'", "sleep 1"}).
		AddShortRunningRule("quick4", []string{"echo 'Quick 4'", "sleep 1"})

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		scheduler := orchestrator.scheduler

		t.Run("ScaleUpOnDemand", func(t *testing.T) {
			// Initially no workers should be created
			active, capacity, pending, executing := orchestrator.workerPool.GetStatus()
			t.Logf("Initial pool status: active=%d, capacity=%d, pending=%d, executing=%d",
				active, capacity, pending, executing)

			// Trigger first job - should create first worker
			event1 := &TriggerEvent{
				Rule:        orchestrator.Config.Rules[0],
				TriggerType: "test",
				Context:     context.Background(),
			}

			err := scheduler.ScheduleRule(event1)
			require.NoError(t, err)

			time.Sleep(100 * time.Millisecond) // Let worker start

			// Trigger more jobs to test scaling
			for i := 1; i < 4; i++ {
				event := &TriggerEvent{
					Rule:        orchestrator.Config.Rules[i],
					TriggerType: "test",
					Context:     context.Background(),
				}
				err := scheduler.ScheduleRule(event)
				require.NoError(t, err)
			}

			// Wait for all jobs to complete
			for i := 0; i < 4; i++ {
				ruleName := orchestrator.Config.Rules[i].Name
				err := orchestrator.WaitForRuleCompletion(ruleName, 5*time.Second)
				require.NoError(t, err, "Job %d should complete", i+1)
			}

			// All jobs should have completed successfully
			for i := 0; i < 4; i++ {
				ruleName := orchestrator.Config.Rules[i].Name
				helper.AssertRuleStatus(orchestrator.Orchestrator, ruleName, false, "SUCCESS")
			}
		})
	})
}
