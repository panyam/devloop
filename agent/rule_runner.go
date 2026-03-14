package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"strings"
	"sync"
	"sync/atomic"
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
	lastError       error

	// Status tracking
	status      *pb.RuleStatus
	statusMutex sync.RWMutex

	// Configuration
	verbose          bool
	configMutex      sync.RWMutex
	debounceDuration time.Duration

	// Channel-based event handling
	triggerChan  chan bool
	execDoneChan chan error
	stopChan     chan struct{} // Shutdown signal
	stoppedChan  chan struct{} // Shutdown complete
	started      atomic.Bool   // Whether eventLoop was started
	stopped      atomic.Bool   // Whether Stop() was already called
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
			LastFinished:    tspb.New(time.Now()),
			LastStarted:     tspb.New(time.Now()),
		},
		debounceDuration: orchestrator.getDebounceDelayForRule(rule),
		verbose:          verbose,
		triggerChan:      make(chan bool, 1), // Single timer event
		execDoneChan:     make(chan error, 1),
		stopChan:         make(chan struct{}), // Shutdown signal
		stoppedChan:      make(chan struct{}), // Shutdown complete
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
	r.started.Store(true)
	go r.eventLoop()

	return nil
}

func (r *RuleRunner) IsRunning() bool {
	r.statusMutex.RLock()
	defer r.statusMutex.RUnlock()
	lastStarted := r.status.LastStarted.AsTime()
	lastFinished := r.status.LastFinished.AsTime()
	return lastStarted.Sub(lastFinished) < 0
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
	var lastEventTime time.Time
	for {
		select {
		case <-r.watcher.EventChan():
			// Record file change event; defer execution to ticker for proper debouncing.
			// This ensures rapid successive events are coalesced into a single execution
			// after the debounce quiet period.
			triggerCount++
			lastEventTime = time.Now()

		case <-r.triggerChan:
			triggerCount = 0
			r.executeNow("manual", true)

		case <-ticker.C:
			// Execute if there are pending events and the debounce quiet period has elapsed
			if triggerCount > 0 && time.Since(lastEventTime) >= r.debounceDuration {
				triggerCount = 0
				r.executeNow("file_change", true)
			}

		case <-r.execDoneChan:
			// called when an execution has finished
			r.logDevloop("Execution complete....")

		case <-r.stopChan:
			// Don't block shutdown on process termination
			r.TerminateProcesses()
			return
		}
	}
}

func (r *RuleRunner) TriggerManual() {
	r.logDevloop("Manual Trigger Called")
	r.triggerChan <- true
}

// executeWithRetry executes the rule with exponential backoff retry logic
/*
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
*/

// Stop terminates all processes and cleans up
func (r *RuleRunner) Stop() error {
	// Prevent double-close of stopChan
	if r.stopped.Swap(true) {
		return nil
	}

	// Signal the event loop to stop
	close(r.stopChan)

	// Stop the file watcher
	if err := r.watcher.Stop(); err != nil {
		r.logDevloop("Error stopping file watcher: %v", err)
	}

	// Only wait for event loop if it was started
	if r.started.Load() {
		<-r.stoppedChan
	}

	// Ensure processes are terminated
	go func() {
		if err := r.TerminateProcesses(); err != nil {
			r.logDevloop("Error during final process termination: %v", err)
		}
	}()

	return nil
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
		LastStarted:     r.status.LastStarted,
		LastFinished:    r.status.LastFinished,
		LastBuildStatus: r.status.LastBuildStatus,
		LastError:       r.status.LastError,
	}
}

// GetRule returns the rule configuration
func (r *RuleRunner) GetRule() *pb.Rule {
	return r.rule
}

// updateStatus updates the rule status and notifies gateway if connected
func (r *RuleRunner) updateStatus(isRunning bool, buildStatus string, errorMsg ...string) {
	r.statusMutex.Lock()
	r.status.IsRunning = isRunning
	r.status.LastBuildStatus = buildStatus

	// Clear error on success or when starting
	if buildStatus == "SUCCESS" || buildStatus == "RUNNING" {
		r.status.LastError = ""
	} else if buildStatus == "FAILED" && len(errorMsg) > 0 {
		r.status.LastError = errorMsg[0]
	}

	if isRunning {
		r.status.LastStarted = tspb.New(time.Now())
	} else {
		r.status.LastFinished = tspb.New(time.Now())
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
	r.orchestrator.logDevloop(r.rule, format, args...)
}

// UpdateStatus allows external systems to update this rule's execution status
// This is called by execution engines to keep RuleRunner in sync
func (r *RuleRunner) UpdateStatus(isRunning bool, buildStatus string, errorMsg ...string) {
	r.updateStatus(isRunning, buildStatus, errorMsg...)
}

// TerminateProcesses terminates all running processes for this rule
func (r *RuleRunner) TerminateProcesses() error {
	r.logDevloop("Terminating running processes for rule %q", r.rule.Name)
	defer r.logDevloop("Terminated ALL running processes for rule %q", r.rule.Name)
	r.commandsMutex.Lock()
	cmds := make([]*exec.Cmd, len(r.runningCommands))
	copy(cmds, r.runningCommands)
	r.runningCommands = []*exec.Cmd{} // Clear the slice
	r.commandsMutex.Unlock()

	if len(cmds) == 0 {
		return nil
	}

	// Only terminate processes that are actually still running
	// Most commands should already be finished since they're executed sequentially
	var activeCommands []*exec.Cmd
	for _, cmd := range cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}

		// Check if process still exists
		if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
			// Process already dead, skip it
			if r.isVerbose() {
				r.logDevloop("Process %d already terminated", cmd.Process.Pid)
			}
			continue
		}

		activeCommands = append(activeCommands, cmd)
	}

	if len(activeCommands) == 0 {
		if r.isVerbose() {
			r.logDevloop("No active processes to terminate for rule %q", r.rule.Name)
		}
		return nil
	}

	if r.isVerbose() {
		r.logDevloop("Terminating %d active processes for rule %q", len(activeCommands), r.rule.Name)
	}

	var wg sync.WaitGroup
	for _, cmd := range activeCommands {
		r.logDevloop("Adding command to kill list: %s", cmd)
		wg.Add(1)
		go func(c *exec.Cmd) {
			defer wg.Done()
			pid := c.Process.Pid

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

				// Wait for c.Wait() to return but with a timeout
				select {
				case <-done:
					// Wait completed
				case <-time.After(1 * time.Second):
					// c.Wait() is still hanging, give up waiting
					r.logDevloop("WARNING: c.Wait() for process %d timed out", pid)
				}
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

// executeNow immediately executes the rule commands
func (r *RuleRunner) executeNow(triggerType string, terminate bool) error {
	rule := r.rule
	if r.isVerbose() {
		r.logDevloop("Starting execution (trigger: %s)", triggerType)
	}

	// Update status to running
	r.updateStatus(true, "RUNNING")

	// Terminate any previously running commands for this worker
	if terminate {
		if err := r.TerminateProcesses(); err != nil {
			r.logDevloop("Error terminating previous processes: %v", err)
		}
	}

	// Get log writer for this rule
	logWriter, err := r.orchestrator.LogManager.GetWriter(rule.Name, rule.AppendOnRestarts)
	if err != nil {
		r.updateStatus(false, "FAILED", err.Error())
		return fmt.Errorf("error getting log writer: %w", err)
	}

	// Execute commands sequentially
	var currentCmds []*exec.Cmd
	var lastCmd *exec.Cmd
	r.status.LastStarted = tspb.New(time.Now())
	r.lastError = nil

	// Start the commands in a goroutine
	go func() {
		var err error
		defer func() {
			r.status.LastFinished = tspb.New(time.Now())
			// Always update status when done (success or failure)
			if r.GetStatus().LastBuildStatus == "RUNNING" {
				r.lastError = err
				if err == nil {
					r.updateStatus(false, "SUCCESS")
				} else {
					r.updateStatus(false, "FAILED")
				}
			}
			// Signal log manager that rule finished with success/error info
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			r.orchestrator.LogManager.SignalFinished(rule.Name, err == nil, errMsg)
			r.execDoneChan <- err
		}()
		configDir := filepath.Dir(r.orchestrator.ConfigPath)

		// Determine reset_env setting (rule overrides global)
		resetEnv := r.orchestrator.Config.Settings.ResetEnv
		if rule.ResetEnv != nil {
			resetEnv = *rule.ResetEnv
		}

		// Determine working directory
		workDir := rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(r.orchestrator.ConfigPath)
		}

		// Capture shell_files env once before the command loop
		hasShellFiles := len(r.orchestrator.Config.Settings.GetShellFiles()) > 0 || len(rule.GetShellFiles()) > 0
		var baseEnv map[string]string
		var currentEnv map[string]string
		if hasShellFiles {
			baseEnv, err = captureShellFileEnv(
				r.orchestrator.Config.Settings.GetShellFiles(),
				rule.GetShellFiles(),
				workDir, configDir,
			)
			if err != nil {
				r.logDevloop("Warning: failed to capture shell file env: %v", err)
				err = nil // non-fatal, continue without captured env
			} else if len(baseEnv) > 0 {
				currentEnv = make(map[string]string, len(baseEnv))
				for k, v := range baseEnv {
					currentEnv[k] = v
				}
			}
		}

		// Create temp file for env cascade between commands
		var envTmpFile *os.File
		if !resetEnv && len(rule.Commands) > 1 {
			envTmpFile, _ = os.CreateTemp("", "devloop-env-*")
			if envTmpFile != nil {
				defer os.Remove(envTmpFile.Name())
				defer envTmpFile.Close()
			}
		}

		for i, cmdStr := range rule.Commands {
			// Expand command aliases and prepend shell file sourcing
			prepared := prepareCommand(cmdStr, r.orchestrator.Config.Settings, rule, configDir)
			if r.isVerbose() && prepared != cmdStr {
				r.logDevloop("Expanded command: %s -> %s", cmdStr, prepared)
			}

			// For non-last commands with cascade enabled, append env capture
			isLastCmd := i == len(rule.Commands)-1
			if !isLastCmd && !resetEnv && envTmpFile != nil {
				prepared = prepared + " && env -0 > " + envTmpFile.Name()
			}

			r.logDevloop("Running command: %s", cmdStr)
			cmd := createCrossPlatformCommand(prepared)

			// Setup output handling
			if err = r.setupCommandOutput(cmd, logWriter); err != nil {
				r.lastError = err
				return
			}

			// Set platform-specific process attributes
			setSysProcAttr(cmd)

			cmd.Dir = workDir

			// Set environment variables
			cmd.Env = os.Environ() // Inherit parent environment

			// Inject captured shell_files env
			for k, v := range currentEnv {
				cmd.Env = append(cmd.Env, k+"="+v)
			}

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

			if err = cmd.Start(); err != nil {
				r.logDevloop("Command %q failed to start for rule: %v", cmdStr, err)
				r.updateStatus(false, "FAILED", err.Error())
				return
			}

			currentCmds = append(currentCmds, cmd)

			// For non-last commands, wait for completion before proceeding
			if !isLastCmd {
				if err = cmd.Wait(); err != nil {
					r.logDevloop("Command failed: %v", err)
					r.updateStatus(false, "FAILED", err.Error())
					return
				}

				// Parse env cascade from temp file (if cascading)
				if !resetEnv && envTmpFile != nil {
					newEnv, parseErr := parseEnvFile(envTmpFile.Name())
					if parseErr == nil && len(newEnv) > 0 {
						osEnv := envSliceToMap(os.Environ())
						currentEnv = envDiff(osEnv, newEnv)
					}
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
			err = lastCmd.Wait()
			if err != nil {
				r.logDevloop("Last command failed: %v", err)
				r.updateStatus(false, "FAILED", err.Error())
				return
			}
		}

	}()

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
