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
	"net"
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

// findAvailablePort finds an available port starting from the given port number
func findAvailablePort(startPort int, portType string) (int, error) {
	// Try the original port first
	if isPortAvailable(startPort) {
		return startPort, nil
	}

	utils.LogDevloop("Port %d is in use, searching for available %s port...", startPort, portType)

	// Search for next available port (limit search to avoid system ports)
	for port := startPort + 1; port < startPort+1000; port++ {
		if isPortAvailable(port) {
			utils.LogDevloop("Found available %s port: %d", portType, port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available %s port found in range %d-%d", portType, startPort+1, startPort+1000)
}

// isPortAvailable checks if a port is available for binding
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// validateFlags checks flag combinations and warns about ignored flags
func validateFlags(mode string, httpPort, grpcPort int, gatewayAddr string, enableMCP, autoPorts bool) {
	log.Printf("[devloop] Starting in %s mode", mode)

	switch mode {
	case "agent":
		// Agent mode warnings
		if httpPort != 9999 || grpcPort != 5555 {
			log.Printf("[devloop] WARNING: --http-port and --grpc-port are ignored in agent mode (no local HTTP/gRPC servers)")
		}
		if gatewayAddr == "" {
			log.Printf("[devloop] WARNING: Agent mode typically requires --gateway-addr to connect to a gateway")
		}
		log.Printf("[devloop] Agent mode: File watching=YES, HTTP/gRPC servers=NO, MCP=%s", mcpStatus(enableMCP))

	case "standalone":
		if gatewayAddr != "" {
			log.Printf("[devloop] WARNING: --gateway-addr is ignored in standalone mode")
		}
		log.Printf("[devloop] Standalone mode: HTTP server=:%d, gRPC server=:%d, MCP=%s", httpPort, grpcPort, mcpStatus(enableMCP))

	case "gateway":
		if gatewayAddr != "" {
			log.Printf("[devloop] WARNING: --gateway-addr is ignored in gateway mode (this IS the gateway)")
		}
		log.Printf("[devloop] Gateway mode: HTTP server=:%d, gRPC server=:%d, MCP=%s", httpPort, grpcPort, mcpStatus(enableMCP))

	default:
		log.Printf("[devloop] ERROR: Unknown mode '%s'. Valid modes: standalone, agent, gateway", mode)
	}
}

func mcpStatus(enableMCP bool) string {
	if enableMCP {
		return "enabled"
	}
	return "disabled"
}

func main() {
	var configPath string
	var showVersion bool
	var httpPort int
	var grpcPort int
	var gatewayAddr string
	var mode string
	var enableMCP bool
	var autoPorts bool

	// Define global flags
	flag.StringVar(&configPath, "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	flag.BoolVar(&showVersion, "version", false, "Display version information")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.IntVar(&httpPort, "http-port", 9999, "Port for the HTTP gateway server.")
	flag.IntVar(&grpcPort, "grpc-port", 5555, "Port for the gRPC server.")
	flag.BoolVar(&enableMCP, "enable-mcp", true, "Enable MCP server for AI tool integration.")
	flag.BoolVar(&autoPorts, "auto-ports", false, "Automatically find available ports if defaults are taken.")
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

	// Validate flag combinations and warn about ignored flags
	validateFlags(mode, httpPort, grpcPort, gatewayAddr, enableMCP, autoPorts)

	// Run the orchestrator in the selected mode
	runOrchestrator(configPath, mode, httpPort, grpcPort, gatewayAddr, enableMCP, autoPorts)
}

func runOrchestrator(configPath, mode string, httpPort, grpcPort int, gatewayAddr string, enableMCP, autoPorts bool) {
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

	// Start HTTP/gRPC gateway service (standalone and gateway modes only)
	if mode == "standalone" || mode == "gateway" {
		// Auto-discover ports if requested
		if autoPorts {
			if newGrpcPort, err := findAvailablePort(grpcPort, "gRPC"); err != nil {
				log.Fatalf("Error: %v\n", err)
			} else {
				grpcPort = newGrpcPort
			}

			if newHttpPort, err := findAvailablePort(httpPort, "HTTP"); err != nil {
				log.Fatalf("Error: %v\n", err)
			} else {
				httpPort = newHttpPort
			}
		}

		utils.LogDevloop("Starting HTTP/gRPC gateway service...")
		gatewayService = gateway.NewGatewayService(orchestrator)
		err = gatewayService.Start(grpcPort, httpPort)
		if err != nil {
			log.Fatalf("Error: Failed to start gateway service: %v\n", err)
		}
		utils.LogDevloop("Gateway service started: HTTP=:%d, gRPC=:%d", httpPort, grpcPort)

		// Mount MCP handlers on the gateway's HTTP server if enabled
		if enableMCP {
			utils.LogDevloop("Mounting MCP handlers on gateway HTTP server...")
			mcpService = mcp.NewMCPService(orchestrator)
			mcpHandler, err := mcpService.CreateHandler()
			if err != nil {
				log.Fatalf("Error: Failed to create MCP handler: %v\n", err)
			}
			gatewayService.MountMCPHandlers(mcpHandler)
			utils.LogDevloop("MCP handlers mounted at http://localhost:%d/mcp/", httpPort)
		}
	} else {
		utils.LogDevloop("Skipping HTTP/gRPC gateway service (not needed in %s mode)", mode)
		if enableMCP {
			utils.LogDevloop("WARNING: MCP requires HTTP server (only available in standalone/gateway modes)")
		}
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
