package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	initOutput string
	initForce  bool
)

// Embed profile files
//go:embed profiles/default.yaml
var defaultProfile []byte

//go:embed profiles/golang.yaml
var golangProfile []byte

//go:embed profiles/typescript.yaml
var typescriptProfile []byte

//go:embed profiles/python.yaml
var pythonProfile []byte

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [profiles...]",
	Short: "Initialize a new .devloop.yaml configuration file",
	Long: `Initialize a new .devloop.yaml configuration file with predefined profiles.

Available profiles:
  golang     - Go project with build and run commands
  ts         - TypeScript/Node.js project with build and serve
  python     - Python Flask project with development server
  
If no profiles are specified, creates a basic "hello world" configuration that
watches all files and runs a simple echo command.

Examples:
  devloop init                    # Create basic configuration
  devloop init golang             # Create Go project configuration  
  devloop init ts python          # Create TypeScript and Python configurations
  devloop init golang ts python   # Create all three project configurations`,
	Run: func(cmd *cobra.Command, args []string) {
		runInit(args)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVarP(&initOutput, "output", "o", ".devloop.yaml", "Output file path")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing configuration file")
}

// DevLoopConfig represents the structure of a .devloop.yaml file
type DevLoopConfig struct {
	Settings *Settings `yaml:"settings,omitempty"`
	Rules    []Rule    `yaml:"rules"`
}

type Settings struct {
	PrefixLogs           bool              `yaml:"prefix_logs,omitempty"`
	PrefixMaxLength      int               `yaml:"prefix_max_length,omitempty"`
	ColorLogs            bool              `yaml:"color_logs,omitempty"`
	ColorScheme          string            `yaml:"color_scheme,omitempty"`
	CustomColors         map[string]string `yaml:"custom_colors,omitempty"`
	Verbose              bool              `yaml:"verbose,omitempty"`
	DefaultDebounceDelay string            `yaml:"default_debounce_delay,omitempty"`
}

type Rule struct {
	Name           string            `yaml:"name"`
	Prefix         string            `yaml:"prefix,omitempty"`
	Color          string            `yaml:"color,omitempty"`
	Workdir        string            `yaml:"workdir,omitempty"`
	RunOnInit      *bool             `yaml:"run_on_init,omitempty"`
	Verbose        bool              `yaml:"verbose,omitempty"`
	DebounceDelay  string            `yaml:"debounce_delay,omitempty"`
	Env            map[string]string `yaml:"env,omitempty"`
	Watch          []WatchPattern    `yaml:"watch"`
	Commands       []string          `yaml:"commands"`
}

type WatchPattern struct {
	Action   string   `yaml:"action"`
	Patterns []string `yaml:"patterns"`
}

func runInit(profiles []string) {
	// Check if output file already exists
	if _, err := os.Stat(initOutput); err == nil && !initForce {
		fmt.Fprintf(os.Stderr, "Error: %s already exists. Use --force to overwrite.\n", initOutput)
		os.Exit(1)
	}

	var config DevLoopConfig

	// If no profiles specified, use default configuration
	if len(profiles) == 0 {
		defaultConfig, err := loadDefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load default configuration: %v\n", err)
			os.Exit(1)
		}
		config = *defaultConfig
	} else {
		// Set default settings for multi-profile configurations
		config.Settings = &Settings{
			PrefixLogs:           true,
			ColorLogs:            true,
			ColorScheme:          "auto",
			DefaultDebounceDelay: "500ms",
		}

		// Create rules based on specified profiles
		for _, profile := range profiles {
			rule, err := loadProfileRule(profile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load profile '%s': %v - skipping\n", profile, err)
				continue
			}
			config.Rules = append(config.Rules, *rule)
		}

		// If no valid profiles were loaded, exit with error
		if len(config.Rules) == 0 {
			fmt.Fprintf(os.Stderr, "Error: No valid profiles could be loaded\n")
			os.Exit(1)
		}
	}

	// Write configuration to file
	if err := writeConfigToFile(config, initOutput); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully created %s", initOutput)
	if len(profiles) > 0 {
		fmt.Printf(" with profiles: %s", strings.Join(profiles, ", "))
	}
	fmt.Println()
}

func loadDefaultConfig() (*DevLoopConfig, error) {
	var config DevLoopConfig
	if err := yaml.Unmarshal(defaultProfile, &config); err != nil {
		return nil, fmt.Errorf("failed to parse default profile: %w", err)
	}
	return &config, nil
}

func loadProfileRule(profileName string) (*Rule, error) {
	var profileData []byte
	
	switch strings.ToLower(profileName) {
	case "golang", "go":
		profileData = golangProfile
	case "ts", "typescript", "node", "nodejs":
		profileData = typescriptProfile
	case "python", "py", "flask":
		profileData = pythonProfile
	default:
		return nil, fmt.Errorf("unknown profile: %s", profileName)
	}

	var rule Rule
	if err := yaml.Unmarshal(profileData, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse profile %s: %w", profileName, err)
	}

	return &rule, nil
}

func writeConfigToFile(config DevLoopConfig, filename string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}