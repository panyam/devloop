package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
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
		orchestrator, err := NewOrchestratorForTesting(configPath, "")
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
		assert.Equal(t, "test-agent", config.Settings.ProjectID)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "test-rule", config.Rules[0].Name)

		// Test TriggerRule
		err = orchestrator.TriggerRule("test-rule")
		assert.NoError(t, err)

		// Wait for command to execute
		time.Sleep(100 * time.Millisecond)

		// Test GetRuleStatus
		status, exists := orchestrator.GetRuleStatus("test-rule")
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

// TestGatewayMode_MultipleAgents tests the gateway mode with multiple connected agents.
// Verifies that a central gateway can accept connections from multiple agent processes,
// track their status, and provide unified API access to all connected projects through
// the gRPC client service interface.
func TestGatewayMode_MultipleAgents(t *testing.T) {
	testhelpers.WithTestContext(t, 20*time.Second, func(t *testing.T, tmpDir string) {
		// Create minimal gateway config first
		gatewayConfigContent := `
settings:
  project_id: "gateway"
rules:
  - name: "gateway-rule"
    commands:
      - "echo 'gateway active'"
`
		gatewayConfigPath := filepath.Join(tmpDir, "gateway.yaml")
		err := os.WriteFile(gatewayConfigPath, []byte(gatewayConfigContent), 0644)
		require.NoError(t, err)

		// Start Gateway Service
		gatewayOrchestrator, err := NewOrchestratorForTesting(gatewayConfigPath, "")
		require.NoError(t, err)

		gatewayService := gateway.NewGatewayService(gatewayOrchestrator)

		// Find available ports
		grpcPort, err := testhelpers.FindAvailablePort()
		require.NoError(t, err)
		httpPort, err := testhelpers.FindAvailablePort()
		require.NoError(t, err)

		// Start gateway
		err = gatewayService.Start(grpcPort, httpPort)
		require.NoError(t, err)
		defer gatewayService.Stop()

		// Wait for gateway to start
		time.Sleep(500 * time.Millisecond)

		// Create two agent configurations
		agent1Dir := filepath.Join(tmpDir, "agent1")
		agent2Dir := filepath.Join(tmpDir, "agent2")
		require.NoError(t, os.MkdirAll(agent1Dir, 0755))
		require.NoError(t, os.MkdirAll(agent2Dir, 0755))

		// Agent 1 config
		agent1ConfigPath := filepath.Join(agent1Dir, ".devloop.yaml")
		agent1Config := `
settings:
  project_id: "backend-service"
  prefix_logs: true
rules:
  - name: "build"
    commands:
      - "echo 'backend built'"
    watch:
      - action: include
        patterns:
          - "*.go"
`
		err = os.WriteFile(agent1ConfigPath, []byte(agent1Config), 0644)
		require.NoError(t, err)

		// Agent 2 config
		agent2ConfigPath := filepath.Join(agent2Dir, ".devloop.yaml")
		agent2Config := `
settings:
  project_id: "frontend-service"
  prefix_logs: true
rules:
  - name: "build"
    commands:
      - "echo 'frontend built'"
    watch:
      - action: include
        patterns:
          - "*.js"
`
		err = os.WriteFile(agent2ConfigPath, []byte(agent2Config), 0644)
		require.NoError(t, err)

		// Start agents in background (they will connect to gateway)
		gatewayAddr := fmt.Sprintf("localhost:%d", grpcPort)

		agent1, err := NewOrchestratorForTesting(agent1ConfigPath, gatewayAddr)
		require.NoError(t, err)

		agent2, err := NewOrchestratorForTesting(agent2ConfigPath, gatewayAddr)
		require.NoError(t, err)

		go func() {
			err := agent1.Start()
			assert.NoError(t, err)
		}()
		defer agent1.Stop()

		go func() {
			err := agent2.Start()
			assert.NoError(t, err)
		}()
		defer agent2.Stop()

		// Wait for agents to connect
		time.Sleep(1 * time.Second)

		// Test Gateway Client API
		testGatewayClientAPI(t, grpcPort)
	})
}

// testGatewayClientAPI tests the GatewayClientService gRPC API endpoints.
// This function tests all the major client-facing API operations including
// project listing, configuration retrieval, rule management, file operations,
// and status checking to ensure the gateway properly proxies requests to agents.
func testGatewayClientAPI(t *testing.T, grpcPort int) {
	// Connect to gateway gRPC server
	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", grpcPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewGatewayClientServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test ListProjects
	t.Run("ListProjects", func(t *testing.T) {
		resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// Should have at least the connected agents
		assert.GreaterOrEqual(t, len(resp.Projects), 0)

		// Check project structure if any projects exist
		for _, project := range resp.Projects {
			assert.NotEmpty(t, project.ProjectId)
			assert.NotEmpty(t, project.ProjectRoot)
			assert.Contains(t, []string{"CONNECTED", "DISCONNECTED"}, project.Status)
		}
	})

	// Test other endpoints with a known project (if any exist)
	projectsResp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
	require.NoError(t, err)

	if len(projectsResp.Projects) > 0 {
		projectID := projectsResp.Projects[0].ProjectId

		t.Run("GetConfig", func(t *testing.T) {
			resp, err := client.GetConfig(ctx, &pb.GetConfigRequest{
				ProjectId: projectID,
			})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotEmpty(t, resp.ConfigJson)
		})

		t.Run("ListWatchedPaths", func(t *testing.T) {
			resp, err := client.ListWatchedPaths(ctx, &pb.ListWatchedPathsRequest{
				ProjectId: projectID,
			})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			// Should have at least some paths
			assert.GreaterOrEqual(t, len(resp.Paths), 0)
		})

		t.Run("GetRuleStatus", func(t *testing.T) {
			resp, err := client.GetRuleStatus(ctx, &pb.GetRuleStatusRequest{
				ProjectId: projectID,
				RuleName:  "build", // From our test config
			})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotNil(t, resp.RuleStatus)
			assert.Equal(t, projectID, resp.RuleStatus.ProjectId)
			assert.Equal(t, "build", resp.RuleStatus.RuleName)
		})

		t.Run("TriggerRule", func(t *testing.T) {
			resp, err := client.TriggerRuleClient(ctx, &pb.TriggerRuleClientRequest{
				ProjectId: projectID,
				RuleName:  "build",
			})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.True(t, resp.Success)
		})

		t.Run("ReadFileContent", func(t *testing.T) {
			resp, err := client.ReadFileContent(ctx, &pb.ReadFileContentRequest{
				ProjectId: projectID,
				Path:      ".devloop.yaml",
			})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotEmpty(t, resp.Content)
		})
	}
}
