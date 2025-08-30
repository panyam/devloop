package agent

import (
	"fmt"
	"io"
	"log"
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
	lastStarted     time.Time
	lastFinished    time.Time
	lastError       error

	// Status tracking
	status      *pb.RuleStatus
	statusMutex sync.RWMutex

	// Configuration
	verbose          bool
	configMutex      sync.RWMutex
	debounceDuration time.Duration

	// Channel-based event handling
	triggerChan chan bool
	killChan    chan struct{} // Kill current execution
	stopChan    chan struct{} // Shutdown signal
	stoppedChan chan struct{} // Shutdown complete
}

// NewRuleRunner creates a new RuleRunner for the given rule
func NewRuleRunner(rule *pb.Rule, orchestrator *Orchestrator) *RuleRunner {
	verbose := orchestrator.isVerboseForRule(rule)

	runner := &RuleRunner{
		rule:            rule,
		lastFinished:    time.Now(),
		lastStarted:     time.Now(),
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
		triggerChan:      make(chan bool, 1),     // Single timer event
		killChan:         make(chan struct{}, 1), // Kill current execution
		stopChan:         make(chan struct{}),    // Shutdown signal
		stoppedChan:      make(chan struct{}),    // Shutdown complete
	}

	return runner
}

// Start begins monitoring for this rule (non-blocking)
func (r *RuleRunner) Start() error {
	// Start the file watcher for this rule
	if err := r.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start file watcher for rule %q: %w", r.rule.Name, err)
	}

	// Start the main event loop
	go r.eventLoop()

	return nil
}

func (r *RuleRunner) IsRunning() bool {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()
	return r.lastStarted.Sub(r.lastFinished) < 0
}

// eventLoop is the main event processing loop for this rule
func (r *RuleRunner) eventLoop() {
	defer close(r.stoppedChan)
	ticker := time.NewTicker(r.debounceDuration)
	defer ticker.Stop()

	// Trigger initial execution if not skipped
	if !r.rule.SkipRunOnInit {
		if r.isVerbose() {
			r.logDevloop("Triggering initial execution")
		}
		r.executeNow("startup", false)
	}

	triggerCount := 0
	for {
		select {
		case <-r.watcher.EventChan():
			// Here we have a simple batching strategy
			// If process is running, see if this time is > startTime + debounceDuration
			if r.lastStarted.Sub(time.Now()) <= r.debounceDuration {
				triggerCount++
			} else {
				triggerCount = 0
				r.executeNow("file_trigger", true)
			}

		case <-r.triggerChan:
			triggerCount = 0
			r.executeNow("manual", true)
			break

		case <-ticker.C:
			// see if there are any pending executions due to debouncing
			if triggerCount > 0 && time.Now().Sub(r.lastStarted) > r.debounceDuration {
				triggerCount = 0
				r.executeNow("file_change", true)
			}

		case <-r.killChan:
		case <-r.stopChan:
			r.TerminateProcesses()
			return
		}
	}
}

func (r *RuleRunner) TriggerManual() {
	log.Println("Manual Trigger Called")
	r.triggerChan <- true
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
			r.logDevloop("Rule failed, retrying in %v (attempt %d/%d) at %s",
				backoffDuration, attempt+1, maxRetries+1,
				nextRetryTime.Format("15:04:05"))

			time.Sleep(backoffDuration)
		}

		if err := r.executeNow("startup_retry", false); err != nil {
			lastErr = err
			r.logDevloop("Rule execution failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		// Success!
		if attempt > 0 {
			r.logDevloop("Rule succeeded on attempt %d/%d", attempt+1, maxRetries+1)
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
	// Signal the event loop to stop
	close(r.stopChan)

	// Stop the file watcher
	if err := r.watcher.Stop(); err != nil {
		r.logDevloop("Error stopping file watcher: %v", err)
	}

	// Wait for event loop to finish
	<-r.stoppedChan

	return nil
}

// Kill terminates currently running processes without stopping the rule
func (r *RuleRunner) Kill() {
	select {
	case r.killChan <- struct{}{}:
	default:
		// Channel full, terminate directly
		r.TerminateProcesses()
	}
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
}

// SetDebounceDelay sets the debounce delay for this rule
func (r *RuleRunner) SetDebounceDelay(duration time.Duration) {
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

// UpdateStatus allows external systems to update this rule's execution status
// This is called by execution engines (WorkerPool, LROManager) to keep RuleRunner in sync
func (r *RuleRunner) UpdateStatus(isRunning bool, buildStatus string) {
	r.updateStatus(isRunning, buildStatus)
}

// TerminateProcesses terminates all running processes for this rule
func (r *RuleRunner) TerminateProcesses() error {
	r.logDevloop("Kill signal received, terminating processes")
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
					r.logDevloop("Process %d for rule already terminated", pid)
				}
				return
			}

			// Try graceful termination first
			if r.isVerbose() {
				r.logDevloop("Terminating process group %d for rule", pid)
			}

			if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
				if !strings.Contains(err.Error(), "no such process") {
					r.logDevloop("Error sending SIGTERM to process group %d for rule: %v", pid, err)
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
					r.logDevloop("Process group %d for rule terminated gracefully", pid)
				}
			case <-time.After(2 * time.Second):
				// Force kill
				r.logDevloop("Force killing process group %d for rule", pid)
				syscall.Kill(-pid, syscall.SIGKILL)
				c.Process.Kill()
				<-done
			}

			// Verify termination
			if err := syscall.Kill(pid, 0); err == nil {
				r.logDevloop("WARNING: Process %d for rule still exists after termination", pid)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}(cmd)
	}

	wg.Wait()
	return nil
}

// executeCommands executes the commands for a rule job
// executeNow immediately sends trigger event to scheduler
func (r *RuleRunner) executeNow(triggerType string, terminate bool) error {
	rule := r.rule
	if r.isVerbose() {
		r.logDevloop("Executing commands for rule")
	}

	// Terminate any previously running commands for this worker
	if terminate {
		if err := r.TerminateProcesses(); err != nil {
			r.logDevloop("Error terminating previous processes: %v", err)
		}
	}

	// Get log writer for this rule
	logWriter, err := r.orchestrator.LogManager.GetWriter(rule.Name)
	if err != nil {
		return fmt.Errorf("error getting log writer: %w", err)
	}

	// Execute commands sequentially
	var currentCmds []*exec.Cmd
	var lastCmd *exec.Cmd
	r.lastStarted = time.Now()
	r.lastError = nil

	for i, cmdStr := range rule.Commands {
		r.logDevloop("Running command: %s", cmdStr)
		cmd := createCrossPlatformCommand(cmdStr)

		// Setup output handling
		if err := r.setupCommandOutput(cmd, logWriter); err != nil {
			r.lastError = err
			return fmt.Errorf("failed to setup command output: %w", err)
		}

		// Set platform-specific process attributes
		setSysProcAttr(cmd)

		// Set working directory - default to config file directory if not specified
		workDir := rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(r.orchestrator.ConfigPath)
		}
		cmd.Dir = workDir

		// Set environment variables
		cmd.Env = os.Environ() // Inherit parent environment

		// Add environment variables to help subprocesses detect color support
		suppressColors := r.orchestrator.Config.Settings.SuppressSubprocessColors
		if r.orchestrator.ColorManager != nil && r.orchestrator.ColorManager.IsEnabled() && !suppressColors {
			cmd.Env = append(cmd.Env, "FORCE_COLOR=1")       // npm, chalk (Node.js)
			cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=1")    // many CLI tools
			cmd.Env = append(cmd.Env, "COLORTERM=truecolor") // general color support indicator
		}

		// Add rule-specific environment variables
		for key, value := range rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		if err := cmd.Start(); err != nil {
			r.logDevloop("Command %q failed to start for rule %q: %v", cmdStr, rule.Name, err)
			return fmt.Errorf("failed to start command: %w", err)
		}

		currentCmds = append(currentCmds, cmd)

		// For non-last commands, wait for completion before proceeding
		if i < len(rule.Commands)-1 {
			if err := cmd.Wait(); err != nil {
				r.logDevloop("Command failed: %v", err)
				return fmt.Errorf("command failed: %w", err)
			}
		} else {
			// This is the last command - let it run and monitor it
			lastCmd = cmd
		}
	}

	// Update running commands
	r.commandsMutex.Lock()
	r.runningCommands = currentCmds
	r.commandsMutex.Unlock()

	// Monitor the last command if it exists
	if lastCmd != nil {
		err := lastCmd.Wait()
		if err != nil {
			r.logDevloop("Last command failed: %v", err)
			return fmt.Errorf("last command failed: %w", err)
		}
	}

	// Signal log manager that rule finished
	r.orchestrator.LogManager.SignalFinished(rule.Name)

	return nil
}

// setupCommandOutput configures stdout/stderr for a command
func (r *RuleRunner) setupCommandOutput(cmd *exec.Cmd, logWriter io.Writer) error {
	rule := r.rule
	writers := []io.Writer{os.Stdout, logWriter}

	if r.orchestrator.Config.Settings.PrefixLogs {
		prefix := rule.Name
		if rule.Prefix != "" {
			prefix = rule.Prefix
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
		coloredWriter := utils.NewColoredPrefixWriter(writers, prefixStr, r.orchestrator.ColorManager, rule)
		cmd.Stdout = coloredWriter
		cmd.Stderr = coloredWriter
	} else {
		// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
		if r.orchestrator.ColorManager != nil && r.orchestrator.ColorManager.IsEnabled() {
			coloredWriter := utils.NewColoredPrefixWriter(writers, "", r.orchestrator.ColorManager, rule)
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
