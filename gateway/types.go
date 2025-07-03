package gateway

import (
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Config represents the top-level structure of the .devloop.yaml file.
type Config struct {
	Settings Settings `yaml:"settings"`
	Rules    []Rule   `yaml:"rules"`
}

// Settings defines global settings for devloop.
type Settings struct {
	ProjectID            string            `yaml:"project_id,omitempty"`
	PrefixLogs           bool              `yaml:"prefix_logs"`
	PrefixMaxLength      int               `yaml:"prefix_max_length"`
	DefaultDebounceDelay *time.Duration    `yaml:"default_debounce_delay,omitempty"`
	Verbose              bool              `yaml:"verbose,omitempty"`
	ColorLogs            bool              `yaml:"color_logs,omitempty"`
	ColorScheme          string            `yaml:"color_scheme,omitempty"`
	CustomColors         map[string]string `yaml:"custom_colors,omitempty"`
	DefaultWatchAction   string            `yaml:"default_watch_action,omitempty"` // "include" or "exclude"
}

// GetColorLogs returns whether color logs are enabled (implements ColorSettings interface)
func (s *Settings) GetColorLogs() bool {
	return s.ColorLogs
}

// GetColorScheme returns the color scheme (implements ColorSettings interface)
func (s *Settings) GetColorScheme() string {
	return s.ColorScheme
}

// GetCustomColors returns the custom color mappings (implements ColorSettings interface)
func (s *Settings) GetCustomColors() map[string]string {
	return s.CustomColors
}

// Rule defines a single watch-and-run rule.
type Rule struct {
	Name          string            `yaml:"name"`
	Prefix        string            `yaml:"prefix,omitempty"`
	Commands      []string          `yaml:"commands"`
	Watch         []*Matcher        `yaml:"watch"`
	Env           map[string]string `yaml:"env,omitempty"`
	WorkDir       string            `yaml:"workdir,omitempty"`
	RunOnInit     *bool             `yaml:"run_on_init,omitempty"`
	DebounceDelay *time.Duration    `yaml:"debounce_delay,omitempty"`
	Verbose       *bool             `yaml:"verbose,omitempty"`
	Color         string            `yaml:"color,omitempty"`
	DefaultAction string            `yaml:"default_action,omitempty"` // "include" or "exclude"
}

// GetName returns the rule name (implements Nameable interface for color generation)
func (r *Rule) GetName() string {
	return r.Name
}

// GetColor returns the rule color (implements ColorRule interface)
func (r *Rule) GetColor() string {
	return r.Color
}

// GetPrefix returns the rule prefix (implements ColorRule interface)
func (r *Rule) GetPrefix() string {
	return r.Prefix
}

// Matcher defines a single include or exclude directive using glob patterns.
type Matcher struct {
	Action   string   `yaml:"action"` // Should be "include" or "exclude"
	Patterns []string `yaml:"patterns"`
}

// Matches checks if the given file path matches the rule's watch criteria.
func (r *Rule) Matches(filePath string) *Matcher {
	for _, matcher := range r.Watch {
		if matcher.Matches(filePath) {
			return matcher
		}
	}
	return nil
}

func (m *Matcher) Matches(filePath string) bool {
	for _, pattern := range m.Patterns {
		matched, err := doublestar.Match(pattern, filePath)
		if err != nil {
			// Log error but continue with other patterns
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// RuleStatus defines the status of a rule.
type RuleStatus struct {
	ProjectID       string    `json:"project_id"`
	RuleName        string    `json:"rule_name"`
	IsRunning       bool      `json:"is_running"`
	StartTime       time.Time `json:"start_time,omitempty"`
	LastBuildTime   time.Time `json:"last_build_time,omitempty"`
	LastBuildStatus string    `json:"last_build_status,omitempty"` // e.g., "SUCCESS", "FAILED", "IDLE"
}
