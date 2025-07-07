package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var (
	pathsServerAddr string
	pathsSort       bool
)

// pathsCmd represents the paths command
var pathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "List all file patterns being watched",
	Long: `List all file glob patterns being monitored by the devloop server.

This command shows all file patterns that trigger rule execution when matching
files change. It combines patterns from all rules and displays them in a
unified list to help understand what file changes will trigger builds.

Examples:
  devloop paths                       # List all watched patterns
  devloop paths --server :6000        # List patterns from server on port 6000
  devloop paths --sort                # Sort patterns alphabetically`,
	Run: func(cmd *cobra.Command, args []string) {
		runListPaths()
	},
}

func init() {
	rootCmd.AddCommand(pathsCmd)

	pathsCmd.Flags().StringVarP(&pathsServerAddr, "server", "s", "localhost:5555", "Server address (host:port)")
	pathsCmd.Flags().BoolVar(&pathsSort, "sort", false, "Sort patterns alphabetically")
}

func runListPaths() {
	client, err := NewClient(Config{
		Address: pathsServerAddr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	paths, err := client.ListWatchedPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list watched paths: %v\n", err)
		os.Exit(1)
	}

	if len(paths) == 0 {
		fmt.Println("No file patterns are being watched.")
		return
	}

	if pathsSort {
		sort.Strings(paths)
	}

	fmt.Printf("Watching %d file patterns:\n\n", len(paths))
	for i, path := range paths {
		fmt.Printf("%3d. %s\n", i+1, path)
	}

	fmt.Printf("\nThese patterns will trigger rule execution when matching files change.\n")
}
