package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/panyam/devloop/testhelpers"
)

// TestAgent_BasicFunctionality tests basic agent functionality without gateway.
// Verifies that an agent can start, load configuration, and handle basic operations.
func TestAgent_BasicFunctionality(t *testing.T) {
	testhelpers.WithTestContext(t, 10*time.Second, func(t *testing.T, tmpDir string) {
		// Create test config
		agentConfigPath := filepath.Join(tmpDir, ".devloop.yaml")
		agentConfigContent := `
settings:
  project_id: "test-agent"
  prefix_logs: true

rules:
  - name: "test-rule"
    commands:
      - "echo 'test command'"
    watch:
      - action: "include"
        patterns:
          - "**/*.txt"
`
		err := os.WriteFile(agentConfigPath, []byte(agentConfigContent), 0644)
		require.NoError(t, err)

		// Start agent
		agent, err := NewOrchestratorForTesting(agentConfigPath)
		require.NoError(t, err)

		// Test basic functionality (without starting orchestrator)
		config := agent.GetConfig()
		require.NotNil(t, config)
		require.Equal(t, "test-agent", config.Settings.ProjectId)
		require.Len(t, config.Rules, 1)
		require.Equal(t, "test-rule", config.Rules[0].Name)

		// Test watched paths (should work without starting)
		paths := agent.GetWatchedPaths()
		require.NotEmpty(t, paths)
	})
}
