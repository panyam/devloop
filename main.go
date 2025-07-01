package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Define subcommands
	convertCmd := flag.NewFlagSet("convert", flag.ExitOnError)
	convertInputPath := convertCmd.String("i", ".air.toml", "Path to the .air.toml input file")

	// Main command flags
	configPath := flag.String("c", "./multi.yaml", "Path to the multi.yaml configuration file")

	if len(os.Args) < 2 {
		// Default behavior: run the orchestrator
		runOrchestrator(*configPath)
		return
	}

	switch os.Args[1] {
	case "convert":
		convertCmd.Parse(os.Args[2:])
		if err := convertAirToml(*convertInputPath); err != nil {
			log.Fatalf("Failed to convert .air.toml: %v", err)
		}
	default:
		// Default behavior: run the orchestrator
		flag.Parse()
		runOrchestrator(*configPath)
	}
}

func runOrchestrator(configPath string) {
	orchestrator, err := NewOrchestrator(configPath)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		orchestrator.Stop()
	}()

	fmt.Println("Config loaded successfully:")
	for _, rule := range orchestrator.Config.Rules {
		fmt.Printf("  Rule Name: %s\n", rule.Name)
		fmt.Printf("    Watch: %v\n", rule.Watch)
		fmt.Printf("    Commands: %v\n", rule.Commands)
	}

	log.Println("Starting orchestrator...")
	if err := orchestrator.Start(); err != nil {
		log.Fatalf("Failed to start orchestrator: %v", err)
	}
}