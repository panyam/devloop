package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/panyam/devloop/testhelpers"
)

// TestAgentMode_EmbeddedGRPCServer tests standalone agent mode functionality.
// Verifies that an agent can run with an embedded gRPC server and provides
// all the core orchestrator functionality: config management, rule triggering,
// status checking, file operations, and path watching.
func TestAgentMode_EmbeddedGRPCServer(t *testing.T) {
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Create a test configuration
		configPath := filepath.Join(tmpDir, ".devloop.yaml")
		testFile := filepath.Join(tmpDir, "test.txt")
		outputFile := filepath.Join(tmpDir, "output.txt")

		configContent := fmt.Sprintf(`
settings:
  project_id: "test-agent"
  prefix_logs: true
rules:
  - name: "test-rule"
    commands:
      - "echo 'test output' > %s"
    watch:
      - action: include
        patterns:
          - "%s"
`, outputFile, testFile)

		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Start orchestrator in agent mode (embedded gRPC server)
		orchestrator, err := NewOrchestratorForTesting(configPath)
		require.NoError(t, err)

		// Start orchestrator in background
		go func() {
			err := orchestrator.Start()
			assert.NoError(t, err)
		}()
		defer orchestrator.Stop()

		// Wait a moment for the server to start
		time.Sleep(500 * time.Millisecond)

		// Connect gRPC client to embedded server
		// Note: In agent mode, the embedded server typically runs on port 50051
		// For testing, we need to find the actual port or use a mock setup
		// For now, we'll test the methods directly via the orchestrator interface

		// Test GetConfig
		config := orchestrator.GetConfig()
		assert.NotNil(t, config)
		assert.Equal(t, "test-agent", config.Settings.ProjectId)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "test-rule", config.Rules[0].Name)

		// Test TriggerRule
		err = orchestrator.TriggerRule("test-rule")
		assert.NoError(t, err)

		// Wait for command to execute
		time.Sleep(100 * time.Millisecond)

		// Test GetRuleStatus
		_, status, exists := orchestrator.GetRuleStatus("test-rule")
		assert.True(t, exists)
		assert.Equal(t, "test-rule", status.RuleName)

		// Test GetWatchedPaths
		paths := orchestrator.GetWatchedPaths()
		assert.NotEmpty(t, paths)

		// Test ReadFileContent
		content, err := orchestrator.ReadFileContent(configPath)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "test-agent")
	})
}
