package mcp

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"github.com/panyam/devloop/gateway"
	v1mcp "github.com/panyam/devloop/gen/go/protos/devloop/v1/v1mcp"
	"github.com/panyam/devloop/utils"
)

// MCPService manages the MCP server for devloop
type MCPService struct {
	mcpServer    *server.MCPServer
	sseServer    *server.SSEServer
	orchestrator gateway.Orchestrator
}

// NewMCPService creates a new MCP service instance
func NewMCPService(orchestrator gateway.Orchestrator) *MCPService {
	return &MCPService{
		orchestrator: orchestrator,
	}
}

// CreateHandler initializes MCP server and returns HTTP handler
func (m *MCPService) CreateHandler() (http.Handler, error) {
	utils.LogMCP("Creating MCP handler")

	// Create MCP server
	m.mcpServer = server.NewMCPServer("devloop", "1.0.0")

	// Create a selective gateway adapter that only exposes essential tools
	gatewayAdapter := &SelectiveGatewayAdapter{orchestrator: m.orchestrator}

	// Register auto-generated MCP tools from protobuf definitions
	v1mcp.RegisterGatewayClientServiceHandler(m.mcpServer, gatewayAdapter)

	// Create SSE server for HTTP transport
	m.sseServer = server.NewSSEServer(m.mcpServer)

	utils.LogMCP("MCP handler created successfully")
	return m.sseServer, nil
}

// Stop cleans up MCP resources (no longer manages HTTP server)
func (m *MCPService) Stop() {
	if m.mcpServer != nil {
		utils.LogMCP("Shutting down MCP service...")
		// MCP server cleanup is handled by the containing HTTP server
		// No separate HTTP server to shutdown since we're just a handler now
	}
}
