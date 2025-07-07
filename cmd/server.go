package cmd

import (
	"fmt"
	"os"

	"github.com/panyam/devloop/server"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the devloop server/agent",
	Long: `Start the devloop server/agent to watch files and execute rules.

This is the default mode when no subcommand is specified. The server will:
- Load configuration from the specified config file
- Start file watching for pattern matching
- Execute commands when matching files change
- Optionally start gRPC and HTTP servers for API access
- Optionally enable MCP (Model Context Protocol) for AI integration

Examples:
  devloop server                    # Start with default settings
  devloop server -c myconfig.yaml   # Start with custom config
  devloop server --grpc-port 6000   # Start with custom gRPC port
  devloop server --auto-ports       # Auto-discover available ports`,
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

func runServer() {
	config := &server.ServerConfig{
		ConfigPath:  configPath,
		Verbose:     verbose,
		HTTPPort:    httpPort,
		GRPCPort:    grpcPort,
		GatewayAddr: gatewayAddr,
		EnableMCP:   enableMCP,
		AutoPorts:   autoPorts,
	}

	srv, err := server.NewServer(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create server: %v\n", err)
		os.Exit(1)
	}

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Server failed: %v\n", err)
		os.Exit(1)
	}
}