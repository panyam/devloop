package agent

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/panyam/devloop/gateway"
	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// RuleRunner manages the execution lifecycle of a single rule
type RuleRunner interface {
	// Lifecycle management
	Start() error   // Start monitoring and execution
	Stop() error    // Stop all processes and cleanup
	Restart() error // Restart all processes
	Execute() error // Execute the rule's commands

	// Status and state
	IsRunning() bool
	GetStatus() *gateway.RuleStatus
	GetRule() gateway.Rule

	// Debouncing
	TriggerDebounced() // Called when a file change is detected

	// Process management
	TerminateProcesses() error

	// Configuration
	SetDebounceDelay(duration time.Duration)
	SetVerbose(verbose bool)
}

// ruleRunner is the concrete implementation of RuleRunner
type ruleRunner struct {
	rule         gateway.Rule
	orchestrator *OrchestratorV2 // Back reference for config, logging, etc.

	// Process management
	runningCommands []*exec.Cmd
	commandsMutex   sync.RWMutex

	// Status tracking
	status      *gateway.RuleStatus
	statusMutex sync.RWMutex

	// Debouncing
	debounceTimer    *time.Timer
	debounceMutex    sync.Mutex
	debounceDuration time.Duration

	// Configuration
	verbose     bool
	configMutex sync.RWMutex

	// Control
	stopChan    chan struct{}
	stoppedChan chan struct{}
}

// NewRuleRunner creates a new RuleRunner for the given rule
func NewRuleRunner(rule gateway.Rule, orchestrator *OrchestratorV2) RuleRunner {
	runner := &ruleRunner{
		rule:            rule,
		orchestrator:    orchestrator,
		runningCommands: make([]*exec.Cmd, 0),
		status: &gateway.RuleStatus{
			ProjectID:       orchestrator.projectID,
			RuleName:        rule.Name,
			IsRunning:       false,
			LastBuildStatus: "IDLE",
		},
		debounceDuration: orchestrator.getDebounceDelayForRule(rule),
		verbose:          orchestrator.isVerboseForRule(rule),
		stopChan:         make(chan struct{}),
		stoppedChan:      make(chan struct{}),
	}
	return runner
}

// Start begins monitoring for this rule
func (r *ruleRunner) Start() error {
	// If run_on_init is true (default), execute immediately
	shouldRunOnInit := true // Default to true
	if r.rule.RunOnInit != nil {
		shouldRunOnInit = *r.rule.RunOnInit
	}

	if shouldRunOnInit {
		if r.isVerbose() {
			log.Printf("[%s] Executing rule %q on initialization (run_on_init: true)", r.rule.Name, r.rule.Name)
		}
		if err := r.Execute(); err != nil {
			return fmt.Errorf("failed to execute rule %q on init: %w", r.rule.Name, err)
		}
	}
	return nil
}

// Stop terminates all processes and cleans up
func (r *ruleRunner) Stop() error {
	close(r.stopChan)

	// Terminate all running processes
	if err := r.TerminateProcesses(); err != nil {
		return fmt.Errorf("failed to terminate processes for rule %q: %w", r.rule.Name, err)
	}

	// Cancel any pending debounce timer
	r.debounceMutex.Lock()
	if r.debounceTimer != nil {
		r.debounceTimer.Stop()
	}
	r.debounceMutex.Unlock()

	close(r.stoppedChan)
	return nil
}

// Restart stops and starts the rule
func (r *ruleRunner) Restart() error {
	if err := r.TerminateProcesses(); err != nil {
		return err
	}
	return r.Execute()
}

// IsRunning returns true if any commands are currently running
func (r *ruleRunner) IsRunning() bool {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()
	return r.status.IsRunning
}

// GetStatus returns a copy of the current status
func (r *ruleRunner) GetStatus() *gateway.RuleStatus {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	return &gateway.RuleStatus{
		ProjectID:       r.status.ProjectID,
		RuleName:        r.status.RuleName,
		IsRunning:       r.status.IsRunning,
		StartTime:       r.status.StartTime,
		LastBuildTime:   r.status.LastBuildTime,
		LastBuildStatus: r.status.LastBuildStatus,
	}
}

// GetRule returns the rule configuration
func (r *ruleRunner) GetRule() gateway.Rule {
	return r.rule
}

// TriggerDebounced triggers execution after debounce period
func (r *ruleRunner) TriggerDebounced() {
	r.debounceMutex.Lock()
	defer r.debounceMutex.Unlock()

	// Cancel existing timer if any
	if r.debounceTimer != nil {
		r.debounceTimer.Stop()
	}

	// Set new timer
	r.debounceTimer = time.AfterFunc(r.debounceDuration, func() {
		if err := r.Execute(); err != nil {
			log.Printf("[%s] Error executing rule %q: %v", r.rule.Name, r.rule.Name, err)
		}
	})

	if r.isVerbose() {
		log.Printf("[%s] File change detected, execution scheduled in %v", r.rule.Name, r.debounceDuration)
	}
}

// updateStatus updates the rule status and notifies gateway if connected
func (r *ruleRunner) updateStatus(isRunning bool, buildStatus string) {
	r.statusMutex.Lock()
	r.status.IsRunning = isRunning
	if isRunning {
		r.status.StartTime = time.Now()
	} else {
		r.status.LastBuildTime = time.Now()
		r.status.LastBuildStatus = buildStatus
	}
	status := r.status // Copy while holding lock
	r.statusMutex.Unlock()

	// Notify gateway if connected
	if r.orchestrator.gatewayStream != nil {
		statusMsg := &pb.DevloopMessage{
			Content: &pb.DevloopMessage_UpdateRuleStatusRequest{
				UpdateRuleStatusRequest: &pb.UpdateRuleStatusRequest{
					RuleStatus: &pb.RuleStatus{
						ProjectId:       status.ProjectID,
						RuleName:        status.RuleName,
						IsRunning:       status.IsRunning,
						StartTime:       status.StartTime.UnixMilli(),
						LastBuildTime:   status.LastBuildTime.UnixMilli(),
						LastBuildStatus: status.LastBuildStatus,
					},
				},
			},
		}

		select {
		case r.orchestrator.gatewaySendChan <- statusMsg:
		default:
			log.Printf("[%s] Failed to send rule status update: channel full", r.rule.Name)
		}
	}
}

// SetDebounceDelay sets the debounce delay for this rule
func (r *ruleRunner) SetDebounceDelay(duration time.Duration) {
	r.debounceMutex.Lock()
	defer r.debounceMutex.Unlock()
	r.debounceDuration = duration
}

// SetVerbose sets the verbose flag for this rule
func (r *ruleRunner) SetVerbose(verbose bool) {
	r.configMutex.Lock()
	defer r.configMutex.Unlock()
	r.verbose = verbose
}

// isVerbose returns whether verbose logging is enabled for this rule
func (r *ruleRunner) isVerbose() bool {
	r.configMutex.RLock()
	defer r.configMutex.RUnlock()
	return r.verbose
}
