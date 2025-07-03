package mcp

import (
	"log"

	"github.com/mark3labs/mcp-go/server"
	"github.com/panyam/devloop/gateway"
	v1mcp "github.com/panyam/devloop/gen/go/protos/devloop/v1/v1mcp"
)

// MCPService manages the MCP server for devloop
type MCPService struct {
	mcpServer    *server.MCPServer
	orchestrator gateway.Orchestrator
	port         int
}

// NewMCPService creates a new MCP service instance
func NewMCPService(orchestrator gateway.Orchestrator, port int) *MCPService {
	return &MCPService{
		orchestrator: orchestrator,
		port:         port,
	}
}

// Start initializes and starts the MCP server
func (m *MCPService) Start() error {
	// Create MCP server
	m.mcpServer = server.NewMCPServer("devloop", "1.0.0")

	// Create a selective gateway adapter that only exposes essential tools
	gatewayAdapter := &SelectiveGatewayAdapter{orchestrator: m.orchestrator}

	// Register auto-generated MCP tools from protobuf definitions
	v1mcp.RegisterGatewayClientServiceHandler(m.mcpServer, gatewayAdapter)

	// Start MCP server via stdio (MCP standard)
	go func() {
		log.Printf("[mcp] MCP server starting on stdio")
		if err := server.ServeStdio(m.mcpServer); err != nil {
			log.Printf("[mcp] MCP server failed: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the MCP server
func (m *MCPService) Stop() {
	if m.mcpServer != nil {
		log.Println("[mcp] Shutting down MCP server...")
		// The mcp-go server doesn't seem to have a graceful shutdown method
		// so we'll rely on the listener being closed
	}
}