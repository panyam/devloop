package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/devloop/testhelpers"
	"github.com/stretchr/testify/require"
)

// TestOrchestrator represents a test orchestrator with cleanup
type TestOrchestrator struct {
	*Orchestrator
	tmpDir     string
	configPath string
	t          *testing.T
}

// Stop cleans up the test orchestrator and temporary directory
func (to *TestOrchestrator) Stop() {
	if to.Orchestrator != nil {
		to.Orchestrator.Stop()
	}
	if to.tmpDir != "" {
		os.RemoveAll(to.tmpDir)
	}
}

// CreateRule creates a new rule for testing
func (to *TestOrchestrator) CreateRule(name string, lro bool, commands []string) {
	// This could be used to dynamically add rules in tests
	// Implementation would modify the orchestrator's config
}

// TriggerRule manually triggers a rule for testing
func (to *TestOrchestrator) TriggerRule(ruleName string) error {
	return to.Orchestrator.TriggerRule(ruleName)
}

// WaitForRuleCompletion waits for a rule to complete with timeout
func (to *TestOrchestrator) WaitForRuleCompletion(ruleName string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		if ruleRunner := to.GetRuleRunner(ruleName); ruleRunner != nil {
			if !ruleRunner.IsRunning() {
				status := ruleRunner.GetStatus()
				if status.LastBuildStatus == "SUCCESS" || status.LastBuildStatus == "FAILED" {
					return nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("rule %q did not complete within %v", ruleName, timeout)
}

// TestConfig represents a test configuration builder
type TestConfig struct {
	ProjectID string
	Verbose   bool
	Rules     []TestRule
	Settings  map[string]interface{}
}

// TestRule represents a rule configuration for testing
type TestRule struct {
	Name           string
	LRO            bool
	SkipRunOnInit  bool
	Commands       []string
	WatchPatterns  []string
	ExcludePatterns []string
	Env            map[string]string
}

// NewTestConfig creates a new test configuration builder
func NewTestConfig(projectID string) *TestConfig {
	return &TestConfig{
		ProjectID: projectID,
		Verbose:   true, // Default to verbose for better test debugging
		Rules:     []TestRule{},
		Settings:  make(map[string]interface{}),
	}
}

// AddRule adds a rule to the test configuration
func (tc *TestConfig) AddRule(rule TestRule) *TestConfig {
	tc.Rules = append(tc.Rules, rule)
	return tc
}

// AddShortRunningRule adds a typical short-running rule
func (tc *TestConfig) AddShortRunningRule(name string, commands []string) *TestConfig {
	return tc.AddRule(TestRule{
		Name:          name,
		LRO:           false,
		SkipRunOnInit: true,
		Commands:      commands,
		WatchPatterns: []string{"*.txt"}, // Safe default pattern
	})
}

// AddLRORule adds a typical long-running rule
func (tc *TestConfig) AddLRORule(name string, commands []string) *TestConfig {
	return tc.AddRule(TestRule{
		Name:          name,
		LRO:           true,
		SkipRunOnInit: true,
		Commands:      commands,
		WatchPatterns: []string{"*.txt"}, // Safe default pattern
	})
}

// WithMaxWorkers sets the max parallel workers setting
func (tc *TestConfig) WithMaxWorkers(max int) *TestConfig {
	tc.Settings["max_parallel_rules"] = max
	return tc
}

// ToYAML converts the test config to YAML string
func (tc *TestConfig) ToYAML() string {
	yaml := fmt.Sprintf("settings:\n  project_id: %q\n", tc.ProjectID)
	
	if tc.Verbose {
		yaml += "  verbose: true\n"
	}
	
	// Add custom settings
	for key, value := range tc.Settings {
		yaml += fmt.Sprintf("  %s: %v\n", key, value)
	}
	
	yaml += "\nrules:\n"
	
	for _, rule := range tc.Rules {
		yaml += fmt.Sprintf("  - name: %q\n", rule.Name)
		yaml += fmt.Sprintf("    lro: %t\n", rule.LRO)
		
		if rule.SkipRunOnInit {
			yaml += "    skip_run_on_init: true\n"
		}
		
		// Add environment variables
		if len(rule.Env) > 0 {
			yaml += "    env:\n"
			for key, value := range rule.Env {
				yaml += fmt.Sprintf("      %s: %q\n", key, value)
			}
		}
		
		// Add commands
		yaml += "    commands:\n"
		for _, cmd := range rule.Commands {
			yaml += fmt.Sprintf("      - %q\n", cmd)
		}
		
		// Add watch patterns
		yaml += "    watch:\n"
		
		// Add exclude patterns first if any
		if len(rule.ExcludePatterns) > 0 {
			yaml += "      - action: exclude\n        patterns:\n"
			for _, pattern := range rule.ExcludePatterns {
				yaml += fmt.Sprintf("          - %q\n", pattern)
			}
		}
		
		// Add include patterns
		if len(rule.WatchPatterns) > 0 {
			yaml += "      - action: include\n        patterns:\n"
			for _, pattern := range rule.WatchPatterns {
				yaml += fmt.Sprintf("          - %q\n", pattern)
			}
		}
		
		yaml += "\n"
	}
	
	return yaml
}

// TestHelper provides utilities for orchestrator testing
type TestHelper struct {
	t       *testing.T
	timeout time.Duration
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T, timeout time.Duration) *TestHelper {
	return &TestHelper{t: t, timeout: timeout}
}

// WithOrchestrator creates an orchestrator from config and runs test function
func (th *TestHelper) WithOrchestrator(config *TestConfig, testFunc func(*TestOrchestrator)) {
	testhelpers.WithTestContext(th.t, th.timeout, func(t *testing.T, tmpDir string) {
		// Write config to temp directory
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		configContent := config.ToYAML()
		
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err, "Failed to write test config")

		// Create logs directory
		logsDir := filepath.Join(tmpDir, "logs")
		err = os.MkdirAll(logsDir, 0755)
		require.NoError(t, err, "Failed to create logs directory")

		// Create orchestrator
		orchestrator, err := NewOrchestrator(configPath)
		require.NoError(t, err, "Failed to create orchestrator")

		testOrchestrator := &TestOrchestrator{
			Orchestrator: orchestrator,
			tmpDir:       tmpDir,
			configPath:   configPath,
			t:            t,
		}
		defer testOrchestrator.Stop()

		// Run the test function
		testFunc(testOrchestrator)
	})
}

// WithRunningOrchestrator creates and starts an orchestrator for integration testing
func (th *TestHelper) WithRunningOrchestrator(config *TestConfig, testFunc func(*TestOrchestrator)) {
	th.WithOrchestrator(config, func(to *TestOrchestrator) {
		// Start orchestrator in background
		go func() {
			if err := to.Start(); err != nil {
				to.t.Logf("Orchestrator start error: %v", err)
			}
		}()
		
		// Give time for components to start
		time.Sleep(200 * time.Millisecond)
		
		testFunc(to)
	})
}

// AssertRuleStatus asserts a rule has the expected status
func (th *TestHelper) AssertRuleStatus(orchestrator *Orchestrator, ruleName string, expectedRunning bool, expectedStatus string) {
	ruleRunner := orchestrator.GetRuleRunner(ruleName)
	require.NotNil(th.t, ruleRunner, "Rule %q should exist", ruleName)
	
	status := ruleRunner.GetStatus()
	require.Equal(th.t, expectedRunning, ruleRunner.IsRunning(), 
		"Rule %q running state. Status: %s", ruleName, status.LastBuildStatus)
	require.Equal(th.t, expectedStatus, status.LastBuildStatus,
		"Rule %q status", ruleName)
}

// AssertLROProcessCount asserts the expected number of LRO processes
func (th *TestHelper) AssertLROProcessCount(lroManager *LROManager, expectedCount int) {
	processes := lroManager.GetRunningProcesses()
	require.Len(th.t, processes, expectedCount, 
		"Expected %d LRO processes, got %d: %v", expectedCount, len(processes), processes)
}
