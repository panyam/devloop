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
// 1. Standalone Mode (Default): Local file watcher for single project development.
// 2. Agent Mode: Connects to a central gateway for multi-project management.
// 3. Gateway Mode: Central hub for managing multiple devloop agents.
//
// # Examples
//
// See the examples/ directory for comprehensive real-world examples including:
// - Full-stack web applications
// - Microservices architectures
// - Data science workflows
// - Docker compose setups
// - Frontend framework development
package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/panyam/devloop/cmd"
	"github.com/spf13/cobra"
)

//go:embed VERSION
var version string

//go:embed .LAST_BUILT_AT
var lastBuiltAt string

func main() {
	// Add version command
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("devloop version %s (Built: %s)\n", strings.TrimSpace(version), strings.TrimSpace(lastBuiltAt))
		},
	}

	// Get the root command and add version subcommand
	rootCmd := cmd.GetRootCommand()
	rootCmd.AddCommand(versionCmd)

	// Execute the CLI
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
