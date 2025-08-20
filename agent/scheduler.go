package agent

import (
	"context"

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
	lroManager   *LROManager
}

// NewDefaultScheduler creates a new scheduler
func NewDefaultScheduler(orchestrator *Orchestrator, workerPool *WorkerPool, lroManager *LROManager) *DefaultScheduler {
	return &DefaultScheduler{
		orchestrator: orchestrator,
		workerPool:   workerPool,
		lroManager:   lroManager,
	}
}

// ScheduleRule routes the rule to the appropriate execution engine
func (s *DefaultScheduler) ScheduleRule(event *TriggerEvent) error {
	rule := event.Rule
	verbose := s.orchestrator.isVerboseForRule(rule)

	if rule.Lro {
		// Route to LRO Manager for long-running processes
		if verbose {
			utils.LogDevloop("[%s] Scheduler: Routing to LRO manager (trigger: %s)", rule.Name, event.TriggerType)
		}
		return s.lroManager.RestartProcess(rule, event.TriggerType)
	} else {
		// Route to WorkerPool for short-running jobs
		if verbose {
			utils.LogDevloop("[%s] Scheduler: Enqueueing to worker pool (trigger: %s)", rule.Name, event.TriggerType)
		}

		job := NewRuleJob(rule, event.TriggerType, event.Context)
		s.workerPool.EnqueueJob(job)
		return nil // Enqueuing is non-blocking
	}
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
