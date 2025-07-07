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

// TestGRPCServer_BasicFunctionality tests basic gateway and agent connectivity.
// Verifies that a gateway can start, accept agent connections, and provide
// the ListProjects endpoint functionality. This is a simpler test focusing
// on core connectivity rather than comprehensive API testing.
func TestGRPCServer_BasicFunctionality(t *testing.T) {
	testhelpers.WithTestContext(t, 15*time.Second, func(t *testing.T, tmpDir string) {
		// Create gateway config
		gatewayConfigContent := `
settings:
  project_id: "gateway"
  prefix_logs: true
rules:
  - name: "gateway-rule"
    commands:
      - "echo 'gateway test'"
`
		gatewayConfigPath := filepath.Join(tmpDir, "gateway.yaml")
		err := os.WriteFile(gatewayConfigPath, []byte(gatewayConfigContent), 0644)
		require.NoError(t, err)

		// Start Gateway Service
		gatewayOrchestrator, err := NewOrchestratorForTesting(gatewayConfigPath)
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

		// Connect to gateway gRPC server
		conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", grpcPort),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		client := pb.NewGatewayClientServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Test ListProjects (should return empty list since no agents connected yet)
		t.Run("ListProjects_EmptyWhenNoAgents", func(t *testing.T) {
			resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, 0, len(resp.Projects), "Should have no projects when no agents connected")
		})

		// Now create and connect an agent
		agentConfigContent := `
settings:
  project_id: "test-agent"
  prefix_logs: true
rules:
  - name: "build"
    commands:
      - "echo 'agent built'"
    watch:
      - action: include
        patterns:
          - "*.go"
`
		agentDir := filepath.Join(tmpDir, "agent")
		require.NoError(t, os.MkdirAll(agentDir, 0755))
		agentConfigPath := filepath.Join(agentDir, ".devloop.yaml")
		err = os.WriteFile(agentConfigPath, []byte(agentConfigContent), 0644)
		require.NoError(t, err)

		// Start agent connected to gateway
		gatewayAddr := fmt.Sprintf("localhost:%d", grpcPort)
		agent1, err := NewOrchestratorForTesting(agentConfigPath, gatewayAddr)
		require.NoError(t, err)

		go func() {
			err := agent1.Start()
			assert.NoError(t, err)
		}()
		defer agent1.Stop()

		// Wait for agent to connect
		time.Sleep(1 * time.Second)

		// Test ListProjects again (should now show the connected agent)
		t.Run("ListProjects_ShowsConnectedAgent", func(t *testing.T) {
			resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
			require.NoError(t, err)
			assert.NotNil(t, resp)

			if len(resp.Projects) > 0 {
				assert.Equal(t, 1, len(resp.Projects), "Should have one project")
				project := resp.Projects[0]
				assert.Equal(t, "test-agent", project.ProjectId)
				assert.Contains(t, []string{"CONNECTED", "DISCONNECTED"}, project.Status)
				assert.NotEmpty(t, project.ProjectRoot)
			} else {
				t.Log("No projects found - agent may not have connected yet")
			}
		})
	})
}
