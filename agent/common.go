// Package agent provides the core orchestration engine for devloop.
//
// The agent package contains the orchestrator implementations that handle:
// - File system watching and event processing
// - Rule execution and process management
// - Gateway communication for distributed setups
// - Logging and output management
//
// # Key Components
//
// The main orchestrator interface provides methods for:
//
//	type Orchestrator interface {
//		Start() error
//		Stop() error
//		GetConfig() *pb.Config
//		TriggerRule(ruleName string) error
//		GetRuleStatus(ruleName string) (*pb.RuleStatus, bool)
//		GetWatchedPaths() []string
//		ReadFileContent(path string) ([]byte, error)
//		StreamLogs(ruleName string, filter string, stream pb.AgentService_StreamLogsClientServer) error
//		SetGlobalDebounceDelay(duration time.Duration)
//		SetVerbose(verbose bool)
//	}
//
// # Usage
//
// Create and start an orchestrator:
//
//	orchestrator := agent.NewOrchestrator("config.yaml", "")
//	if err := orchestrator.Start(); err != nil {
//		log.Fatal(err)
//	}
//	defer orchestrator.Stop()
//
// # Configuration
//
// The orchestrator is configured via YAML files with rules that define:
// - File patterns to watch
// - Commands to execute when files change
// - Debounce settings and execution options
//
// Example configuration:
//
//	settings:
//	  project_id: "my-project"
//	  prefix_logs: true
//	rules:
//	  - name: "build"
//	    watch:
//	      - action: include
//	        patterns: ["**/*.go"]
//	    commands:
//	      - "go build ."
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// createCrossPlatformCommand creates a command that works on both Unix and Windows
func createCrossPlatformCommand(cmdStr string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", cmdStr)
	default:
		// Unix-like systems (Linux, macOS, BSD, etc.)
		shell := "bash"
		// Check if bash exists, fallback to sh for better POSIX compatibility
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}
		return exec.Command(shell, "-c", cmdStr)
	}
}

// LoadConfig reads and unmarshals the .devloop.yaml configuration file,
// resolving all relative watch paths to be absolute from the config file's location.
func LoadConfig(configPath string) (*pb.Config, error) {
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config file: %w", err)
	}

	data, err := os.ReadFile(absConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %q", absConfigPath)
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", absConfigPath, err)
	}

	// First parse into a generic map to handle problematic fields
	var rawConfig map[string]interface{}
	err = yaml.Unmarshal(data, &rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q into raw map: %w", absConfigPath, err)
	}

	// Then parse into protobuf struct
	var config pb.Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", absConfigPath, err)
	}

	// Workaround: Manually fix fields that don't parse correctly from YAML to protobuf structs
	// This is needed because protobuf-generated struct tags don't work correctly with gopkg.in/yaml.v3

	// Fix settings fields
	if settings, ok := rawConfig["settings"].(map[string]interface{}); ok {
		if projectId, exists := settings["project_id"]; exists {
			if projectIdStr, ok := projectId.(string); ok {
				config.Settings.ProjectId = projectIdStr
			}
		}
		if prefixLogs, exists := settings["prefix_logs"]; exists {
			if prefixLogsBool, ok := prefixLogs.(bool); ok {
				config.Settings.PrefixLogs = prefixLogsBool
			}
		}
	}

	// Fix rule skipRunOnInit fields
	if rules, ok := rawConfig["rules"].([]interface{}); ok {
		for i, rawRule := range rules {
			if ruleMap, ok := rawRule.(map[string]interface{}); ok {
				// Check for both possible field names (camelCase and snake_case)
				if skipInit, exists := ruleMap["skipRunOnInit"]; exists {
					if skipBool, ok := skipInit.(bool); ok && i < len(config.Rules) {
						config.Rules[i].SkipRunOnInit = skipBool
					}
				} else if skipInit, exists := ruleMap["skip_run_on_init"]; exists {
					if skipBool, ok := skipInit.(bool); ok && i < len(config.Rules) {
						config.Rules[i].SkipRunOnInit = skipBool
					}
				}
			}
		}
	}

	// Ensure Settings is initialized
	if config.Settings == nil {
		config.Settings = &pb.Settings{}
	}

	// Default to exclude if not specified globally
	if config.Settings.DefaultWatchAction == "" {
		config.Settings.DefaultWatchAction = "exclude"
	}

	// Resolve all patterns in all rules to be absolute paths.
	for i := range config.Rules {
		rule := config.Rules[i]

		// Default to global setting if not specified on rule
		if rule.DefaultAction == "" {
			rule.DefaultAction = config.Settings.DefaultWatchAction
		}

		for j := range rule.Watch {
			matcher := rule.Watch[j]
			for k, pattern := range matcher.Patterns {
				if !filepath.IsAbs(pattern) {
					// Make relative patterns absolute relative to the config file's directory.
					resolvedPattern := filepath.Join(filepath.Dir(absConfigPath), pattern)
					matcher.Patterns[k] = resolvedPattern
				}
			}
		}
	}

	return &config, nil
}

// NewOrchestratorForTesting creates a new V2 orchestrator for testing
func NewOrchestratorForTesting(configPath string) (*Orchestrator, error) {
	return NewOrchestrator(configPath)
}
