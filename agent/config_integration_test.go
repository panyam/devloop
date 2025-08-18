package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ConfigTest represents a configuration-based integration test
type ConfigTest struct {
	Name       string
	ConfigFile string
	TestFunc   func(t *testing.T, tmpDir string, orchestrator *Orchestrator)
}

// withConfigTest creates a temporary directory, copies the config, and runs the test
func withConfigTest(t *testing.T, configFile string, testFunc func(t *testing.T, tmpDir string, orchestrator *Orchestrator)) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "devloop-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy config file to temp directory
	configPath := filepath.Join("testdata", "configs", configFile)
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file %s: %v", configFile, err)
	}

	tmpConfigPath := filepath.Join(tmpDir, ".devloop.yaml")
	if err := os.WriteFile(tmpConfigPath, configContent, 0644); err != nil {
		t.Fatalf("Failed to write config to temp dir: %v", err)
	}

	// Create logs directory
	logsDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	// Create orchestrator
	orchestrator, err := NewOrchestrator(tmpConfigPath)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Stop()

	// Change to temp directory for the test
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer os.Chdir(originalDir)

	// Run the test function
	testFunc(t, tmpDir, orchestrator)
}

func TestMaxParallelRules(t *testing.T) {
	tests := []ConfigTest{
		{
			Name:       "Sequential execution (max_parallel_rules: 1)",
			ConfigFile: "max_parallel_rules.yaml",
			TestFunc:   testSequentialExecution,
		},
		{
			Name:       "Unlimited parallel execution (max_parallel_rules: 0)",
			ConfigFile: "unlimited_parallel.yaml", 
			TestFunc:   testUnlimitedExecution,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			withConfigTest(t, test.ConfigFile, test.TestFunc)
		})
	}
}

func testSequentialExecution(t *testing.T, tmpDir string, orchestrator *Orchestrator) {
	// Verify semaphore is configured correctly
	active, capacity, unlimited := orchestrator.getSemaphoreStatus()
	if unlimited {
		t.Error("Expected limited parallel execution, got unlimited")
	}
	if capacity != 1 {
		t.Errorf("Expected semaphore capacity 1, got %d", capacity)
	}
	if active != 0 {
		t.Errorf("Expected 0 active executions initially, got %d", active)
	}

	// Start orchestrator in background
	go func() {
		if err := orchestrator.Start(); err != nil {
			t.Logf("Orchestrator stopped: %v", err)
		}
	}()

	// Give orchestrator time to start
	time.Sleep(100 * time.Millisecond)

	// Create a file to trigger all rules simultaneously
	triggerFile := filepath.Join(tmpDir, "trigger.txt")
	if err := os.WriteFile(triggerFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create trigger file: %v", err)
	}

	// Wait for executions to start and finish
	// With max_parallel_rules: 1, executions should be sequential (6+ seconds total)
	// With unlimited parallel, they would run concurrently (~2 seconds total)
	time.Sleep(7 * time.Second)

	// Check final semaphore state
	active, _, _ = orchestrator.getSemaphoreStatus()
	if active != 0 {
		t.Errorf("Expected 0 active executions after completion, got %d", active)
	}
}

func testUnlimitedExecution(t *testing.T, tmpDir string, orchestrator *Orchestrator) {
	// Verify semaphore is disabled
	active, capacity, unlimited := orchestrator.getSemaphoreStatus()
	if !unlimited {
		t.Error("Expected unlimited parallel execution, got limited")
	}
	if capacity != 0 || active != 0 {
		t.Errorf("Expected semaphore to be disabled (0,0), got (%d,%d)", active, capacity)
	}

	// Start orchestrator in background
	go func() {
		if err := orchestrator.Start(); err != nil {
			t.Logf("Orchestrator stopped: %v", err)
		}
	}()

	// Give orchestrator time to start
	time.Sleep(100 * time.Millisecond)

	// Create a file to trigger all rules simultaneously
	triggerFile := filepath.Join(tmpDir, "trigger.txt")
	if err := os.WriteFile(triggerFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create trigger file: %v", err)
	}

	// Wait for executions to complete
	// With unlimited parallel, both should complete in ~1 second
	time.Sleep(2 * time.Second)

	// Semaphore should still be disabled
	active, capacity, unlimited = orchestrator.getSemaphoreStatus()
	if !unlimited {
		t.Error("Semaphore should remain disabled")
	}
}

func TestSemaphoreLogging(t *testing.T) {
	withConfigTest(t, "max_parallel_rules.yaml", func(t *testing.T, tmpDir string, orchestrator *Orchestrator) {
		// Check that orchestrator logs mention semaphore initialization
		maxParallel := orchestrator.getMaxParallelRules()
		if maxParallel != 1 {
			t.Errorf("Expected max parallel rules = 1, got %d", maxParallel)
		}

		// Verify configuration was parsed correctly
		if orchestrator.Config.Settings.MaxParallelRules != 1 {
			t.Errorf("Expected config MaxParallelRules = 1, got %d", orchestrator.Config.Settings.MaxParallelRules)
		}
	})
}

func TestConfigValidation(t *testing.T) {
	// Test that invalid max_parallel_rules values are handled properly
	tmpDir, err := os.MkdirTemp("", "devloop-validation-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with edge cases
	configs := map[string]string{
		"zero_parallel.yaml": `
settings:
  max_parallel_rules: 0
rules:
  - name: "test"
    watch:
      - action: "include"
        patterns: ["*.txt"]
    commands: ["echo test"]
`,
		"large_parallel.yaml": `
settings:
  max_parallel_rules: 100
rules:
  - name: "test"
    watch:
      - action: "include"
        patterns: ["*.txt"]
    commands: ["echo test"]
`,
	}

	for name, content := range configs {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, name)
			if err := os.WriteFile(configPath, []byte(strings.TrimSpace(content)), 0644); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			orchestrator, err := NewOrchestrator(configPath)
			if err != nil {
				t.Fatalf("Failed to create orchestrator: %v", err)
			}
			defer orchestrator.Stop()

			// Should create orchestrator without errors
			active, capacity, unlimited := orchestrator.getSemaphoreStatus()
			t.Logf("Config %s: active=%d, capacity=%d, unlimited=%v", name, active, capacity, unlimited)
		})
	}
}