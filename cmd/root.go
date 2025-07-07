package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath  string
	verbose     bool
	httpPort    int
	grpcPort    int
	gatewayAddr string
	enableMCP   bool
	autoPorts   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "devloop",
	Short: "A versatile development automation tool",
	Long: `Devloop is a versatile tool designed to streamline your inner development loop,
particularly within Multi-Component Projects. It combines functionalities inspired
by live-reloading tools like air (for Go) and build automation tools like make,
focusing on simple, configuration-driven orchestration of tasks based on file system changes.

By default, devloop starts the agent/server when no subcommands are specified.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: start the server
		serverCmd.Run(cmd, args)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// GetRootCommand returns the root command for external use
func GetRootCommand() *cobra.Command {
	return rootCmd
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().IntVar(&httpPort, "http-port", 9999, "Port for the HTTP gateway server (-1 to disable)")
	rootCmd.PersistentFlags().IntVar(&grpcPort, "grpc-port", 5555, "Port for the gRPC server (-1 to disable)")
	rootCmd.PersistentFlags().StringVar(&gatewayAddr, "gateway", "", "Host and port of the gateway to connect to")
	rootCmd.PersistentFlags().BoolVar(&enableMCP, "enable-mcp", true, "Enable MCP server for AI tool integration")
	rootCmd.PersistentFlags().BoolVar(&autoPorts, "auto-ports", false, "Automatically find available ports if specified ports are in use")
}

// getGlobalLogger returns a logger with consistent configuration
func getGlobalLogger() *log.Logger {
	return log.New(os.Stdout, "[devloop] ", log.LstdFlags)
}
