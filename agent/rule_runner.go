package agent

import (
	"context"
	"fmt"
	"os/exec"

	"strings"
	"sync"
	"syscall"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// RuleRunner manages the execution lifecycle of a single rule
type RuleRunner struct {
	rule         *pb.Rule
	orchestrator *Orchestrator // Back reference for config, logging, etc.

	// File watching (per-rule)
	watcher *Watcher

	// Process management
	runningCommands []*exec.Cmd
	commandsMutex   sync.RWMutex

	// Status tracking
	status      *pb.RuleStatus
	statusMutex sync.RWMutex

	// Debouncing
	debounceTimer    *time.Timer
	debounceMutex    sync.Mutex
	debounceDuration time.Duration

	// Pending execution management - prevents queuing builds during execution
	pendingExecution      bool
	pendingExecutionMutex sync.Mutex

	// Configuration
	verbose     bool
	configMutex sync.RWMutex

	// Trigger tracking for rate limiting
	triggerTracker *TriggerTracker

	// Control
	stopChan    chan struct{}
	stoppedChan chan struct{}
}

// NewRuleRunner creates a new RuleRunner for the given rule
func NewRuleRunner(rule *pb.Rule, orchestrator *Orchestrator) *RuleRunner {
	verbose := orchestrator.isVerboseForRule(rule)

	runner := &RuleRunner{
		rule:            rule,
		orchestrator:    orchestrator,
		watcher:         NewWatcher(rule, orchestrator.ConfigPath, verbose),
		runningCommands: make([]*exec.Cmd, 0),
		status: &pb.RuleStatus{
			ProjectId:       orchestrator.projectID,
			RuleName:        rule.Name,
			IsRunning:       false,
			LastBuildStatus: "IDLE",
		},
		debounceDuration: orchestrator.getDebounceDelayForRule(rule),
		verbose:          verbose,
		triggerTracker:   NewTriggerTracker(),
		stopChan:         make(chan struct{}),
		stoppedChan:      make(chan struct{}),
	}

	// Set up the watcher event handler to trigger rule execution
	runner.watcher.SetEventHandler(func(filePath string) {
		runner.handleFileChange(filePath)
	})

	return runner
}

// Start begins monitoring for this rule (non-blocking)
func (r *RuleRunner) Start() error {
	// Start the file watcher for this rule
	if err := r.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start file watcher for rule %q: %w", r.rule.Name, err)
	}

	// Skip execution if skip_run_on_init is true
	if r.rule.SkipRunOnInit {
		if r.isVerbose() {
			utils.LogDevloop("[%s] Skipping initialization execution (skip_run_on_init: true)", r.rule.Name)
		}
		return nil
	}

	// Start background retry logic - don't block orchestrator startup
	if r.isVerbose() {
		utils.LogDevloop("[%s] Starting background initialization for rule %q", r.rule.Name, r.rule.Name)
	}

	// Run initialization in background goroutine
	go r.startWithRetry()

	return nil
}

// startWithRetry runs the initialization retry logic in background
func (r *RuleRunner) startWithRetry() {
	// Use debounced execution for startup to coordinate with file change triggers
	// This ensures startup and file change executions are properly serialized
	r.triggerDebouncedWithRetry(true) // bypass rate limiting for startup
}

// executeWithRetry executes the rule with exponential backoff retry logic
func (r *RuleRunner) executeWithRetry() error {
	maxRetries := r.getMaxInitRetries()
	backoffBase := r.getInitRetryBackoffBase()

	var lastErr error
	for attempt := uint32(0); attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration: backoffBase * 2^(attempt-1)
			backoffMs := backoffBase * (1 << (attempt - 1))
			backoffDuration := time.Duration(backoffMs) * time.Millisecond

			nextRetryTime := time.Now().Add(backoffDuration)
			utils.LogDevloop("[%s] Rule %q failed, retrying in %v (attempt %d/%d) at %s",
				r.rule.Name, r.rule.Name, backoffDuration, attempt+1, maxRetries+1,
				nextRetryTime.Format("15:04:05"))

			time.Sleep(backoffDuration)
		}

		if err := r.Execute(); err != nil {
			lastErr = err
			utils.LogDevloop("[%s] Rule %q execution failed (attempt %d/%d): %v",
				r.rule.Name, r.rule.Name, attempt+1, maxRetries+1, err)
			continue
		}

		// Success!
		if attempt > 0 {
			utils.LogDevloop("[%s] Rule %q succeeded on attempt %d/%d",
				r.rule.Name, r.rule.Name, attempt+1, maxRetries+1)
		}
		return nil
	}

	return fmt.Errorf("rule %q failed after %d attempts: %w", r.rule.Name, maxRetries+1, lastErr)
}

// getMaxInitRetries returns the max retry count with default fallback
func (r *RuleRunner) getMaxInitRetries() uint32 {
	if r.rule.MaxInitRetries == 0 {
		return 10 // default
	}
	return r.rule.MaxInitRetries
}

// getInitRetryBackoffBase returns the base backoff duration with default fallback
func (r *RuleRunner) getInitRetryBackoffBase() uint64 {
	if r.rule.InitRetryBackoffBase == 0 {
		return 3000 // default 3000ms
	}
	return r.rule.InitRetryBackoffBase
}

// Stop terminates all processes and cleans up
func (r *RuleRunner) Stop() error {
	close(r.stopChan)

	// Stop the file watcher
	if err := r.watcher.Stop(); err != nil {
		utils.LogDevloop("[%s] Error stopping file watcher: %v", r.rule.Name, err)
	}

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
func (r *RuleRunner) Restart() error {
	if err := r.TerminateProcesses(); err != nil {
		return err
	}
	return r.Execute()
}

// IsRunning returns true if any commands are currently running
func (r *RuleRunner) IsRunning() bool {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()
	return r.status.IsRunning
}

// GetStatus returns a copy of the current status
func (r *RuleRunner) GetStatus() *pb.RuleStatus {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	return &pb.RuleStatus{
		ProjectId:       r.status.ProjectId,
		RuleName:        r.status.RuleName,
		IsRunning:       r.status.IsRunning,
		StartTime:       r.status.StartTime,
		LastBuildTime:   r.status.LastBuildTime,
		LastBuildStatus: r.status.LastBuildStatus,
	}
}

// GetRule returns the rule configuration
func (r *RuleRunner) GetRule() *pb.Rule {
	return r.rule
}

// handleFileChange handles file change events from the watcher
func (r *RuleRunner) handleFileChange(filePath string) {
	if r.isVerbose() {
		utils.LogDevloop("[%s] File change detected: %s", r.rule.Name, filePath)
	}

	// Trigger debounced execution
	r.TriggerDebounced()
}

// TriggerDebounced triggers execution after debounce period
func (r *RuleRunner) TriggerDebounced() {
	r.TriggerDebouncedWithOptions(false)
}

// triggerDebouncedWithRetry triggers execution with retry logic after debounce period
func (r *RuleRunner) triggerDebouncedWithRetry(bypassRateLimit bool) {
	r.debounceMutex.Lock()
	defer r.debounceMutex.Unlock()

	// Check if rate limiting is enabled and if we're exceeding limits BEFORE recording trigger
	if !bypassRateLimit && r.orchestrator.isDynamicProtectionEnabled() {
		// Check if we're in backoff period
		if r.triggerTracker.IsInBackoff() {
			level := r.triggerTracker.GetBackoffLevel()
			r.logDevloop("[%s] In backoff period (level %d), skipping execution", r.rule.Name, level)
			return
		}

		// Check if we're exceeding rate limits (check before recording new trigger)
		if r.isRateLimited() {
			r.triggerTracker.SetBackoff()
			rate := r.triggerTracker.GetTriggerRate()
			level := r.triggerTracker.GetBackoffLevel()
			r.logDevloop("[%s] Rate limit exceeded (%.1f triggers/min), entering backoff (level %d)", r.rule.Name, rate, level)
			return
		}

		// Reset backoff if we're not rate limited
		r.triggerTracker.ResetBackoff()
	}

	// Record the trigger for rate limiting (only after passing rate limit checks)
	r.triggerTracker.RecordTrigger()

	// Cancel existing timer if any
	if r.debounceTimer != nil {
		r.debounceTimer.Stop()
	}

	// Set new timer with retry logic for startup
	// Use very short delay for startup to avoid interfering with file change debouncing
	startupDelay := 1 * time.Millisecond
	if !bypassRateLimit {
		startupDelay = r.debounceDuration // Use normal debounce delay for file changes
	}
	r.debounceTimer = time.AfterFunc(startupDelay, func() {
		err := r.executeWithRetry()
		if err != nil {
			// Check if we should exit on failed init
			if r.rule.ExitOnFailedInit {
				utils.LogDevloop("[%s] Critical rule %q failed after all retries - signaling devloop to exit", r.rule.Name, r.rule.Name)
				// Send critical failure to orchestrator
				select {
				case r.orchestrator.criticalFailure <- r.rule.Name:
					// Critical failure sent successfully
				default:
					// Channel is full, log error
					utils.LogDevloop("[%s] Failed to send critical failure for rule %q - channel full", r.rule.Name, r.rule.Name)
				}
				return
			}
			// Log the failure but continue (devloop continues)
			utils.LogDevloop("[%s] Rule %q failed startup after all retries, but continuing devloop (exit_on_failed_init: false)", r.rule.Name, r.rule.Name)
		}
	})

	if r.isVerbose() {
		triggerType := "File change"
		if bypassRateLimit {
			triggerType = "Startup trigger"
		}
		r.logDevloop("[%s] %s detected, execution scheduled in %v", r.rule.Name, triggerType, r.debounceDuration)
	}
}

// TriggerDebouncedWithOptions triggers execution after debounce period with options
func (r *RuleRunner) TriggerDebouncedWithOptions(bypassRateLimit bool) {
	r.debounceMutex.Lock()
	defer r.debounceMutex.Unlock()

	// Check if rate limiting is enabled and if we're exceeding limits BEFORE recording trigger
	if !bypassRateLimit && r.orchestrator.isDynamicProtectionEnabled() {
		// Check if we're in backoff period
		if r.triggerTracker.IsInBackoff() {
			level := r.triggerTracker.GetBackoffLevel()
			r.logDevloop("[%s] In backoff period (level %d), skipping execution", r.rule.Name, level)
			return
		}

		// Check if we're exceeding rate limits (check before recording new trigger)
		if r.isRateLimited() {
			r.triggerTracker.SetBackoff()
			rate := r.triggerTracker.GetTriggerRate()
			level := r.triggerTracker.GetBackoffLevel()
			r.logDevloop("[%s] Rate limit exceeded (%.1f triggers/min), entering backoff (level %d)", r.rule.Name, rate, level)
			return
		}

		// Reset backoff if we're not rate limited
		r.triggerTracker.ResetBackoff()
	}

	// Record the trigger for rate limiting (only after passing rate limit checks)
	r.triggerTracker.RecordTrigger()

	// Cancel existing timer if any
	if r.debounceTimer != nil {
		r.debounceTimer.Stop()
	}

	// Check if rule is currently running
	isRunning := r.isCurrentlyRunning()

	if isRunning {
		// Rule is currently executing - mark for pending execution after completion
		wasPending := r.hasPendingExecution()
		r.setPendingExecution(true)

		if r.isVerbose() {
			triggerType := "File change"
			if bypassRateLimit {
				triggerType = "Manual trigger"
			}
			if wasPending {
				r.logDevloop("[%s] %s detected while running, replacing previous pending execution", r.rule.Name, triggerType)
			} else {
				r.logDevloop("[%s] %s detected while running, scheduling execution after completion", r.rule.Name, triggerType)
			}
		}
	} else {
		// Rule is not running - use normal debouncing
		r.debounceTimer = time.AfterFunc(r.debounceDuration, func() {
			if err := r.Execute(); err != nil {
				r.logDevloop("[%s] Error executing rule %q: %v", r.rule.Name, r.rule.Name, err)
			}
		})

		if r.isVerbose() {
			triggerType := "File change"
			if bypassRateLimit {
				triggerType = "Manual trigger"
			}
			r.logDevloop("[%s] %s detected, execution scheduled in %v", r.rule.Name, triggerType, r.debounceDuration)
		}
	}
}

// isRateLimited checks if the rule is currently rate limited
func (r *RuleRunner) isRateLimited() bool {
	settings := r.orchestrator.getCycleDetectionSettings()
	maxTriggers := settings.MaxTriggersPerMinute

	if maxTriggers == 0 {
		return false // No rate limiting
	}

	currentRate := r.triggerTracker.GetTriggerCount(time.Minute)
	return uint32(currentRate) > maxTriggers
}

// GetTriggerRate returns the current trigger rate for this rule
func (r *RuleRunner) GetTriggerRate() float64 {
	return r.triggerTracker.GetTriggerRate()
}

// GetTriggerCount returns the trigger count within the specified duration
func (r *RuleRunner) GetTriggerCount(duration time.Duration) int {
	return r.triggerTracker.GetTriggerCount(duration)
}

// CleanupTriggerHistory removes old trigger records to prevent memory growth
func (r *RuleRunner) CleanupTriggerHistory() {
	// Keep triggers for up to 5 minutes to allow for rate limiting calculations
	r.triggerTracker.CleanupOldTriggers(5 * time.Minute)
}

// updateStatus updates the rule status and notifies gateway if connected
func (r *RuleRunner) updateStatus(isRunning bool, buildStatus string) {
	r.statusMutex.Lock()
	r.status.IsRunning = isRunning
	r.status.LastBuildStatus = buildStatus
	if isRunning {
		r.status.StartTime = tspb.New(time.Now())
	} else {
		r.status.LastBuildTime = tspb.New(time.Now())
	}
	r.statusMutex.Unlock()

	// TODO - Add an event emitter if needed
	/*
		// Notify gateway if connected
		if r.orchestrator.gatewayStream != nil {
			statusMsg := &pb.DevloopMessage{
				Content: &pb.DevloopMessage_UpdateRuleStatusRequest{
					UpdateRuleStatusRequest: &pb.UpdateRuleStatusRequest{
						RuleStatus: &pb.RuleStatus{
							ProjectId:       status.ProjectId,
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
				utils.LogDevloop("[%s] Failed to send rule status update: channel full", r.rule.Name)
			}
		}
	*/
}

// setPendingExecution marks that an execution should happen after the current one completes
func (r *RuleRunner) setPendingExecution(pending bool) {
	r.pendingExecutionMutex.Lock()
	defer r.pendingExecutionMutex.Unlock()
	r.pendingExecution = pending
}

// hasPendingExecution returns whether there's a pending execution scheduled
func (r *RuleRunner) hasPendingExecution() bool {
	r.pendingExecutionMutex.Lock()
	defer r.pendingExecutionMutex.Unlock()
	return r.pendingExecution
}

// isCurrentlyRunning checks if the rule is currently executing
func (r *RuleRunner) isCurrentlyRunning() bool {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()
	return r.status.IsRunning
}

// SetDebounceDelay sets the debounce delay for this rule
func (r *RuleRunner) SetDebounceDelay(duration time.Duration) {
	r.debounceMutex.Lock()
	defer r.debounceMutex.Unlock()
	r.debounceDuration = duration
}

// SetVerbose sets the verbose flag for this rule
func (r *RuleRunner) SetVerbose(verbose bool) {
	r.configMutex.Lock()
	defer r.configMutex.Unlock()
	r.verbose = verbose
}

// isVerbose returns whether verbose logging is enabled for this rule
func (r *RuleRunner) isVerbose() bool {
	r.configMutex.RLock()
	defer r.configMutex.RUnlock()
	return r.verbose
}

// logDevloop logs a message using the orchestrator's logging mechanism
func (r *RuleRunner) logDevloop(format string, args ...any) {
	r.orchestrator.logDevloop(format, args...)
}

// Execute sends a trigger event to the scheduler for execution
func (r *RuleRunner) Execute() error {
	// Create trigger event
	event := &TriggerEvent{
		Rule:        r.rule,
		TriggerType: "file_change", // TODO: Could be made more specific
		Context:     context.Background(),
	}

	if r.isVerbose() {
		r.logDevloop("[%s] Sending trigger event to scheduler", r.rule.Name)
	}

	// Send to scheduler for routing
	return r.orchestrator.scheduler.ScheduleRule(event)
}

// UpdateStatus allows external systems to update this rule's execution status
// This is called by execution engines (WorkerPool, LROManager) to keep RuleRunner in sync
func (r *RuleRunner) UpdateStatus(isRunning bool, buildStatus string) {
	r.updateStatus(isRunning, buildStatus)
}

// TerminateProcesses terminates all running processes for this rule
func (r *RuleRunner) TerminateProcesses() error {
	r.commandsMutex.Lock()
	cmds := make([]*exec.Cmd, len(r.runningCommands))
	copy(cmds, r.runningCommands)
	r.runningCommands = []*exec.Cmd{} // Clear the slice
	r.commandsMutex.Unlock()

	if len(cmds) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for _, cmd := range cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}

		wg.Add(1)
		go func(c *exec.Cmd) {
			defer wg.Done()
			pid := c.Process.Pid

			// Check if process still exists
			if err := syscall.Kill(pid, 0); err != nil {
				// Process already dead
				if r.isVerbose() {
					utils.LogDevloop("Process %d for rule %q already terminated", pid, r.rule.Name)
				}
				return
			}

			// Try graceful termination first
			if r.isVerbose() {
				utils.LogDevloop("Terminating process group %d for rule %q", pid, r.rule.Name)
			}

			if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
				if !strings.Contains(err.Error(), "no such process") {
					utils.LogDevloop("Error sending SIGTERM to process group %d for rule %q: %v",
						pid, r.rule.Name, err)
				}
			}

			// Give it time to exit gracefully
			done := make(chan bool, 1)
			go func() {
				c.Wait()
				done <- true
			}()

			select {
			case <-done:
				if r.isVerbose() {
					utils.LogDevloop("Process group %d for rule %q terminated gracefully",
						pid, r.rule.Name)
				}
			case <-time.After(2 * time.Second):
				// Force kill
				utils.LogDevloop("Force killing process group %d for rule %q", pid, r.rule.Name)
				syscall.Kill(-pid, syscall.SIGKILL)
				c.Process.Kill()
				<-done
			}

			// Verify termination
			if err := syscall.Kill(pid, 0); err == nil {
				utils.LogDevloop("WARNING: Process %d for rule %q still exists after termination",
					pid, r.rule.Name)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}(cmd)
	}

	wg.Wait()
	return nil
}
