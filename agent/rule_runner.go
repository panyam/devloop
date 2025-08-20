package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	if isRunning {
		r.status.StartTime = tspb.New(time.Now())
	} else {
		r.status.LastBuildTime = tspb.New(time.Now())
		r.status.LastBuildStatus = buildStatus
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

// handleExecutionCompletion handles post-execution logic including pending executions
func (r *RuleRunner) handleExecutionCompletion() {
	// Check if there's a pending execution that should be scheduled
	if r.hasPendingExecution() {
		// Clear the pending flag first
		r.setPendingExecution(false)

		// Schedule the pending execution with debounce delay
		r.debounceMutex.Lock()
		if r.debounceTimer != nil {
			r.debounceTimer.Stop()
		}

		r.debounceTimer = time.AfterFunc(r.debounceDuration, func() {
			if err := r.Execute(); err != nil {
				r.logDevloop("[%s] Error executing pending rule %q: %v", r.rule.Name, r.rule.Name, err)
			}
		})
		r.debounceMutex.Unlock()

		if r.isVerbose() {
			r.logDevloop("[%s] Pending execution scheduled in %v", r.rule.Name, r.debounceDuration)
		}
	}
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
func (r *RuleRunner) logDevloop(format string, args ...interface{}) {
	r.orchestrator.logDevloop(format, args...)
}

// Execute runs the commands for this rule
func (r *RuleRunner) Execute() error {
	// Acquire semaphore slot if parallel execution is limited
	if r.orchestrator.ruleSemaphore != nil {
		r.orchestrator.ruleSemaphore <- struct{}{}
		defer func() { <-r.orchestrator.ruleSemaphore }()

		if r.isVerbose() {
			r.logDevloop("[%s] Acquired execution slot (%d/%d)", r.rule.Name,
				len(r.orchestrator.ruleSemaphore), cap(r.orchestrator.ruleSemaphore))
		}
	}

	r.updateStatus(true, "RUNNING")

	// Set current executing rule for trigger chain tracking
	r.orchestrator.setCurrentExecutingRule(r.rule.Name)
	defer r.orchestrator.clearCurrentExecutingRule()

	if r.isVerbose() {
		utils.LogDevloop("Executing commands for rule %q", r.rule.Name)
	}

	// Terminate any previously running commands
	if err := r.TerminateProcesses(); err != nil {
		utils.LogDevloop("Error terminating previous processes for rule %q: %v", r.rule.Name, err)
	}

	// Get log writer for this rule
	logWriter, err := r.orchestrator.LogManager.GetWriter(r.rule.Name)
	if err != nil {
		return fmt.Errorf("error getting log writer: %w", err)
	}

	// Execute commands sequentially
	var currentCmds []*exec.Cmd
	var lastCmd *exec.Cmd

	for i, cmdStr := range r.rule.Commands {
		r.orchestrator.logDevloop("Running command: %s", cmdStr)
		cmd := createCrossPlatformCommand(cmdStr)

		// Setup output handling
		if err := r.setupCommandOutput(cmd, logWriter); err != nil {
			return fmt.Errorf("failed to setup command output: %w", err)
		}

		// Set platform-specific process attributes
		setSysProcAttr(cmd)

		// Set working directory - default to config file directory if not specified
		workDir := r.rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(r.orchestrator.ConfigPath)
		}
		cmd.Dir = workDir

		// Set environment variables
		cmd.Env = os.Environ() // Inherit parent environment

		// Add environment variables to help subprocesses detect color support
		// Only if suppress_subprocess_colors is disabled (default: false, so colors are preserved)
		suppressColors := r.orchestrator.Config.Settings.SuppressSubprocessColors
		if r.orchestrator.ColorManager != nil && r.orchestrator.ColorManager.IsEnabled() && !suppressColors {
			// Force color output for common tools
			cmd.Env = append(cmd.Env, "FORCE_COLOR=1")       // npm, chalk (Node.js)
			cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=1")    // many CLI tools
			cmd.Env = append(cmd.Env, "COLORTERM=truecolor") // general color support indicator
		}

		// Add rule-specific environment variables
		for key, value := range r.rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		if err := cmd.Start(); err != nil {
			utils.LogDevloop("Command %q failed to start for rule %q: %v", cmdStr, r.rule.Name, err)
			r.updateStatus(false, "FAILED")
			return fmt.Errorf("failed to start command: %w", err)
		}

		currentCmds = append(currentCmds, cmd)

		// For non-last commands, wait for completion before proceeding
		if i < len(r.rule.Commands)-1 {
			if err := cmd.Wait(); err != nil {
				utils.LogDevloop("Command failed for rule %q: %v", r.rule.Name, err)
				r.updateStatus(false, "FAILED")
				return fmt.Errorf("command failed: %w", err)
			}
		} else {
			// This is the last command - let it run in background
			lastCmd = cmd
		}
	}

	// Update running commands
	r.commandsMutex.Lock()
	r.runningCommands = currentCmds
	r.commandsMutex.Unlock()

	// Monitor the last command
	if lastCmd != nil {
		go r.monitorLastCommand(lastCmd)
	} else {
		// No long-running command, mark as successful
		r.updateStatus(false, "SUCCESS")
		r.orchestrator.LogManager.SignalFinished(r.rule.Name)
		utils.LogDevloop("Rule %q commands finished.", r.rule.Name)

		// Handle any pending executions
		r.handleExecutionCompletion()
	}

	return nil
}

// setupCommandOutput configures stdout/stderr for a command
func (r *RuleRunner) setupCommandOutput(cmd *exec.Cmd, logWriter io.Writer) error {
	writers := []io.Writer{os.Stdout, logWriter}

	if r.orchestrator.Config.Settings.PrefixLogs {
		prefix := r.rule.Name
		if r.rule.Prefix != "" {
			prefix = r.rule.Prefix
		}

		// Apply prefix length constraints and left-align the text
		if r.orchestrator.Config.Settings.PrefixMaxLength > 0 {
			if uint32(len(prefix)) > r.orchestrator.Config.Settings.PrefixMaxLength {
				prefix = prefix[:r.orchestrator.Config.Settings.PrefixMaxLength]
			} else {
				// Left-align the prefix within the max length
				totalPadding := int(r.orchestrator.Config.Settings.PrefixMaxLength - uint32(len(prefix)))
				prefix = prefix + strings.Repeat(" ", totalPadding)
			}
		}

		// Use ColoredPrefixWriter for enhanced output with color support
		prefixStr := "[" + prefix + "] "
		coloredWriter := utils.NewColoredPrefixWriter(writers, prefixStr, r.orchestrator.ColorManager, r.rule)
		cmd.Stdout = coloredWriter
		cmd.Stderr = coloredWriter
	} else {
		// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
		// This ensures consistent color handling even without prefixes
		if r.orchestrator.ColorManager != nil && r.orchestrator.ColorManager.IsEnabled() {
			coloredWriter := utils.NewColoredPrefixWriter(writers, "", r.orchestrator.ColorManager, r.rule)
			cmd.Stdout = coloredWriter
			cmd.Stderr = coloredWriter
		} else {
			multiWriter := io.MultiWriter(writers...)
			cmd.Stdout = multiWriter
			cmd.Stderr = multiWriter
		}
	}

	return nil
}

// monitorLastCommand monitors the last (long-running) command
func (r *RuleRunner) monitorLastCommand(cmd *exec.Cmd) {
	err := cmd.Wait()

	if err != nil {
		utils.LogDevloop("Command for rule %q failed: %v", r.rule.Name, err)
		r.updateStatus(false, "FAILED")
	} else {
		r.updateStatus(false, "SUCCESS")
	}

	r.orchestrator.LogManager.SignalFinished(r.rule.Name)
	utils.LogDevloop("Rule %q commands finished.", r.rule.Name)

	// Handle any pending executions
	r.handleExecutionCompletion()
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
