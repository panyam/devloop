package agent

import (
	"context"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// TriggerEvent represents a request to execute a rule
type TriggerEvent struct {
	Rule        *pb.Rule
	TriggerType string // "file_change", "manual", "startup"
	Context     context.Context
}

// Scheduler interface for routing rule execution
type Scheduler interface {
	ScheduleRule(event *TriggerEvent) error
	Start() error
	Stop() error
}

// DefaultScheduler routes rules to appropriate execution engines
type DefaultScheduler struct {
	orchestrator *Orchestrator
	workerPool   *WorkerPool
}

// NewDefaultScheduler creates a new scheduler
func NewDefaultScheduler(orchestrator *Orchestrator, workerPool *WorkerPool) *DefaultScheduler {
	return &DefaultScheduler{
		orchestrator: orchestrator,
		workerPool:   workerPool,
	}
}

// ScheduleRule routes the rule to the appropriate execution engine
func (s *DefaultScheduler) ScheduleRule(event *TriggerEvent) error {
	rule := event.Rule
	verbose := s.orchestrator.isVerboseForRule(rule)

	// Check if rule is disabled by cycle breaker before scheduling
	if s.orchestrator.cycleBreaker.IsRuleDisabled(rule.Name) {
		if verbose {
			utils.LogDevloop("[%s] Scheduler: Rule disabled by cycle breaker, skipping execution", rule.Name)
		}
		return nil // Not an error - just skip execution
	}

	// Check for cycle detection based on actual executions (not file change triggers)
	// This distinguishes between rapid file changes (which debounce) and actual cycles
	if s.orchestrator.isDynamicProtectionEnabled() {
		executionCount := s.orchestrator.GetExecutionCount(rule.Name, 30*time.Second)

		// Only trigger emergency break for excessive actual executions
		// More than 4 executions in 30 seconds indicates a true cycle (debouncing failed)
		if executionCount >= 4 {
			if verbose {
				utils.LogDevloop("[%s] Scheduler: Cycle detected - %d executions in 30s",
					rule.Name, executionCount)
			}

			utils.LogDevloop("[%s] Scheduler: Triggering emergency break due to execution cycle (%d executions)",
				rule.Name, executionCount)
			s.orchestrator.cycleBreaker.TriggerEmergencyBreak(rule.Name)
			return nil // Skip this execution
		}
	}

	// Record that this rule is about to execute (for cycle detection)
	s.orchestrator.RecordExecution(rule.Name)

	// Route all rules to WorkerPool (no more LRO vs non-LRO distinction)
	if verbose {
		utils.LogDevloop("[%s] Scheduler: Enqueueing to worker pool (trigger: %s)", rule.Name, event.TriggerType)
	}

	job := NewRuleJob(rule, event.TriggerType, event.Context)
	s.workerPool.EnqueueJob(job)
	return nil // Enqueuing is non-blocking
}

// Start starts the scheduler (currently no background processes needed)
func (s *DefaultScheduler) Start() error {
	utils.LogDevloop("Scheduler started")
	return nil
}

// Stop stops the scheduler
func (s *DefaultScheduler) Stop() error {
	utils.LogDevloop("Scheduler stopped")
	return nil
}
