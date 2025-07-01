package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runApp(configPath string) (*Orchestrator, error) {
	orchestrator, err := NewOrchestrator(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create orchestrator: %w", err)
	}

	fmt.Println("Config loaded successfully:")
	for _, rule := range orchestrator.Config.Rules {
		fmt.Printf("  Rule Name: %s\n", rule.Name)
		fmt.Printf("    Watch: %v\n", rule.Watch)
		fmt.Printf("    Commands: %v\n", rule.Commands)
	}

	log.Println("Starting orchestrator...")
	return orchestrator, nil
}

func main() {
	configPath := flag.String("c", "./multi.yaml", "Path to the multi.yaml configuration file")
	flag.Parse()

	orchestrator, err := runApp(*configPath)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer orchestrator.Stop()

	// Set up a channel to listen for OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the orchestrator in a goroutine
	go func() {
		if err := orchestrator.Start(); err != nil {
			log.Fatalf("Failed to start orchestrator: %v", err)
		}
	}()

	// Block until a signal is received
	sig := <-sigChan
	log.Printf("Received signal %v. Shutting down...", sig)
}
