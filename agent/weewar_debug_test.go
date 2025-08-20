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

func TestWeeWarPatternMatching(t *testing.T) {
	testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Reproduce the exact weewar pattern structure
		configContent := `
settings:
  project_id: "weewar-debug"
  verbose: true

rules:
  - name: "betests"
    skip_run_on_init: true
    watch:
      - action: "include"
        patterns:
          - "web/server/*.go"
      - action: "exclude" 
        patterns:
          - "**/*.log"
          - "web/**/*"
          - "web"
      - action: "include"
        patterns:
          - "lib/**/*.go"
          - "cmd/**/*.go"
          - "services/**/*.go"
    commands:
      - "echo 'betests triggered by file change'"
`
		
		// Create directory structure matching weewar
		webServerDir := filepath.Join(tmpDir, "web", "server")
		libDir := filepath.Join(tmpDir, "lib")
		
		err := os.MkdirAll(webServerDir, 0755)
		require.NoError(t, err)
		
		err = os.MkdirAll(libDir, 0755)
		require.NoError(t, err)
		
		// Create test files
		webServerFile := filepath.Join(webServerDir, "main.go")
		libFile := filepath.Join(libDir, "utils.go")
		
		err = os.WriteFile(webServerFile, []byte("package main\n"), 0644)
		require.NoError(t, err)
		
		err = os.WriteFile(libFile, []byte("package lib\n"), 0644)
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
		
		// Test 1: Check that web/server directory is being watched
		ruleRunner := orchestrator.GetRuleRunner("betests")
		require.NotNil(t, ruleRunner)
		
		// Get watcher info (we need to check if web/server is actually being watched)
		t.Logf("Rule betests watcher status - rule exists: %t", ruleRunner != nil)
		
		// Test 2: Manual trigger to verify rule works
		err = orchestrator.TriggerRule("betests")
		require.NoError(t, err)
		
		// Wait for completion
		time.Sleep(2 * time.Second)
		
		status := ruleRunner.GetStatus()
		t.Logf("After manual trigger - running: %t, status: %s", ruleRunner.IsRunning(), status.LastBuildStatus)
		assert.Equal(t, "SUCCESS", status.LastBuildStatus, "Manual trigger should work")
		
		// Test 3: File change to web/server/main.go (should trigger due to FIFO)
		t.Logf("Modifying web/server/main.go...")
		
		// Modify the file
		err = os.WriteFile(webServerFile, []byte("package main\n\n// Modified\n"), 0644)
		require.NoError(t, err)
		
		// Wait for file change detection and processing
		time.Sleep(3 * time.Second)
		
		// Check if rule was triggered by file change
		newStatus := ruleRunner.GetStatus()
		t.Logf("After file change - last build time changed: %t, status: %s", 
			newStatus.LastBuildTime.AsTime().After(status.LastBuildTime.AsTime()),
			newStatus.LastBuildStatus)
		
		// The key test: was the rule triggered by the file change?
		if !newStatus.LastBuildTime.AsTime().After(status.LastBuildTime.AsTime()) {
			t.Error("File change to web/server/main.go should have triggered betests rule")
			t.Logf("Original build time: %v", status.LastBuildTime.AsTime())
			t.Logf("New build time: %v", newStatus.LastBuildTime.AsTime())
		}
		
		// Test 4: File change to lib/utils.go (should also trigger)
		t.Logf("Modifying lib/utils.go...")
		
		beforeLibChange := newStatus.LastBuildTime.AsTime()
		
		err = os.WriteFile(libFile, []byte("package lib\n\n// Modified\n"), 0644)
		require.NoError(t, err)
		
		time.Sleep(3 * time.Second)
		
		finalStatus := ruleRunner.GetStatus()
		t.Logf("After lib change - triggered: %t, status: %s",
			finalStatus.LastBuildTime.AsTime().After(beforeLibChange),
			finalStatus.LastBuildStatus)
		
		if !finalStatus.LastBuildTime.AsTime().After(beforeLibChange) {
			t.Error("File change to lib/utils.go should have triggered betests rule")
		}
	})
}
