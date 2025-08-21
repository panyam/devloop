package agent

import (
	"sync"
	"time"

	"github.com/panyam/devloop/utils"
)

// RuleExecution represents a rule execution event
type RuleExecution struct {
	RuleName    string
	StartTime   time.Time
	EndTime     time.Time
	TriggerType string // "file_change", "manual", "startup"
}

// RuleTrigger represents a rule trigger event
type RuleTrigger struct {
	RuleName       string
	TriggerTime    time.Time
	TriggerType    string
	TriggeringFile string // File that caused the trigger
}

// CorrelationTracker tracks rule execution and trigger patterns to detect cycles
type CorrelationTracker struct {
	executions []RuleExecution
	triggers   []RuleTrigger
	mutex      sync.RWMutex

	// Configuration
	maxHistoryDuration time.Duration // How long to keep history
	verbose            bool
}

// NewCorrelationTracker creates a new correlation tracker
func NewCorrelationTracker(verbose bool) *CorrelationTracker {
	return &CorrelationTracker{
		executions:         make([]RuleExecution, 0),
		triggers:           make([]RuleTrigger, 0),
		maxHistoryDuration: 10 * time.Minute, // Keep 10 minutes of history
		verbose:            verbose,
	}
}

// RecordRuleStart records when a rule starts executing
func (ct *CorrelationTracker) RecordRuleStart(ruleName, triggerType string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	execution := RuleExecution{
		RuleName:    ruleName,
		StartTime:   time.Now(),
		TriggerType: triggerType,
	}

	ct.executions = append(ct.executions, execution)
	ct.cleanupOldHistory()

	if ct.verbose {
		utils.LogDevloop("CorrelationTracker: Rule %q started (trigger: %s)", ruleName, triggerType)
	}
}

// RecordRuleEnd records when a rule finishes executing
func (ct *CorrelationTracker) RecordRuleEnd(ruleName string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	// Find the most recent execution for this rule and mark it complete
	for i := len(ct.executions) - 1; i >= 0; i-- {
		if ct.executions[i].RuleName == ruleName && ct.executions[i].EndTime.IsZero() {
			ct.executions[i].EndTime = time.Now()

			if ct.verbose {
				duration := ct.executions[i].EndTime.Sub(ct.executions[i].StartTime)
				utils.LogDevloop("CorrelationTracker: Rule %q completed in %v", ruleName, duration)
			}
			break
		}
	}
}

// RecordRuleTrigger records when a rule is triggered by a file change
func (ct *CorrelationTracker) RecordRuleTrigger(ruleName, triggerType, triggeringFile string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	trigger := RuleTrigger{
		RuleName:       ruleName,
		TriggerTime:    time.Now(),
		TriggerType:    triggerType,
		TriggeringFile: triggeringFile,
	}

	ct.triggers = append(ct.triggers, trigger)
	ct.cleanupOldHistory()

	if ct.verbose {
		utils.LogDevloop("CorrelationTracker: Rule %q triggered by %s (type: %s)",
			ruleName, triggeringFile, triggerType)
	}

	// Analyze for potential cycles
	ct.analyzeForCycles(trigger)
}

// analyzeForCycles looks for patterns indicating rule cycles
func (ct *CorrelationTracker) analyzeForCycles(newTrigger RuleTrigger) {
	// Look for recent executions that might have created the triggering file
	recentWindow := 30 * time.Second
	cutoff := newTrigger.TriggerTime.Add(-recentWindow)

	for _, execution := range ct.executions {
		// Skip if execution is too old or still running
		if execution.StartTime.Before(cutoff) || execution.EndTime.IsZero() {
			continue
		}

		// Check if this execution ended recently and could have created the triggering file
		if execution.EndTime.After(cutoff) && execution.EndTime.Before(newTrigger.TriggerTime) {
			// Potential correlation: Rule A executed → File created → Rule B triggered
			utils.LogDevloop("CORRELATION: Rule %q finished at %v, then rule %q triggered by %s at %v (gap: %v)",
				execution.RuleName,
				execution.EndTime.Format("15:04:05.000"),
				newTrigger.RuleName,
				newTrigger.TriggeringFile,
				newTrigger.TriggerTime.Format("15:04:05.000"),
				newTrigger.TriggerTime.Sub(execution.EndTime))

			// Check for self-triggering (potential cycle)
			if execution.RuleName == newTrigger.RuleName {
				utils.LogDevloop("POTENTIAL CYCLE: Rule %q may be triggering itself via file %s",
					newTrigger.RuleName, newTrigger.TriggeringFile)
			}

			// Check for cross-rule cycles
			ct.checkCrossRuleCycle(execution.RuleName, newTrigger.RuleName)
		}
	}
}

// checkCrossRuleCycle looks for A→B→A type cycles
func (ct *CorrelationTracker) checkCrossRuleCycle(executedRule, triggeredRule string) {
	if executedRule == triggeredRule {
		return // Same rule, not cross-rule
	}

	// Look for recent pattern: triggeredRule executed → executedRule triggered
	recentWindow := 60 * time.Second
	cutoff := time.Now().Add(-recentWindow)

	foundReverse := false
	for _, execution := range ct.executions {
		if execution.RuleName == triggeredRule && execution.StartTime.After(cutoff) {
			// Check if executedRule was triggered around the same time
			for _, trigger := range ct.triggers {
				if trigger.RuleName == executedRule &&
					trigger.TriggerTime.After(execution.StartTime) &&
					trigger.TriggerTime.Before(execution.EndTime.Add(10*time.Second)) {
					foundReverse = true
					break
				}
			}
		}
	}

	if foundReverse {
		utils.LogDevloop("CROSS-RULE CYCLE DETECTED: %q ↔ %q triggering each other",
			executedRule, triggeredRule)
	}
}

// cleanupOldHistory removes old entries to prevent memory growth
func (ct *CorrelationTracker) cleanupOldHistory() {
	cutoff := time.Now().Add(-ct.maxHistoryDuration)

	// Clean executions
	newExecutions := make([]RuleExecution, 0, len(ct.executions))
	for _, exec := range ct.executions {
		if exec.StartTime.After(cutoff) {
			newExecutions = append(newExecutions, exec)
		}
	}
	ct.executions = newExecutions

	// Clean triggers
	newTriggers := make([]RuleTrigger, 0, len(ct.triggers))
	for _, trigger := range ct.triggers {
		if trigger.TriggerTime.After(cutoff) {
			newTriggers = append(newTriggers, trigger)
		}
	}
	ct.triggers = newTriggers
}

// GetCorrelationSummary returns recent rule correlations for debugging
func (ct *CorrelationTracker) GetCorrelationSummary() map[string]interface{} {
	ct.mutex.RLock()
	defer ct.mutex.RUnlock()

	return map[string]interface{}{
		"recent_executions": len(ct.executions),
		"recent_triggers":   len(ct.triggers),
		"history_duration":  ct.maxHistoryDuration.String(),
	}
}
