package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const version = "0.0.4" // Define the current version

var verbose bool // Global flag for verbose logging

func main() {
	var configPath string
	var showVersion bool
	var httpPort string

	// Define global flags
	flag.StringVar(&configPath, "c", "./.devloop.yaml", "Path to the .devloop.yaml configuration file")
	flag.BoolVar(&showVersion, "version", false, "Display version information")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.StringVar(&httpPort, "http-port", "", "Port for the HTTP log streaming server. By default the log server is not started")

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
			if err := convertAirToml(*convertInputPath); err != nil {
				log.Fatalf("Error: Failed to convert .air.toml: %v\n", err)
			}
			return
		}
	}

	// Default behavior: run the orchestrator
	runOrchestrator(configPath, httpPort)
}

func runOrchestrator(configPath string, httpPort string) {
	orchestrator, err := NewOrchestrator(configPath, httpPort)
	if err != nil {
		log.Fatalf("Error: Failed to initialize orchestrator: %v\n", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\n[devloop] Shutting down...")
		orchestrator.Stop()
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
}
