package agent

import (
	"os"
	"time"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/protos/devloop/v1"
)

// NewOrchestratorForTesting creates an orchestrator based on the DEVLOOP_ORCHESTRATOR_VERSION environment variable.
// If DEVLOOP_ORCHESTRATOR_VERSION=v1, it returns the original Orchestrator, otherwise it returns OrchestratorV2.
// This is used by tests to verify both implementations work correctly.
func NewOrchestratorForTesting(configPath string, gatewayAddr string) (OrchestratorInterface, error) {
	version := os.Getenv("DEVLOOP_ORCHESTRATOR_VERSION")

	switch version {
	case "v1":
		return NewOrchestrator(configPath, gatewayAddr)
	default:
		// Default to v2 (new default)
		return NewOrchestratorV2(configPath, gatewayAddr)
	}
}

// OrchestratorInterface defines the common interface for both orchestrator implementations.
// This allows tests to work with either version.
type OrchestratorInterface interface {
	Start() error
	Stop() error
	GetConfig() *gateway.Config
	GetRuleStatus(ruleName string) (*gateway.RuleStatus, bool)
	TriggerRule(ruleName string) error
	GetWatchedPaths() []string
	ReadFileContent(path string) ([]byte, error)
	StreamLogs(ruleName string, filter string, stream pb.GatewayClientService_StreamLogsClientServer) error
	ProjectRoot() string

	// Configuration methods
	SetGlobalDebounceDelay(duration time.Duration)
	SetRuleDebounceDelay(ruleName string, duration time.Duration) error
	SetVerbose(verbose bool)
	SetRuleVerbose(ruleName string, verbose bool) error
}
