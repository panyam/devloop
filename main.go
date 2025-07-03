// Package main provides the devloop command-line interface for development workflow automation.
//
// Devloop is a versatile tool designed to streamline your inner development loop,
// particularly within Multi-Component Projects. It combines functionalities inspired
// by live-reloading tools like air (for Go) and build automation tools like make,
// focusing on simple, configuration-driven orchestration of tasks based on file system changes.
//
// # Installation
//
//	go install github.com/panyam/devloop@latest
//
// # Quick Start
//
// Create a .devloop.yaml file:
//
//	rules:
//	  - name: "Go Backend Build and Run"
//	    watch:
//	      - action: "include"
//	        patterns:
//	          - "**/*.go"
//	          - "go.mod"
//	          - "go.sum"
//	    commands:
//	      - "go build -o ./bin/server ./cmd/server"
//	      - "./bin/server"
//
// Run devloop:
//
//	devloop -c .devloop.yaml
//
// # Architecture Modes
//
// Devloop operates in three distinct modes:
//
// 1. Standalone Mode (Default): Local file watcher for single project development
// 2. Agent Mode: Standalone with embedded gRPC server for API access
// 3. Gateway Mode: Central hub managing multiple project agents
//
// # Examples
//
// See the examples/ directory for comprehensive real-world examples including:
// - Full-stack web applications
// - Microservices architectures  
// - Data science workflows
// - Docker compose setups
// - Frontend framework development
// - AI/ML development pipelines
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/panyam/devloop/agent"
	"github.com/panyam/devloop/gateway"
	"github.com/panyam/devloop/internal/mcp"
	"github.com/panyam/devloop/utils"
)

//go:embed VERSION
var version string

var verbose bool

func main() {
	var configPath string
	var showVersion bool
	var httpPort int
	var grpcPort int
	var mcpPort int
	var gatewayAddr string
	var mode string
	var enableMCP bool

	// Define global flags
	flag.StringVar(&configPath, "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	flag.BoolVar(&showVersion, "version", false, "Display version information")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.IntVar(&httpPort, "http-port", 8080, "Port for the HTTP gateway server.")
	flag.IntVar(&grpcPort, "grpc-port", 50051, "Port for the gRPC server.")
	flag.IntVar(&mcpPort, "mcp-port", 3000, "Port for the MCP server.")
	flag.BoolVar(&enableMCP, "enable-mcp", true, "Enable MCP server for AI tool integration.")
	flag.StringVar(&gatewayAddr, "gateway-addr", "", "Address of the devloop gateway service (e.g., localhost:50051). If set, devloop will register with the gateway.")
	flag.StringVar(&mode, "mode", "standalone", "Operating mode: standalone, agent, or gateway")

	// Define subcommands
	convertCmd := flag.NewFlagSet("convert", flag.ExitOnError)
	convertInputPath := convertCmd.String("i", ".air.toml", "Path to the .air.toml input file")

	// Parse global flags first
	flag.Parse()

	if showVersion {
		fmt.Printf("devloop version %s\n", version)
		return
	}

	// Check for subcommands
	if len(flag.Args()) > 0 {
		switch flag.Args()[0] {
		case "convert":
			convertCmd.Parse(flag.Args()[1:])
			if err := gateway.ConvertAirToml(*convertInputPath); err != nil {
				log.Fatalf("Error: Failed to convert .air.toml: %v\n", err)
			}
			return
		}
	}

	// Run the orchestrator in the selected mode
	runOrchestrator(configPath, mode, httpPort, grpcPort, mcpPort, gatewayAddr, enableMCP)
}

func runOrchestrator(configPath, mode string, httpPort, grpcPort, mcpPort int, gatewayAddr string, enableMCP bool) {
	orchestrator, err := agent.NewOrchestratorV2(configPath, gatewayAddr)
	if err != nil {
		log.Fatalf("Error: Failed to initialize orchestrator: %v\n", err)
	}
	orchestrator.Verbose = verbose

	// Initialize global logger for consistent formatting across all components
	utils.InitGlobalLogger(
		orchestrator.Config.Settings.PrefixLogs,
		orchestrator.Config.Settings.PrefixMaxLength,
		orchestrator.ColorManager,
	)

	var gatewayService *gateway.GatewayService
	var mcpService *mcp.MCPService

	if mode == "standalone" || mode == "gateway" {
		gatewayService = gateway.NewGatewayService(orchestrator)
		err = gatewayService.Start(grpcPort, httpPort)
		if err != nil {
			log.Fatalf("Error: Failed to start gateway service: %v\n", err)
		}
	}

	// Start MCP server if enabled (can run alongside any core mode)
	if enableMCP {
		mcpService = mcp.NewMCPService(orchestrator, mcpPort)
		err = mcpService.Start()
		if err != nil {
			log.Fatalf("Error: Failed to start MCP service: %v\n", err)
		}
		utils.LogDevloop("MCP server enabled alongside %s mode", mode)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	shutdownComplete := make(chan bool, 1)
	go func() {
		<-sigChan
		utils.LogDevloop("Shutting down...")
		if gatewayService != nil {
			gatewayService.Stop()
		}
		if mcpService != nil {
			mcpService.Stop()
		}
		orchestrator.Stop()
		shutdownComplete <- true
	}()

	if verbose {
		utils.LogDevloop("Config loaded successfully:")
		for _, rule := range orchestrator.Config.Rules {
			fmt.Printf("  Rule Name: %s\n", rule.Name)
			fmt.Printf("    Watch: %v\n", rule.Watch)
			fmt.Printf("    Commands: %v\n", rule.Commands)
		}
	}

	utils.LogDevloop("Starting orchestrator...")
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("Error: Failed to start orchestrator: %v\n", err)
	}

	// Wait for shutdown to complete if it was triggered
	select {
	case <-shutdownComplete:
		utils.LogDevloop("Shutdown complete")
		os.Exit(0)
	default:
		// Normal exit
	}
}
