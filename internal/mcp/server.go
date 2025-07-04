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
	mcpServer         *server.MCPServer
	streamableServer  *server.StreamableHTTPServer
	orchestrator      gateway.Orchestrator
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

	// Create StreamableHTTP server for HTTP transport (MCP 2025-03-26 spec)
	// Use stateless mode to avoid sessionId requirement for better compatibility
	m.streamableServer = server.NewStreamableHTTPServer(m.mcpServer,
		server.WithStateLess(true),           // Stateless mode - no sessionId required
		server.WithEndpointPath("/mcp"),      // Single endpoint for all MCP operations
	)

	utils.LogMCP("MCP handler created successfully")
	return m.streamableServer, nil
}

// Stop cleans up MCP resources (no longer manages HTTP server)
func (m *MCPService) Stop() {
	if m.mcpServer != nil {
		utils.LogMCP("Shutting down MCP service...")
		// MCP server cleanup is handled by the containing HTTP server
		// No separate HTTP server to shutdown since we're just a handler now
	}
}
