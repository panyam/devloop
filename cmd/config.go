package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/panyam/devloop/client"
	"github.com/spf13/cobra"
)

var (
	configOutputFormat string
	configServerAddr   string
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Get configuration from running devloop server",
	Long: `Retrieve the current configuration from a running devloop server.

This command connects to a running devloop server and retrieves its configuration,
including all rules, settings, and file watch patterns. The output can be formatted
as JSON or YAML for easy inspection or processing.

Examples:
  devloop config                      # Get config from default server
  devloop config --server :6000       # Get config from server on port 6000
  devloop config --format yaml        # Output in YAML format
  devloop config --format json        # Output in JSON format (default)`,
	Run: func(cmd *cobra.Command, args []string) {
		runGetConfig()
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	
	configCmd.Flags().StringVarP(&configOutputFormat, "format", "f", "json", "Output format (json, yaml)")
	configCmd.Flags().StringVarP(&configServerAddr, "server", "s", "localhost:5555", "Server address (host:port)")
}

func runGetConfig() {
	client, err := client.NewClient(client.Config{
		Address: configServerAddr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	config, err := client.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get config: %v\n", err)
		os.Exit(1)
	}

	switch configOutputFormat {
	case "json":
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to marshal config to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	case "yaml":
		// For now, just output a simplified YAML representation
		fmt.Printf("rules:\n")
		for _, rule := range config.Rules {
			fmt.Printf("  - name: %s\n", rule.Name)
			fmt.Printf("    commands:\n")
			for _, cmd := range rule.Commands {
				fmt.Printf("      - %s\n", cmd)
			}
			if len(rule.Watch) > 0 {
				fmt.Printf("    watch:\n")
				for _, watch := range rule.Watch {
					fmt.Printf("      - action: %s\n", watch.Action)
					fmt.Printf("        patterns:\n")
					for _, pattern := range watch.Patterns {
						fmt.Printf("          - %s\n", pattern)
					}
				}
			}
		}
		if config.Settings != nil {
			fmt.Printf("settings:\n")
			fmt.Printf("  verbose: %v\n", config.Settings.Verbose)
			fmt.Printf("  prefix_logs: %v\n", config.Settings.PrefixLogs)
			fmt.Printf("  color_logs: %v\n", config.Settings.ColorLogs)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown output format %q. Use 'json' or 'yaml'\n", configOutputFormat)
		os.Exit(1)
	}
}