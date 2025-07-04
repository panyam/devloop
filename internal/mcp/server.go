package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/panyam/devloop/gateway"
	v1mcp "github.com/panyam/devloop/gen/go/protos/devloop/v1/v1mcp"
	"github.com/panyam/devloop/utils"
)

// MCPService manages the MCP server for devloop
type MCPService struct {
	mcpServer    *server.MCPServer
	sseServer    *server.SSEServer
	httpServer   *http.Server
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
	utils.LogMCP("Starting MCP service - port: %d", m.port)

	// Create MCP server
	m.mcpServer = server.NewMCPServer("devloop", "1.0.0")

	// Create a selective gateway adapter that only exposes essential tools
	gatewayAdapter := &SelectiveGatewayAdapter{orchestrator: m.orchestrator}

	// Register auto-generated MCP tools from protobuf definitions
	v1mcp.RegisterGatewayClientServiceHandler(m.mcpServer, gatewayAdapter)

	// Start MCP server via HTTP/SSE transport only
	// HTTP enables multiple clients and preserves devloop's terminal output
	if m.port > 0 {
		utils.LogMCP("Port is > 0, starting HTTP server...")
		// Create SSE server for HTTP transport
		m.sseServer = server.NewSSEServer(m.mcpServer)

		// Create HTTP server
		m.httpServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", m.port),
			Handler: m.sseServer,
		}

		go func() {
			utils.LogMCP("MCP server starting on HTTP port %d (SSE transport)", m.port)
			utils.LogMCP("MCP endpoints available at:")
			utils.LogMCP("  SSE: http://localhost:%d/sse", m.port)
			utils.LogMCP("  Message: http://localhost:%d/message", m.port)

			if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[mcp] MCP HTTP server failed: %v", err)
			}
		}()
	} else {
		utils.LogMCP("Port is 0 or negative (%d), MCP server disabled (HTTP-only mode requires valid port)", m.port)
	}

	return nil
}

// Stop gracefully shuts down the MCP server
func (m *MCPService) Stop() {
	if m.mcpServer != nil {
		utils.LogMCP("Shutting down MCP server...")

		// Shutdown HTTP server if running
		if m.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.httpServer.Shutdown(ctx); err != nil {
				log.Printf("[mcp] Error shutting down HTTP server: %v", err)
			}
		}

		// Only HTTP server to shutdown in HTTP-only mode
	}
}
