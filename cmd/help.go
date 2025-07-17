package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// helpCmd represents the help command
var helpCmd = &cobra.Command{
	Use:   "help [topic]",
	Short: "Help about devloop usage and configuration",
	Long: `Devloop is a versatile tool designed to streamline your inner development loop,
particularly within Multi-Component Projects. It combines functionalities inspired
by live-reloading tools like air (for Go) and build automation tools like make,
focusing on simple, configuration-driven orchestration of tasks based on file system changes.

By default, devloop starts the agent when no subcommands are specified.

The help sub command provides detailed help information about devloop usage, configuration options,
and other topics. Use 'devloop help config' to get detailed information about
.devloop.yaml configuration options.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Help()
			return
		}

		topic := args[0]
		switch topic {
		case "config":
			showConfigHelp()
		default:
			fmt.Printf("Unknown help topic: %s\n", topic)
			fmt.Println("Available topics: config")
		}
	},
}

var helpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show detailed configuration help",
	Long:  `Show detailed help about .devloop.yaml configuration options`,
	Run: func(cmd *cobra.Command, args []string) {
		showConfigHelp()
	},
}

func init() {
	rootCmd.AddCommand(helpCmd)
	helpCmd.AddCommand(helpConfigCmd)
}

func showConfigHelp() {
	configHelp := `
Configuration Reference for .devloop.yaml

Complete Configuration Structure:

  settings:                      # Optional: Global settings
    prefix_logs: boolean         # Enable/disable log prefixing (default: true)
    prefix_max_length: number    # Max length for prefixes (default: unlimited)
    color_logs: boolean          # Enable colored output (default: true)
    color_scheme: string         # Color scheme: "auto", "dark", "light" (default: "auto")
    custom_colors:               # Optional: Custom color mappings
      rule_name: "color"         # Map rule names to specific colors
    verbose: boolean             # Global verbose logging (default: false)
    default_debounce_delay: duration  # Global debounce delay (default: 500ms)

  rules:                         # Required: Array of rules
    - name: string              # Required: Unique rule identifier
      prefix: string            # Optional: Custom log prefix (defaults to name)
      color: string             # Optional: Custom color for this rule's output
      workdir: string           # Optional: Working directory for commands (defaults to config dir)
      run_on_init: boolean      # Optional: Run on startup (default: true)
      verbose: boolean          # Optional: Per-rule verbose logging
      debounce_delay: duration  # Optional: Per-rule debounce delay (e.g., "200ms")
      env:                      # Optional: Environment variables
        KEY: "value"
      watch:                    # Required: File watch configuration
        - action: string        # Required: "include" or "exclude"
          patterns: [string]    # Required: Glob patterns
      commands: [string]        # Required: Shell commands to execute

Settings Options:

┌─────────────────────┬─────────┬─────────────┬─────────────────────────────────────────────────────┐
│ Option              │ Type    │ Default     │ Description                                         │
├─────────────────────┼─────────┼─────────────┼─────────────────────────────────────────────────────┤
│ prefix_logs         │ boolean │ true        │ Prepend rule name/prefix to each output line       │
│ prefix_max_length   │ integer │ unlimited   │ Truncate/pad prefixes to this length for alignment │
│ color_logs          │ boolean │ true        │ Enable colored output to distinguish different     │
│                     │         │             │ rules                                               │
│ color_scheme        │ string  │ "auto"      │ Color palette: "auto" (detect), "dark", "light",   │
│                     │         │             │ or "custom"                                         │
│ custom_colors       │ map     │ {}          │ Map rule names to specific colors                   │
│                     │         │             │ (e.g., rule_name: "blue")                          │
│ verbose             │ boolean │ false       │ Enable verbose logging globally                     │
│ default_debounce_   │ duration│ "500ms"     │ Default delay before executing commands after      │
│ delay               │         │             │ file changes                                        │
└─────────────────────┴─────────┴─────────────┴─────────────────────────────────────────────────────┘

Rule Options:

┌─────────────────────┬─────────┬──────────┬─────────────────────────────────────────────────────────┐
│ Option              │ Type    │ Required │ Description                                             │
├─────────────────────┼─────────┼──────────┼─────────────────────────────────────────────────────────┤
│ name                │ string  │ ✅       │ Unique identifier for the rule                         │
│ prefix              │ string  │ ❌       │ Custom prefix for log output (overrides name)         │
│ color               │ string  │ ❌       │ Custom color for this rule's output                    │
│                     │         │          │ (e.g., "blue", "red", "bold-green")                   │
│ workdir             │ string  │ ❌       │ Working directory for command execution                │
│                     │         │          │ (defaults to config file directory)                   │
│ verbose             │ boolean │ ❌       │ Enable verbose logging for this rule only             │
│ debounce_delay      │ duration│ ❌       │ Delay before executing after file changes             │
│                     │         │          │ (e.g., "200ms")                                       │
│ env                 │ map     │ ❌       │ Additional environment variables                       │
│ watch               │ array   │ ✅       │ File patterns to monitor                               │
│ commands            │ array   │ ✅       │ Commands to execute when files change                  │
│ run_on_init         │ boolean │ ❌       │ Run commands on startup (default: true)               │
│ exit_on_failed_init │ boolean │ ❌       │ Exit devloop when this rule fails startup             │
│                     │         │          │ (default: false)                                       │
│ max_init_retries    │ integer │ ❌       │ Maximum retry attempts for failed startup             │
│                     │         │          │ (default: 10)                                          │
│ init_retry_backoff_ │ integer │ ❌       │ Base backoff duration in ms for startup retries       │
│ base                │         │          │ (default: 3000)                                        │
└─────────────────────┴─────────┴──────────┴─────────────────────────────────────────────────────────┘

Watch Configuration:

  Each watch entry consists of:

┌─────────┬─────────┬─────────────────────┬─────────────────────────────────────────────────────────┐
│ Field   │ Type    │ Values              │ Description                                             │
├─────────┼─────────┼─────────────────────┼─────────────────────────────────────────────────────────┤
│ action  │ string  │ include, exclude    │ Whether to trigger on or ignore matches                │
│ patterns│ array   │ glob patterns       │ File patterns using doublestar syntax                  │
└─────────┴─────────┴─────────────────────┴─────────────────────────────────────────────────────────┘

Glob Pattern Syntax:

  * - Matches any sequence of non-separator characters
  ** - Matches any sequence of characters including path separators
  ? - Matches any single non-separator character
  [abc] - Matches any character in the set
  [a-z] - Matches any character in the range
  {a,b} - Matches either pattern a or b

Pattern Matching Examples:

  "**/*.go"              # Match all Go files
  "src/**/*.go"          # Match Go files only in src directory
  "**/*_test.go"         # Match test files
  "**/*.{js,jsx,ts,tsx}" # Match multiple extensions
  "cmd/server/**/*"      # Match specific directory
  "*.go"                 # Match top-level files only
  "**/.*"                # Match hidden files

Example Configuration:

  settings:
    prefix_logs: true
    prefix_max_length: 12

  rules:
    - name: "Backend API"
      prefix: "api"
      workdir: "./backend"
      exit_on_failed_init: false
      max_init_retries: 5
      init_retry_backoff_base: 2000
      env:
        NODE_ENV: "development"
        PORT: "3000"
        DATABASE_URL: "postgres://localhost/myapp"
      watch:
        - action: "exclude"
          patterns:
            - "**/vendor/**"
            - "**/*_test.go"
            - "**/.*"
        - action: "include"
          patterns:
            - "**/*.go"
            - "go.mod"
            - "go.sum"
      commands:
        - "echo 'Building API server...'"
        - "go mod tidy"
        - "go build -tags dev -o bin/api ./cmd/api"
        - "./bin/api --dev"

For more examples and detailed documentation, see:
  https://github.com/panyam/devloop/blob/main/README.md
`

	fmt.Print(strings.TrimLeft(configHelp, "\n"))
}
