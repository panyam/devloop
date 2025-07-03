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
	var gatewayAddr string
	var mode string

	// Define global flags
	flag.StringVar(&configPath, "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	flag.BoolVar(&showVersion, "version", false, "Display version information")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.IntVar(&httpPort, "http-port", 8080, "Port for the HTTP gateway server.")
	flag.IntVar(&grpcPort, "grpc-port", 50051, "Port for the gRPC server.")
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
			if err := utils.ConvertAirToml(*convertInputPath); err != nil {
				log.Fatalf("Error: Failed to convert .air.toml: %v\n", err)
			}
			return
		}
	}

	// Run the orchestrator in the selected mode
	runOrchestrator(configPath, mode, httpPort, grpcPort, gatewayAddr)
}

func runOrchestrator(configPath, mode string, httpPort, grpcPort int, gatewayAddr string) {
	orchestrator, err := agent.NewOrchestratorV2(configPath, gatewayAddr)
	if err != nil {
		log.Fatalf("Error: Failed to initialize orchestrator: %v\n", err)
	}
	orchestrator.Verbose = verbose

	var gatewayService *gateway.GatewayService
	if mode == "standalone" || mode == "gateway" {
		gatewayService = gateway.NewGatewayService(orchestrator)
		err = gatewayService.Start(grpcPort, httpPort)
		if err != nil {
			log.Fatalf("Error: Failed to start gateway service: %v\n", err)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	shutdownComplete := make(chan bool, 1)
	go func() {
		<-sigChan
		log.Println("\n[devloop] Shutting down...")
		if gatewayService != nil {
			gatewayService.Stop()
		}
		orchestrator.Stop()
		shutdownComplete <- true
	}()

	if verbose {
		fmt.Println("[devloop] Config loaded successfully:")
		for _, rule := range orchestrator.Config.Rules {
			fmt.Printf("  Rule Name: %s\n", rule.Name)
			fmt.Printf("    Watch: %v\n", rule.Watch)
			fmt.Printf("    Commands: %v\n", rule.Commands)
		}
	}

	log.Println("[devloop] Starting orchestrator...")
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("Error: Failed to start orchestrator: %v\n", err)
	}

	// Wait for shutdown to complete if it was triggered
	select {
	case <-shutdownComplete:
		log.Println("[devloop] Shutdown complete")
		os.Exit(0)
	default:
		// Normal exit
	}
}
