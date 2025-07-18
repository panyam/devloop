// Package gateway provides the central hub for managing multiple devloop agents.
//
// The gateway package implements a distributed architecture where multiple
// devloop agents can connect to a central gateway service. This enables:
// - Centralized monitoring of multiple projects
// - Unified API access to all connected agents
// - Cross-project coordination and management
//
// # Gateway Service
//
// The gateway service provides both gRPC and HTTP endpoints for:
// - Agent registration and communication
// - Client API access for external tools
// - Real-time log streaming
// - Project status monitoring
//
// # Usage
//
// Start a gateway service:
//
//	gateway := gateway.NewGatewayService(orchestrator)
//	if err := gateway.Start(grpcPort, httpPort); err != nil {
//		log.Fatal(err)
//	}
//	defer gateway.Stop()
//
// Connect agents to the gateway:
//
//	devloop --mode agent --gateway-addr localhost:50051 -c project-a/.devloop.yaml
//	devloop --mode agent --gateway-addr localhost:50051 -c project-b/.devloop.yaml
//
// # Configuration Types
//
// The package defines configuration structures for devloop projects:
// - Config: Main configuration structure
// - Rule: Individual automation rules
// - WatchConfig: File watching patterns
// - Settings: Global project settings
//
// # Example Gateway Setup
//
//	// Start the central gateway
//	devloop --mode gateway --http-port 8080 --grpc-port 50051
//
//	// Connect individual project agents
//	cd project-a && devloop --mode agent --gateway-addr localhost:50051
//	cd project-b && devloop --mode agent --gateway-addr localhost:50051
package agent

import (
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

type Settings struct {
	*pb.Settings
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
	*pb.Rule
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

// resolvePattern resolves a pattern relative to a rule's working directory
func resolvePattern(pattern string, rule *pb.Rule, configPath string) string {
	// If pattern is already absolute, return as-is
	if filepath.IsAbs(pattern) {
		return pattern
	}

	// Determine the base directory for pattern resolution
	var baseDir string
	if rule.WorkDir != "" {
		// If rule has a work_dir, use it as the base
		if filepath.IsAbs(rule.WorkDir) {
			baseDir = rule.WorkDir
		} else {
			// work_dir is relative to config file location
			baseDir = filepath.Join(filepath.Dir(configPath), rule.WorkDir)
		}
	} else {
		// No work_dir specified, use config file directory
		baseDir = filepath.Dir(configPath)
	}

	// Resolve pattern relative to base directory
	return filepath.Join(baseDir, pattern)
}

// Matches checks if the given file path matches the rule's watch criteria.
func RuleMatches(r *pb.Rule, filePath string, configPath string) *pb.RuleMatcher {
	for _, matcher := range r.Watch {
		if MatcherMatches(matcher, filePath, r, configPath) {
			return matcher
		}
	}
	return nil
}

func MatcherMatches(m *pb.RuleMatcher, filePath string, rule *pb.Rule, configPath string) bool {
	for _, pattern := range m.Patterns {
		// Resolve pattern relative to rule's work_dir
		resolvedPattern := resolvePattern(pattern, rule, configPath)
		matched, err := doublestar.Match(resolvedPattern, filePath)
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
	*pb.RuleStatus
}
