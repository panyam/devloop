package cmd

import (
	"fmt"
	"os"

	"github.com/panyam/devloop/gateway"
	"github.com/spf13/cobra"
)

var (
	convertInput string
)

// convertCmd represents the convert command
var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert .air.toml configuration to .devloop.yaml",
	Long: `Convert an existing .air.toml configuration file to devloop's .devloop.yaml format.

This command helps migrate from the air live-reloading tool to devloop by
converting the configuration format. It reads the .air.toml file and creates
an equivalent .devloop.yaml file with similar functionality.

Examples:
  devloop convert                       # Convert .air.toml in current directory
  devloop convert -i path/to/.air.toml  # Convert specific .air.toml file
  devloop convert --input my.air.toml   # Convert with explicit input file`,
	Run: func(cmd *cobra.Command, args []string) {
		runConvert()
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVarP(&convertInput, "input", "i", ".air.toml", "Path to the .air.toml input file")
}

func runConvert() {
	if err := gateway.ConvertAirToml(convertInput); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to convert .air.toml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully converted %s to .devloop.yaml\n", convertInput)
}
