package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/panyam/devloop/client"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/spf13/cobra"
)

var (
	statusServerAddr string
	statusWatch      bool
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status [rule-name]",
	Short: "Get status of rules from running devloop server",
	Long: `Get the execution status of rules from a running devloop server.

This command shows the current status of rules, including whether they are running,
their last execution result, and execution history. If no rule name is provided,
it shows the status of all rules.

Examples:
  devloop status                      # Show status of all rules
  devloop status backend              # Show status of specific rule
  devloop status --server :6000       # Get status from server on port 6000
  devloop status --watch backend      # Watch status changes (future)`,
	Run: func(cmd *cobra.Command, args []string) {
		runGetStatus(args)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringVarP(&statusServerAddr, "server", "s", "localhost:5555", "Server address (host:port)")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Watch for status changes (not yet implemented)")
}

func runGetStatus(args []string) {
	client, err := client.NewClient(client.Config{
		Address: statusServerAddr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if len(args) > 0 {
		// Get status for specific rule
		ruleName := args[0]
		rule, err := client.GetRuleStatus(ruleName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get status for rule %q: %v\n", ruleName, err)
			os.Exit(1)
		}
		printRuleStatus(rule)
	} else {
		// Get status for all rules
		config, err := client.GetConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Status of %d rules:\n\n", len(config.Rules))
		for _, configRule := range config.Rules {
			rule, err := client.GetRuleStatus(configRule.Name)
			if err != nil {
				fmt.Printf("Rule: %s - Error: %v\n", configRule.Name, err)
				continue
			}
			printRuleStatus(rule)
			fmt.Println()
		}
	}
}

func printRuleStatus(rule *pb.Rule) {
	fmt.Printf("Rule: %s\n", rule.Name)
	fmt.Printf("  Status: %s\n", formatStatus(rule.Status))
	fmt.Printf("  Commands: %s\n", strings.Join(rule.Commands, ", "))

	if rule.Status != nil {
		if rule.Status.IsRunning {
			fmt.Printf("  Currently Running: Yes\n")
		} else {
			fmt.Printf("  Currently Running: No\n")
		}

		if rule.Status.StartTime != nil {
			fmt.Printf("  Started At: %s\n", rule.Status.StartTime.AsTime().Format("2006-01-02 15:04:05"))
		}

		if rule.Status.LastBuildTime != nil {
			fmt.Printf("  Last Build: %s\n", rule.Status.LastBuildTime.AsTime().Format("2006-01-02 15:04:05"))
		}

		if rule.Status.LastBuildStatus != "" {
			fmt.Printf("  Last Build Status: %s\n", rule.Status.LastBuildStatus)
		}
	}

	if len(rule.Watch) > 0 {
		fmt.Printf("  Watch Patterns:\n")
		for _, watch := range rule.Watch {
			fmt.Printf("    %s: %s\n", watch.Action, strings.Join(watch.Patterns, ", "))
		}
	}
}

func formatStatus(status *pb.RuleStatus) string {
	if status == nil {
		return "Unknown"
	}

	if status.IsRunning {
		return "Running"
	}

	switch status.LastBuildStatus {
	case "SUCCESS":
		return "Success"
	case "FAILED":
		return "Failed"
	case "IDLE":
		return "Idle"
	default:
		return "Unknown"
	}
}
