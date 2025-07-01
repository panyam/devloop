package main

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// AirConfig represents the structure of a .air.toml file.
type AirConfig struct {
	Root   string      `toml:"root"`
	TmpDir string      `toml:"tmp_dir"`
	Build  BuildConfig `toml:"build"`
	Log    LogConfig   `toml:"log"`
	Misc   MiscConfig  `toml:"misc"`
	Color  ColorConfig `toml:"color"`
}

type BuildConfig struct {
	Cmd          string   `toml:"cmd"`
	Bin          string   `toml:"bin"`
	PreCmd       []string `toml:"pre_cmd"`
	PostCmd      []string `toml:"post_cmd"`
	IncludeExt   []string `toml:"include_ext"`
	ExcludeDir   []string `toml:"exclude_dir"`
	ExcludeFile  []string `toml:"exclude_file"`
	IncludeDir   []string `toml:"include_dir"`
	ExcludeRegex []string `toml:"exclude_regex"`
	StopOnError  bool     `toml:"stop_on_error"`
	Delay        int      `toml:"delay"`
}

type LogConfig struct {
	LogName        string `toml:"log_name"`
	RedirectOutput bool   `toml:"redirect_output"`
}

type MiscConfig struct {
	CleanOnExit bool `toml:"clean_on_exit"`
}

type ColorConfig struct {
	Main    string `toml:"main"`
	Watcher string `toml:"watcher"`
	Build   string `toml:"build"`
	Runner  string `toml:"runner"`
}

func convertAirToml(inputPath string) error {
	// Read the .air.toml file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Parse the TOML data
	var airConfig AirConfig
	if err := toml.Unmarshal(data, &airConfig); err != nil {
		return fmt.Errorf("failed to parse .air.toml: %w", err)
	}

	// Convert the AirConfig to a devloop Rule
	devloopRule := Rule{
		Name: "Imported from .air.toml",
	}

	// Convert build commands
	if airConfig.Build.Cmd != "" {
		devloopRule.Commands = append(devloopRule.Commands, airConfig.Build.Cmd)
	}
	if len(airConfig.Build.PreCmd) > 0 {
		devloopRule.Commands = append(airConfig.Build.PreCmd, devloopRule.Commands...)
	}
	if len(airConfig.Build.PostCmd) > 0 {
		devloopRule.Commands = append(devloopRule.Commands, airConfig.Build.PostCmd...)
	}

	// Convert watch patterns
	var includePatterns []string
	for _, ext := range airConfig.Build.IncludeExt {
		includePatterns = append(includePatterns, fmt.Sprintf("**/*.%s", ext))
	}
	for _, dir := range airConfig.Build.IncludeDir {
		includePatterns = append(includePatterns, fmt.Sprintf("%s/**/*", dir))
	}
	if len(includePatterns) > 0 {
		devloopRule.Watch = append(devloopRule.Watch, &Matcher{
			Action:   "include",
			Patterns: includePatterns,
		})
	}

	var excludePatterns []string
	for _, dir := range airConfig.Build.ExcludeDir {
		excludePatterns = append(excludePatterns, fmt.Sprintf("%s/**/*", dir))
	}
	excludePatterns = append(excludePatterns, airConfig.Build.ExcludeFile...)
	if len(excludePatterns) > 0 {
		devloopRule.Watch = append(devloopRule.Watch, &Matcher{
			Action:   "exclude",
			Patterns: excludePatterns,
		})
	}

	// Print a warning for unsupported directives
	if len(airConfig.Build.ExcludeRegex) > 0 {
		fmt.Println("Warning: The 'exclude_regex' directive is not supported and will be ignored.")
	}

	// Print the devloop Rule as YAML
	yamlData, err := yaml.Marshal(devloopRule)
	if err != nil {
		return fmt.Errorf("failed to marshal devloop rule to YAML: %w", err)
	}

	fmt.Println(string(yamlData))

	return nil
}
