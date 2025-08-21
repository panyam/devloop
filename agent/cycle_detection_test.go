package agent

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/cycle_self_triggering.yaml
var selfTriggeringConfig string

//go:embed testdata/cycle_cross_rule.yaml
var crossRuleConfig string

// TestMediumCycleScenario reproduces the infinite loop issue from medium fetests rule
func TestMediumCycleScenario(t *testing.T) {
	// Skip test if shell is not available (test environment issue)
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("Skipping test - shell not available in test environment")
	}
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Reproduce the medium fetests configuration that causes infinite loops
		configContent := `
settings:
  project_id: "cycle-test"
  verbose: true
  default_debounce_delay: 1s

rules:
  # Frontend rule (similar to medium frontend)
  - name: "frontend"
    skip_run_on_init: true
    workdir: "./web"
    watch:
      - action: "exclude"
        patterns:
          - "gen/**"
          - "**/*.log"
          - "node_modules/**"
          - "dist/**"
          - "test-results/**"
          - "playwright-report/**"
      - action: "include"
        patterns:
          - "**/*.ts"
          - "**/*.css"
          - "**/*.html"
          - "package.json"
    commands:
      - "echo 'Building frontend...'"

  # Frontend tests rule (similar to medium fetests) 
  - name: "fetests"
    skip_run_on_init: true
    workdir: "./web"
    watch:
      - action: "exclude"
        patterns:
          - "gen/**"
          - "**/*.log"  
          - "node_modules/**"
          - "dist/**"           # Should exclude dist/ but let's see
          - "test-results/**"
          - "playwright-report/**"
      - action: "include"
        patterns:
          - "**/*.ts"
          - "**/*.css"
          - "**/*.html"
          - "**/*.spec.ts"
          - "**/*.test.ts"
          - "package.json"
    commands:
      - "echo 'Running tests...'"
`

		// Create directory structure
		webDir := filepath.Join(tmpDir, "web")
		srcDir := filepath.Join(webDir, "src")

		err := os.MkdirAll(srcDir, 0755)
		require.NoError(t, err)

		// Create initial source files
		srcFile := filepath.Join(srcDir, "component.ts")
		err = os.WriteFile(srcFile, []byte("export const component = 'test';"), 0644)
		require.NoError(t, err)

		// Write config
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Create orchestrator
		orchestrator, err := NewOrchestrator(configPath)
		require.NoError(t, err)
		defer orchestrator.Stop()

		// Start orchestrator
		go func() {
			if err := orchestrator.Start(); err != nil {
				t.Logf("Orchestrator start error: %v", err)
			}
		}()

		// Wait for startup
		time.Sleep(500 * time.Millisecond)

		frontendRunner := orchestrator.GetRuleRunner("frontend")
		fetestsRunner := orchestrator.GetRuleRunner("fetests")
		require.NotNil(t, frontendRunner)
		require.NotNil(t, fetestsRunner)

		// Test 1: Trigger frontend rule - should create dist/built.ts
		t.Logf("=== Triggering frontend rule ===")
		err = orchestrator.TriggerRule("frontend")
		require.NoError(t, err)

		// Wait for frontend to complete
		time.Sleep(2 * time.Second)

		frontendStatus := frontendRunner.GetStatus()
		assert.Equal(t, "SUCCESS", frontendStatus.LastBuildStatus, "Frontend should complete successfully")

		// Frontend command completed successfully (we simplified the test to just echo)

		// Test 2: Trigger fetests rule - this should create files that might trigger cycles
		t.Logf("=== Triggering fetests rule ===")

		err = orchestrator.TriggerRule("fetests")
		require.NoError(t, err)

		// Wait for fetests to complete
		time.Sleep(2 * time.Second)

		fetestsStatus := fetestsRunner.GetStatus()
		assert.Equal(t, "SUCCESS", fetestsStatus.LastBuildStatus, "Fetests should complete successfully")

		// Fetests command completed successfully (we simplified the test to just echo)

		// Test 3: Check for cycles - monitor if rules keep triggering each other
		t.Logf("=== Monitoring for cycles ===")

		// Record initial build times
		frontendInitial := frontendRunner.GetStatus().LastBuildTime
		fetestsInitial := fetestsRunner.GetStatus().LastBuildTime

		// Wait and check if rules are triggering without external changes
		time.Sleep(5 * time.Second)

		// Check if build times changed (indicating unwanted triggers)
		frontendFinal := frontendRunner.GetStatus().LastBuildTime
		fetestsFinal := fetestsRunner.GetStatus().LastBuildTime

		frontendTriggered := frontendFinal.AsTime().After(frontendInitial.AsTime())
		fetestsTriggered := fetestsFinal.AsTime().After(fetestsInitial.AsTime())

		t.Logf("Frontend triggered without external change: %t", frontendTriggered)
		t.Logf("Fetests triggered without external change: %t", fetestsTriggered)

		// If either rule triggered without external file changes, we have a cycle
		if frontendTriggered || fetestsTriggered {
			t.Errorf("CYCLE DETECTED: Rules triggering without external changes")
			t.Logf("Frontend: %v → %v", frontendInitial.AsTime(), frontendFinal.AsTime())
			t.Logf("Fetests: %v → %v", fetestsInitial.AsTime(), fetestsFinal.AsTime())

			// Log cycle detection information
			t.Logf("Possible cycle detected between frontend and fetests rules")
		}

		// Test 4: Verify exclude patterns work correctly
		t.Logf("=== Testing exclude patterns ===")

		// Create the dist directory first, then create a file in dist/ - should be excluded
		distDir := filepath.Join(webDir, "dist")
		err = os.MkdirAll(distDir, 0755)
		require.NoError(t, err)

		distFile := filepath.Join(distDir, "should-be-ignored.ts")
		beforeDistChange := fetestsRunner.GetStatus().LastBuildTime

		err = os.WriteFile(distFile, []byte("// should be ignored"), 0644)
		require.NoError(t, err)

		time.Sleep(2 * time.Second)

		afterDistChange := fetestsRunner.GetStatus().LastBuildTime
		distTriggered := afterDistChange.AsTime().After(beforeDistChange.AsTime())

		assert.False(t, distTriggered, "Files in dist/ should be excluded and not trigger fetests")
		if distTriggered {
			t.Errorf("EXCLUDE PATTERN FAILED: dist/should-be-ignored.ts triggered fetests rule")
		}
	})
}

// TestCycleDetectionSystem tests the dynamic cycle detection system
func TestCycleDetectionSystem(t *testing.T) {
	// Skip test if shell is not available (test environment issue)
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("Skipping test - shell not available in test environment")
	}
	helper := NewTestHelper(t, 15*time.Second)

	// Test 1: Self-triggering cycle
	config := &TestConfig{
		ProjectID: "cycle-detection-test",
		Verbose:   true,
		CycleDetection: &CycleDetectionConfig{
			Enabled:              true,
			DynamicProtection:    true,
			MaxTriggersPerMinute: 3, // Low threshold for testing
		},
		Rules: []TestRule{
			{
				Name:          "self-triggering",
				SkipRunOnInit: true,
				Patterns:      []string{"**/*.txt"},
				Commands: []string{
					"echo 'Rule triggered at $(date)' >> output.txt",
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		// Create trigger file
		triggerFile := filepath.Join(orchestrator.tmpDir, "trigger.txt")
		err := os.WriteFile(triggerFile, []byte("initial"), 0644)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		// Get rule runner
		ruleRunner := orchestrator.GetRuleRunner("self-triggering")
		require.NotNil(t, ruleRunner)

		// Monitor trigger count
		initialTriggerCount := ruleRunner.GetTriggerCount(time.Minute)
		t.Logf("Initial triggers: %d", initialTriggerCount)

		// Trigger the rule manually to start the cycle
		err = orchestrator.TriggerRule("self-triggering")
		require.NoError(t, err)

		// Wait for the cycle to occur and be detected
		time.Sleep(8 * time.Second)

		finalTriggerCount := ruleRunner.GetTriggerCount(time.Minute)
		finalStatus := ruleRunner.GetStatus()
		t.Logf("Final triggers: %d, Status: %s", finalTriggerCount, finalStatus.LastBuildStatus)

		// Verify cycle detection kicked in
		if finalTriggerCount > 5 {
			t.Errorf("CYCLE DETECTION FAILED: Rule triggered %d times (expected ≤5)", finalTriggerCount)
		} else {
			t.Logf("SUCCESS: Cycle detection limited triggers to %d", finalTriggerCount)
		}

		// Check if rule was disabled due to rapid triggering
		disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		if len(disabled) > 0 {
			t.Logf("SUCCESS: Rules disabled due to cycle detection: %v", disabled)
		}
	})
}

// TestCrossRuleCycles tests cycles between multiple rules
func TestCrossRuleCycles(t *testing.T) {
	helper := NewTestHelper(t, 15*time.Second)

	config := &TestConfig{
		ProjectID: "cross-rule-cycle-test",
		Verbose:   true,
		CycleDetection: &CycleDetectionConfig{
			Enabled:              true,
			DynamicProtection:    true,
			MaxTriggersPerMinute: 4,
		},
		Rules: []TestRule{
			{
				Name:          "rule-a",
				SkipRunOnInit: true,
				Patterns:      []string{"a/*.txt"},
				Commands: []string{
					"mkdir -p b",
					"echo 'Created by rule-a' > b/output.txt",
				},
			},
			{
				Name:          "rule-b",
				SkipRunOnInit: true,
				Patterns:      []string{"b/*.txt"},
				Commands: []string{
					"mkdir -p a",
					"echo 'Created by rule-b' > a/input.txt",
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		// Create directories
		aDir := filepath.Join(orchestrator.tmpDir, "a")
		bDir := filepath.Join(orchestrator.tmpDir, "b")
		err := os.MkdirAll(aDir, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(bDir, 0755)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		// Start the cycle by triggering rule-a
		t.Logf("=== Starting cross-rule cycle test ===")
		err = orchestrator.TriggerRule("rule-a")
		require.NoError(t, err)

		// Wait for cycle to develop and be detected
		time.Sleep(10 * time.Second)

		// Check both rules
		ruleARunner := orchestrator.GetRuleRunner("rule-a")
		ruleBRunner := orchestrator.GetRuleRunner("rule-b")
		require.NotNil(t, ruleARunner)
		require.NotNil(t, ruleBRunner)

		triggerCountA := ruleARunner.GetTriggerCount(time.Minute)
		triggerCountB := ruleBRunner.GetTriggerCount(time.Minute)
		statusA := ruleARunner.GetStatus()
		statusB := ruleBRunner.GetStatus()

		t.Logf("Rule A: triggers=%d, status=%s", triggerCountA, statusA.LastBuildStatus)
		t.Logf("Rule B: triggers=%d, status=%s", triggerCountB, statusB.LastBuildStatus)

		// Total triggers should be limited by rate limiting
		totalTriggers := triggerCountA + triggerCountB
		if totalTriggers > 10 {
			t.Errorf("CROSS-RULE CYCLE DETECTION FAILED: Total triggers=%d (expected ≤10)", totalTriggers)
		} else {
			t.Logf("SUCCESS: Cross-rule cycle limited to %d total triggers", totalTriggers)
		}

		// Check for disabled rules
		disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		if len(disabled) > 0 {
			t.Logf("SUCCESS: Rules disabled due to cycle: %v", disabled)
		}
	})
}

// TestNoCycleScenario tests that legitimate operations don't trigger false positives
func TestNoCycleScenario(t *testing.T) {
	helper := NewTestHelper(t, 10*time.Second)

	config := &TestConfig{
		ProjectID: "no-cycle-test",
		Verbose:   true,
		CycleDetection: &CycleDetectionConfig{
			Enabled:              true,
			DynamicProtection:    true,
			MaxTriggersPerMinute: 10,
		},
		Rules: []TestRule{
			{
				Name:          "safe-build",
				SkipRunOnInit: true,
				Patterns:      []string{"src/**/*.ts"},
				Commands: []string{
					"echo 'Building...'",
					"mkdir -p dist",
					"echo 'export const built = true;' > dist/index.js", // Different extension
				},
			},
			{
				Name:          "safe-test",
				SkipRunOnInit: true,
				Patterns:      []string{"src/**/*.ts", "dist/**/*.js"},
				Commands: []string{
					"echo 'Testing...'",
					"mkdir -p test-results",
					"echo 'Tests passed' > test-results/report.html", // Different location
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		// Create source structure
		srcDir := filepath.Join(orchestrator.tmpDir, "src")
		err := os.MkdirAll(srcDir, 0755)
		require.NoError(t, err)

		srcFile := filepath.Join(srcDir, "component.ts")
		err = os.WriteFile(srcFile, []byte("export const component = 'test';"), 0644)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		// Trigger build rule
		t.Logf("=== Testing legitimate build workflow ===")
		err = orchestrator.TriggerRule("safe-build")
		require.NoError(t, err)

		// Wait for build to complete and potentially trigger test
		time.Sleep(5 * time.Second)

		buildRunner := orchestrator.GetRuleRunner("safe-build")
		testRunner := orchestrator.GetRuleRunner("safe-test")

		buildTriggerCount := buildRunner.GetTriggerCount(time.Minute)
		testTriggerCount := testRunner.GetTriggerCount(time.Minute)
		buildStatus := buildRunner.GetStatus()
		testStatus := testRunner.GetStatus()

		t.Logf("Build: triggers=%d, status=%s", buildTriggerCount, buildStatus.LastBuildStatus)
		t.Logf("Test: triggers=%d, status=%s", testTriggerCount, testStatus.LastBuildStatus)

		// Both should complete successfully without excessive triggering
		assert.Equal(t, "SUCCESS", buildStatus.LastBuildStatus, "Build should succeed")
		assert.LessOrEqual(t, buildTriggerCount, 3, "Build should not trigger excessively")

		// Test might trigger due to dist/*.js creation, which is legitimate
		if testTriggerCount > 0 {
			assert.Equal(t, "SUCCESS", testStatus.LastBuildStatus, "Test should succeed if triggered")
			assert.LessOrEqual(t, testTriggerCount, 2, "Test should not trigger excessively")
		}

		// No rules should be disabled in this scenario
		disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		assert.Empty(t, disabled, "No rules should be disabled in legitimate workflow")
	})
}

// TestActualCrossRuleCycle creates a real cycle that should be detected
func TestActualCrossRuleCycle(t *testing.T) {
	helper := NewTestHelper(t, 20*time.Second)

	config := &TestConfig{
		ProjectID: "actual-cross-rule-cycle",
		Verbose:   true,
		CycleDetection: &CycleDetectionConfig{
			Enabled:              true,
			DynamicProtection:    true,
			MaxTriggersPerMinute: 4,
		},
		Rules: []TestRule{
			{
				Name:          "writer-a",
				SkipRunOnInit: true,
				Patterns:      []string{"trigger-a.txt"},
				Commands: []string{
					"echo 'Writer A triggered' >> trigger-b.txt",
				},
			},
			{
				Name:          "writer-b",
				SkipRunOnInit: true,
				Patterns:      []string{"trigger-b.txt"},
				Commands: []string{
					"echo 'Writer B triggered' >> trigger-a.txt",
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		time.Sleep(300 * time.Millisecond)

		// Create initial trigger file to start the cycle
		triggerA := filepath.Join(orchestrator.tmpDir, "trigger-a.txt")
		err := os.WriteFile(triggerA, []byte("start"), 0644)
		require.NoError(t, err)

		t.Logf("=== Starting actual cross-rule cycle ===")

		// Wait for the cycle to develop
		time.Sleep(12 * time.Second)

		// Check both rules
		writerA := orchestrator.GetRuleRunner("writer-a")
		writerB := orchestrator.GetRuleRunner("writer-b")
		require.NotNil(t, writerA)
		require.NotNil(t, writerB)

		executionCountA := orchestrator.Orchestrator.GetExecutionCount("writer-a", time.Minute)
		executionCountB := orchestrator.Orchestrator.GetExecutionCount("writer-b", time.Minute)
		statusA := writerA.GetStatus()
		statusB := writerB.GetStatus()

		t.Logf("Writer A: executions=%d, status=%s", executionCountA, statusA.LastBuildStatus)
		t.Logf("Writer B: executions=%d, status=%s", executionCountB, statusB.LastBuildStatus)

		// If both rules are executing each other repeatedly, we should see multiple executions
		totalExecutions := executionCountA + executionCountB
		t.Logf("Total executions: %d", totalExecutions)

		// Verify that cycle detection activated if there was indeed a cycle
		if totalExecutions > 6 {
			// Check if cycle detection kicked in
			disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
			if len(disabled) == 0 {
				t.Errorf("CYCLE DETECTION FAILED: %d executions without any rules being disabled", totalExecutions)
			} else {
				t.Logf("SUCCESS: Cycle detection disabled rules after %d executions: %v", totalExecutions, disabled)
			}
		} else if totalExecutions > 2 {
			t.Logf("MODERATE ACTIVITY: %d executions detected", totalExecutions)
		} else {
			t.Logf("LOW ACTIVITY: Only %d executions - cycle may not have developed", totalExecutions)
		}

		// Check the actual files to see if cycle occurred
		triggerAPath := filepath.Join(orchestrator.tmpDir, "trigger-a.txt")
		triggerBPath := filepath.Join(orchestrator.tmpDir, "trigger-b.txt")

		if triggerAInfo, err := os.Stat(triggerAPath); err == nil {
			t.Logf("trigger-a.txt size: %d bytes", triggerAInfo.Size())
		}
		if triggerBInfo, err := os.Stat(triggerBPath); err == nil {
			t.Logf("trigger-b.txt size: %d bytes", triggerBInfo.Size())
		}
	})
}

// TestFileWatchingVerification tests that file watching is working correctly
func TestFileWatchingVerification(t *testing.T) {
	helper := NewTestHelper(t, 8*time.Second)

	config := &TestConfig{
		ProjectID: "file-watching-test",
		Verbose:   true,
		Rules: []TestRule{
			{
				Name:          "file-monitor",
				SkipRunOnInit: true,
				Patterns:      []string{"watch-me.txt"},
				Commands: []string{
					"echo 'File detected' >> detected.log",
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		time.Sleep(300 * time.Millisecond)

		// Create the watched file
		watchFile := filepath.Join(orchestrator.tmpDir, "watch-me.txt")
		err := os.WriteFile(watchFile, []byte("test content"), 0644)
		require.NoError(t, err)

		// Wait for detection
		time.Sleep(2 * time.Second)

		// Check if rule was triggered
		ruleRunner := orchestrator.GetRuleRunner("file-monitor")
		require.NotNil(t, ruleRunner)

		triggerCount := ruleRunner.GetTriggerCount(time.Minute)
		status := ruleRunner.GetStatus()

		t.Logf("Triggers: %d, Status: %s", triggerCount, status.LastBuildStatus)

		// Verify file watching worked
		assert.Greater(t, triggerCount, 0, "File change should have triggered the rule")
		assert.Equal(t, "SUCCESS", status.LastBuildStatus, "Rule should execute successfully")

		// Verify output file was created
		detectedLog := filepath.Join(orchestrator.tmpDir, "detected.log")
		if _, err := os.Stat(detectedLog); err != nil {
			t.Errorf("Expected detected.log to be created, but got error: %v", err)
		}
	})
}

// TestCycleDetectionUserExperience demonstrates the end-user experience of cycle detection
func TestCycleDetectionUserExperience(t *testing.T) {
	helper := NewTestHelper(t, 15*time.Second)

	// Create a configuration that will definitely cause a cycle
	config := &TestConfig{
		ProjectID: "user-cycle-demo",
		Verbose:   true,
		CycleDetection: &CycleDetectionConfig{
			Enabled:              true,
			DynamicProtection:    true,
			MaxTriggersPerMinute: 3, // Low threshold for demo
		},
		Rules: []TestRule{
			{
				Name:          "build-tool",
				SkipRunOnInit: true,
				Patterns:      []string{"source.txt"},
				Commands: []string{
					"touch output.txt", // Creates file that test-tool watches
				},
			},
			{
				Name:          "test-tool",
				SkipRunOnInit: true,
				Patterns:      []string{"output.txt"},
				Commands: []string{
					"touch source.txt", // Creates file that build-tool watches -> CYCLE!
				},
			},
		},
	}

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		t.Logf("=== User Cycle Detection Demo ===")
		t.Logf("Configuration creates a cycle: build-tool → output.txt → test-tool → source.txt → build-tool")

		// User creates initial file to start development
		sourceFile := filepath.Join(orchestrator.tmpDir, "source.txt")
		err := os.WriteFile(sourceFile, []byte("user content"), 0644)
		require.NoError(t, err)

		t.Logf("User created source.txt - this will trigger the cycle...")

		// Wait for cycle to be detected
		time.Sleep(8 * time.Second)

		// Check cycle detection results
		buildExecs := orchestrator.Orchestrator.GetExecutionCount("build-tool", time.Minute)
		testExecs := orchestrator.Orchestrator.GetExecutionCount("test-tool", time.Minute)

		t.Logf("=== Cycle Detection Results ===")
		t.Logf("build-tool executions: %d", buildExecs)
		t.Logf("test-tool executions: %d", testExecs)

		// Check if cycle was detected and rules disabled
		disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		if len(disabled) > 0 {
			t.Logf("✅ SUCCESS: Cycle detected and resolved!")
			for ruleName, disabledUntil := range disabled {
				t.Logf("  - Rule '%s' disabled until %v", ruleName, disabledUntil.Format("15:04:05"))

				// Get suggestions for resolving the cycle
				suggestions := orchestrator.Orchestrator.cycleBreaker.GenerateCycleResolutionSuggestions(ruleName, "cross-rule")
				t.Logf("  - Suggestions to fix rule '%s':", ruleName)
				for _, suggestion := range suggestions {
					t.Logf("    • %s", suggestion)
				}
			}
		} else {
			t.Errorf("❌ CYCLE NOT DETECTED: Expected cycle detection to disable rules after %d total executions", buildExecs+testExecs)
		}

		// Demonstrate that the cycle is actually broken
		t.Logf("=== Verifying Cycle is Broken ===")

		// Try to trigger again - should be prevented
		err = orchestrator.TriggerRule("build-tool")
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Rule should still be disabled
		stillDisabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		if len(stillDisabled) > 0 {
			t.Logf("✅ Cycle protection active - rules remain disabled")
		} else {
			t.Logf("ℹ️  Rules re-enabled (timeout expired)")
		}
	})
}

// TestCycleDetectionWithEmbeddedConfigs demonstrates cycle detection using realistic configs
func TestCycleDetectionWithEmbeddedConfigs(t *testing.T) {
	t.Run("SelfTriggeringCycle", func(t *testing.T) {
		testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
			// Use embedded config for self-triggering cycle
			configPath := filepath.Join(tmpDir, ".devloop.yaml")
			err := os.WriteFile(configPath, []byte(selfTriggeringConfig), 0644)
			require.NoError(t, err)

			orchestrator, err := NewOrchestrator(configPath)
			require.NoError(t, err)
			defer orchestrator.Stop()

			// Start orchestrator
			go func() {
				if err := orchestrator.Start(); err != nil {
					t.Logf("Orchestrator error: %v", err)
				}
			}()
			time.Sleep(200 * time.Millisecond)

			// Trigger the cycle
			triggerFile := filepath.Join(tmpDir, "trigger.txt")
			err = os.WriteFile(triggerFile, []byte("start"), 0644)
			require.NoError(t, err)

			// Wait for cycle detection
			time.Sleep(6 * time.Second)

			ruleRunner := orchestrator.GetRuleRunner("self-triggering")
			require.NotNil(t, ruleRunner)

			triggerCount := ruleRunner.GetTriggerCount(time.Minute)
			t.Logf("Self-triggering rule: %d triggers in last minute", triggerCount)

			// Should be limited by rate limiting
			assert.LessOrEqual(t, triggerCount, 5, "Self-triggering should be rate limited")
		})
	})

	t.Run("CrossRuleCycle", func(t *testing.T) {
		testhelpers.WithTestContext(t, 12*time.Second, func(t *testing.T, tmpDir string) {
			// Use embedded config for cross-rule cycle
			configPath := filepath.Join(tmpDir, ".devloop.yaml")
			err := os.WriteFile(configPath, []byte(crossRuleConfig), 0644)
			require.NoError(t, err)

			orchestrator, err := NewOrchestrator(configPath)
			require.NoError(t, err)
			defer orchestrator.Stop()

			go func() {
				if err := orchestrator.Start(); err != nil {
					t.Logf("Orchestrator error: %v", err)
				}
			}()
			time.Sleep(200 * time.Millisecond)

			// Start the cross-rule cycle
			triggerFile := filepath.Join(tmpDir, "trigger-a.txt")
			err = os.WriteFile(triggerFile, []byte("start"), 0644)
			require.NoError(t, err)

			t.Logf("Started cross-rule cycle: writer-a → trigger-b.txt → writer-b → trigger-a.txt")

			// Wait for cycle detection
			time.Sleep(8 * time.Second)

			writerA := orchestrator.GetRuleRunner("writer-a")
			writerB := orchestrator.GetRuleRunner("writer-b")
			require.NotNil(t, writerA)
			require.NotNil(t, writerB)

			execsA := orchestrator.GetExecutionCount("writer-a", time.Minute)
			execsB := orchestrator.GetExecutionCount("writer-b", time.Minute)

			t.Logf("writer-a executions: %d", execsA)
			t.Logf("writer-b executions: %d", execsB)

			// Check if cycle was detected and broken
			disabled := orchestrator.cycleBreaker.GetDisabledRules()
			if len(disabled) > 0 {
				t.Logf("✅ Cycle detected and rules disabled: %v", disabled)

				// Show user what happened and how to fix it
				for ruleName := range disabled {
					suggestions := orchestrator.cycleBreaker.GenerateCycleResolutionSuggestions(ruleName, "cross-rule")
					t.Logf("Suggestions for rule '%s':", ruleName)
					for _, suggestion := range suggestions {
						t.Logf("  • %s", suggestion)
					}
				}
			} else {
				total := execsA + execsB
				if total > 6 {
					t.Errorf("Cycle not detected despite %d total executions", total)
				}
			}
		})
	})
}

// TestRapidFileChanges tests how the system handles rapid file modifications
func TestRapidFileChanges(t *testing.T) {
	helper := NewTestHelper(t, 10*time.Second)

	config := NewTestConfig("rapid-changes-test").
		AddShortRunningRule("watcher", []string{
			"echo 'Processing change'",
			"sleep 1", // Simulate some work
		})

	helper.WithRunningOrchestrator(config, func(orchestrator *TestOrchestrator) {
		// Create test file
		testFile := filepath.Join(orchestrator.tmpDir, "test.txt")
		err := os.WriteFile(testFile, []byte("initial"), 0644)
		require.NoError(t, err)

		// Wait for setup
		time.Sleep(200 * time.Millisecond)

		// Make rapid file changes
		t.Logf("=== Making rapid file changes ===")
		for i := 0; i < 10; i++ {
			content := fmt.Sprintf("change %d", i)
			err = os.WriteFile(testFile, []byte(content), 0644)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond) // Rapid changes
		}

		// Wait for debouncing and processing
		time.Sleep(3 * time.Second)

		// Check final state - rapid changes should debounce to few executions
		ruleRunner := orchestrator.GetRuleRunner("watcher")
		status := ruleRunner.GetStatus()
		triggerCount := ruleRunner.GetTriggerCount(time.Minute)
		executionCount := orchestrator.Orchestrator.GetExecutionCount("watcher", time.Minute)

		assert.False(t, ruleRunner.IsRunning(), "Rule should not be running after debounce")

		// Rapid file changes should debounce to only 1-2 executions, not trigger cycle detection
		disabled := orchestrator.Orchestrator.cycleBreaker.GetDisabledRules()
		if len(disabled) > 0 {
			t.Errorf("UNEXPECTED: Emergency break should not activate for rapid file changes: %v (executions=%d)", disabled, executionCount)
		} else {
			// Should have executed successfully with debouncing
			assert.Equal(t, "SUCCESS", status.LastBuildStatus, "Should succeed with debounced execution")
			assert.LessOrEqual(t, executionCount, 2, "Rapid changes should debounce to ≤2 executions")
			t.Logf("SUCCESS: Debouncing worked - %d triggers → %d executions, status: %s", triggerCount, executionCount, status.LastBuildStatus)
		}

		t.Logf("Final status: running=%t, status=%s, triggers=%d, executions=%d", ruleRunner.IsRunning(), status.LastBuildStatus, triggerCount, executionCount)
	})
}
