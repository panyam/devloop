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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
		// Try multiple shell fallbacks for test environment compatibility
		shells := []string{"sh", "/bin/sh", "bash", "/bin/bash"}
		for _, shell := range shells {
			if _, err := exec.LookPath(shell); err == nil {
				cmd := exec.Command(shell, "-c", cmdStr)
				cmd.Env = os.Environ()
				return cmd
			}
		}
		// Last resort - try sh without checking
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Env = os.Environ()
		return cmd
	}
}

// expandCommandAliases performs single-pass left-to-right $name substitution.
// Each $name token is expanded at most once (no recursive expansion).
func expandCommandAliases(cmd string, commands map[string]string) string {
	if len(commands) == 0 {
		return cmd
	}

	var result strings.Builder
	i := 0
	for i < len(cmd) {
		if cmd[i] != '$' {
			result.WriteByte(cmd[i])
			i++
			continue
		}

		// Extract the name after $: alphanumeric + hyphens + underscores
		j := i + 1
		for j < len(cmd) && (cmd[j] == '-' || cmd[j] == '_' ||
			(cmd[j] >= 'a' && cmd[j] <= 'z') ||
			(cmd[j] >= 'A' && cmd[j] <= 'Z') ||
			(cmd[j] >= '0' && cmd[j] <= '9')) {
			j++
		}

		name := cmd[i+1 : j]
		if expansion, ok := commands[name]; ok {
			result.WriteString(expansion)
			i = j
		} else {
			result.WriteByte(cmd[i])
			i++
		}
	}
	return result.String()
}

// buildSourcePrefix builds a "source X && source Y && " prefix string from
// global and rule-level shell files. Global files resolve relative to configDir,
// rule files resolve relative to workDir.
func buildSourcePrefix(globalFiles, ruleFiles []string, workDir, configDir string) string {
	if len(globalFiles) == 0 && len(ruleFiles) == 0 {
		return ""
	}

	var parts []string
	for _, f := range globalFiles {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		parts = append(parts, "source "+path)
	}
	for _, f := range ruleFiles {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		parts = append(parts, "source "+path)
	}

	return strings.Join(parts, " && ") + " && "
}

// prepareCommand expands command aliases and prepends shell file sourcing.
// This is called before each command is passed to createCrossPlatformCommand.
func prepareCommand(cmd string, settings *pb.Settings, rule *pb.Rule, configDir string) string {
	// Step 1: expand command aliases ($name -> value)
	expanded := expandCommandAliases(cmd, settings.GetCommands())

	// Step 2: build source prefix from shell files
	workDir := rule.GetWorkDir()
	if workDir == "" {
		workDir = configDir
	}
	prefix := buildSourcePrefix(settings.GetShellFiles(), rule.GetShellFiles(), workDir, configDir)

	if prefix == "" {
		return expanded
	}
	return prefix + expanded
}

// parseEnvOutput parses NUL-delimited env output (from `env -0`) into a map.
func parseEnvOutput(data []byte) map[string]string {
	result := make(map[string]string)
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		idx := bytes.IndexByte(entry, '=')
		if idx < 0 {
			continue
		}
		key := string(entry[:idx])
		value := string(entry[idx+1:])
		result[key] = value
	}
	return result
}

// envDiff returns entries in current that are new or changed relative to base.
func envDiff(base, current map[string]string) map[string]string {
	diff := make(map[string]string)
	for k, v := range current {
		if baseVal, ok := base[k]; !ok || baseVal != v {
			diff[k] = v
		}
	}
	return diff
}

// envSliceToMap converts os.Environ()-style []string to a map.
func envSliceToMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, e := range environ {
		idx := strings.IndexByte(e, '=')
		if idx >= 0 {
			m[e[:idx]] = e[idx+1:]
		}
	}
	return m
}

// captureShellFileEnv sources global+rule shell files once and returns only
// the env vars that are new or changed relative to the current process env.
func captureShellFileEnv(globalFiles, ruleFiles []string, workDir, configDir string) (map[string]string, error) {
	if len(globalFiles) == 0 && len(ruleFiles) == 0 {
		return nil, nil
	}

	prefix := buildSourcePrefix(globalFiles, ruleFiles, workDir, configDir)
	cmdStr := prefix + "env -0"

	cmd := createCrossPlatformCommand(cmdStr)
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to capture shell file env: %w", err)
	}

	captured := parseEnvOutput(output)
	baseEnv := envSliceToMap(os.Environ())
	return envDiff(baseEnv, captured), nil
}

// parseEnvFile reads a NUL-delimited env file (written by `env -0 > file`).
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseEnvOutput(data), nil
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
		if prefixMaxLength, exists := settings["prefix_max_length"]; exists {
			if prefixMaxLengthInt, ok := prefixMaxLength.(int); ok {
				config.Settings.PrefixMaxLength = uint32(prefixMaxLengthInt)
			}
		}
		if colorLogs, exists := settings["color_logs"]; exists {
			if colorLogsBool, ok := colorLogs.(bool); ok {
				config.Settings.ColorLogs = colorLogsBool
			}
		}
		if colorScheme, exists := settings["color_scheme"]; exists {
			if colorSchemeStr, ok := colorScheme.(string); ok {
				config.Settings.ColorScheme = colorSchemeStr
			}
		}
		if customColors, exists := settings["custom_colors"]; exists {
			if customColorsMap, ok := customColors.(map[string]interface{}); ok {
				config.Settings.CustomColors = make(map[string]string)
				for key, value := range customColorsMap {
					if valueStr, ok := value.(string); ok {
						config.Settings.CustomColors[key] = valueStr
					}
				}
			}
		}

		// Fix cycle_detection field
		if cycleDetection, exists := settings["cycle_detection"]; exists {
			if cycleDetectionMap, ok := cycleDetection.(map[string]interface{}); ok {
				cycleSettings := &pb.CycleDetectionSettings{}

				if enabled, exists := cycleDetectionMap["enabled"]; exists {
					if enabledBool, ok := enabled.(bool); ok {
						cycleSettings.Enabled = enabledBool
					}
				}

				if staticValidation, exists := cycleDetectionMap["static_validation"]; exists {
					if staticValidationBool, ok := staticValidation.(bool); ok {
						cycleSettings.StaticValidation = staticValidationBool
					}
				}

				if dynamicProtection, exists := cycleDetectionMap["dynamic_protection"]; exists {
					if dynamicProtectionBool, ok := dynamicProtection.(bool); ok {
						cycleSettings.DynamicProtection = dynamicProtectionBool
					}
				}

				if maxTriggersPerMinute, exists := cycleDetectionMap["max_triggers_per_minute"]; exists {
					if maxTriggersInt, ok := maxTriggersPerMinute.(int); ok {
						cycleSettings.MaxTriggersPerMinute = uint32(maxTriggersInt)
					}
				}

				if maxChainDepth, exists := cycleDetectionMap["max_chain_depth"]; exists {
					if maxChainDepthInt, ok := maxChainDepth.(int); ok {
						cycleSettings.MaxChainDepth = uint32(maxChainDepthInt)
					}
				}

				if fileThrashWindowSeconds, exists := cycleDetectionMap["file_thrash_window_seconds"]; exists {
					if fileThrashWindowSecondsInt, ok := fileThrashWindowSeconds.(int); ok {
						cycleSettings.FileThrashWindowSeconds = uint32(fileThrashWindowSecondsInt)
					}
				}

				if fileThrashThreshold, exists := cycleDetectionMap["file_thrash_threshold"]; exists {
					if fileThrashThresholdInt, ok := fileThrashThreshold.(int); ok {
						cycleSettings.FileThrashThreshold = uint32(fileThrashThresholdInt)
					}
				}

				config.Settings.CycleDetection = cycleSettings
			}
		}

		// Fix max_parallel_rules field
		if maxParallelRules, exists := settings["max_parallel_rules"]; exists {
			if maxParallelRulesInt, ok := maxParallelRules.(int); ok {
				config.Settings.MaxParallelRules = uint32(maxParallelRulesInt)
			}
		}

		// Fix shell_files field
		if shellFiles, exists := settings["shell_files"]; exists {
			if shellFilesList, ok := shellFiles.([]interface{}); ok {
				config.Settings.ShellFiles = make([]string, 0, len(shellFilesList))
				for _, sf := range shellFilesList {
					if sfStr, ok := sf.(string); ok {
						config.Settings.ShellFiles = append(config.Settings.ShellFiles, sfStr)
					}
				}
			}
		}

		// Fix commands field
		if commands, exists := settings["commands"]; exists {
			if commandsMap, ok := commands.(map[string]interface{}); ok {
				config.Settings.Commands = make(map[string]string)
				for key, value := range commandsMap {
					if valueStr, ok := value.(string); ok {
						config.Settings.Commands[key] = valueStr
					}
				}
			}
		}

		// Fix reset_env field
		if resetEnv, exists := settings["reset_env"]; exists {
			if resetEnvBool, ok := resetEnv.(bool); ok {
				config.Settings.ResetEnv = resetEnvBool
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

				if appendOnRestarts, exists := ruleMap["append_on_restarts"]; exists {
					if appendBool, ok := appendOnRestarts.(bool); ok && i < len(config.Rules) {
						config.Rules[i].AppendOnRestarts = appendBool
					}
				}

				if disabled, exists := ruleMap["disabled"]; exists {
					if disabledBool, ok := disabled.(bool); ok && i < len(config.Rules) {
						config.Rules[i].Disabled = disabledBool
					}
				}

				if shellFiles, exists := ruleMap["shell_files"]; exists {
					if shellFilesList, ok := shellFiles.([]interface{}); ok && i < len(config.Rules) {
						config.Rules[i].ShellFiles = make([]string, 0, len(shellFilesList))
						for _, sf := range shellFilesList {
							if sfStr, ok := sf.(string); ok {
								config.Rules[i].ShellFiles = append(config.Rules[i].ShellFiles, sfStr)
							}
						}
					}
				}

				if resetEnv, exists := ruleMap["reset_env"]; exists {
					if resetEnvBool, ok := resetEnv.(bool); ok && i < len(config.Rules) {
						config.Rules[i].ResetEnv = &resetEnvBool
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

	// suppress_subprocess_colors defaults to false (preserve colors by default)
	// No special handling needed - protobuf bool defaults work perfectly for this

	// Resolve all patterns in all rules to be absolute paths.
	for i := range config.Rules {
		rule := config.Rules[i]

		// Default to global setting if not specified on rule
		if rule.DefaultAction == "" {
			rule.DefaultAction = config.Settings.DefaultWatchAction
		}

		// Note: We no longer resolve relative patterns to absolute paths here.
		// Patterns will be resolved dynamically relative to each rule's work_dir
		// when matching files. This allows patterns to be relative to the rule's
		// working directory instead of the config file location.
	}

	return &config, nil
}

// NewOrchestratorForTesting creates a new V2 orchestrator for testing
func NewOrchestratorForTesting(configPath string) (*Orchestrator, error) {
	return NewOrchestrator(configPath)
}
