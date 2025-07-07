package cmd

import (
	"fmt"
	"os"

	"github.com/panyam/devloop/client"
	"github.com/spf13/cobra"
)

var (
	triggerServerAddr string
	triggerWait       bool
)

// triggerCmd represents the trigger command
var triggerCmd = &cobra.Command{
	Use:   "trigger <rule-name>",
	Short: "Trigger execution of a specific rule",
	Long: `Manually trigger execution of a specific rule on a running devloop server.

This command immediately starts execution of the specified rule, bypassing file
watching. It will terminate any currently running instance of the rule and start
a new execution with all commands in the rule definition.

Examples:
  devloop trigger backend             # Trigger the backend rule
  devloop trigger tests               # Trigger the tests rule
  devloop trigger --server :6000 build # Trigger build rule on server at port 6000
  devloop trigger --wait frontend     # Trigger and wait for completion (future)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runTrigger(args[0])
	},
}

func init() {
	rootCmd.AddCommand(triggerCmd)
	
	triggerCmd.Flags().StringVarP(&triggerServerAddr, "server", "s", "localhost:5555", "Server address (host:port)")
	triggerCmd.Flags().BoolVarP(&triggerWait, "wait", "w", false, "Wait for rule execution to complete (not yet implemented)")
}

func runTrigger(ruleName string) {
	client, err := client.NewClient(client.Config{
		Address: triggerServerAddr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, err := client.TriggerRule(ruleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to trigger rule %q: %v\n", ruleName, err)
		os.Exit(1)
	}

	if resp.Success {
		fmt.Printf("✅ Successfully triggered rule %q\n", ruleName)
		if resp.Message != "" {
			fmt.Printf("Message: %s\n", resp.Message)
		}
		fmt.Println("\nUse 'devloop status' to monitor execution progress.")
	} else {
		fmt.Printf("❌ Failed to trigger rule %q\n", ruleName)
		if resp.Message != "" {
			fmt.Printf("Message: %s\n", resp.Message)
		}
		os.Exit(1)
	}
}