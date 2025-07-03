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
	PrefixLogs      bool `yaml:"prefix_logs"`
	PrefixMaxLength int  `yaml:"prefix_max_length"`
}

// Rule defines a single watch-and-run rule.
type Rule struct {
	Name      string            `yaml:"name"`
	Prefix    string            `yaml:"prefix,omitempty"`
	Commands  []string          `yaml:"commands"`
	Watch     []*Matcher        `yaml:"watch"`
	Env       map[string]string `yaml:"env,omitempty"`
	WorkDir   string            `yaml:"workdir,omitempty"`
	RunOnInit bool              `yaml:"run_on_init,omitempty"`
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
