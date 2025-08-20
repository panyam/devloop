package agent

import (
	"context"
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
)

// RuleJob represents a job to execute a rule
type RuleJob struct {
	Rule        *pb.Rule
	TriggerType string // "file_change", "manual", "startup"
	Context     context.Context
	JobID       string // Unique job identifier
	CreatedAt   time.Time
}

// NewRuleJob creates a new rule job with unique ID
func NewRuleJob(rule *pb.Rule, triggerType string, ctx context.Context) *RuleJob {
	return &RuleJob{
		Rule:        rule,
		TriggerType: triggerType,
		Context:     ctx,
		JobID:       fmt.Sprintf("%s-%d", rule.Name, time.Now().UnixNano()),
		CreatedAt:   time.Now(),
	}
}

// Worker represents a single worker that executes rule jobs
type Worker struct {
	id           int
	orchestrator *Orchestrator

	// Process management
	runningCommands []*exec.Cmd
	commandsMutex   sync.RWMutex

	// Worker lifecycle
	stopChan    chan struct{}
	stoppedChan chan struct{}

	// Current job tracking
	currentJob   *RuleJob
	currentMutex sync.RWMutex
}

// NewWorker creates a new worker instance
func NewWorker(id int, orchestrator *Orchestrator) *Worker {
	return &Worker{
		id:              id,
		orchestrator:    orchestrator,
		runningCommands: make([]*exec.Cmd, 0),
		stopChan:        make(chan struct{}),
		stoppedChan:     make(chan struct{}),
	}
}

// Start begins the worker's job processing loop
func (w *Worker) Start(jobQueue <-chan *RuleJob, workerPool *WorkerPool) {
	utils.LogDevloop("Worker %d starting", w.id)

	go func() {
		defer close(w.stoppedChan)

		for {
			select {
			case <-w.stopChan:
				utils.LogDevloop("Worker %d stopping", w.id)
				w.terminateCurrentJob()
				return

			case job := <-jobQueue:
				if job == nil {
					continue // Channel closed
				}

				w.executeJob(job, workerPool)
			}
		}
	}()
}

// Stop gracefully stops the worker
func (w *Worker) Stop() error {
	close(w.stopChan)

	// Wait for worker to stop with timeout
	select {
	case <-w.stoppedChan:
		utils.LogDevloop("Worker %d stopped gracefully", w.id)
	case <-time.After(5 * time.Second):
		utils.LogDevloop("Worker %d stop timeout, forcing termination", w.id)
		w.terminateCurrentJob()
	}

	return nil
}

// executeJob executes a single rule job
func (w *Worker) executeJob(job *RuleJob, workerPool *WorkerPool) {
	// Set current job
	w.currentMutex.Lock()
	w.currentJob = job
	w.currentMutex.Unlock()

	defer func() {
		// Clear current job
		w.currentMutex.Lock()
		w.currentJob = nil
		w.currentMutex.Unlock()

		// Notify worker pool of completion
		workerPool.CompleteJob(job, w)
	}()

	ruleName := job.Rule.Name
	verbose := w.orchestrator.isVerboseForRule(job.Rule)

	if verbose {
		utils.LogDevloop("Worker %d executing job for rule %q (trigger: %s)", w.id, ruleName, job.TriggerType)
	}

	// Update rule status to running
	w.updateRuleStatus(job.Rule, true, "RUNNING")

	// Set current executing rule for trigger chain tracking
	w.orchestrator.setCurrentExecutingRule(ruleName)
	defer w.orchestrator.clearCurrentExecutingRule()

	// Execute the job
	err := w.executeCommands(job)

	// Update final status
	if err != nil {
		w.updateRuleStatus(job.Rule, false, "FAILED")
		utils.LogDevloop("Worker %d: Rule %q execution failed: %v", w.id, ruleName, err)
	} else {
		w.updateRuleStatus(job.Rule, false, "SUCCESS")
		if verbose {
			utils.LogDevloop("Worker %d: Rule %q execution completed successfully", w.id, ruleName)
		}
	}
}

// executeCommands executes the commands for a rule job
func (w *Worker) executeCommands(job *RuleJob) error {
	rule := job.Rule
	verbose := w.orchestrator.isVerboseForRule(rule)

	if verbose {
		utils.LogDevloop("Worker %d: Executing commands for rule %q", w.id, rule.Name)
	}

	// Terminate any previously running commands for this worker
	if err := w.TerminateProcesses(); err != nil {
		utils.LogDevloop("Worker %d: Error terminating previous processes: %v", w.id, err)
	}

	// Get log writer for this rule
	logWriter, err := w.orchestrator.LogManager.GetWriter(rule.Name)
	if err != nil {
		return fmt.Errorf("error getting log writer: %w", err)
	}

	// Execute commands sequentially
	var currentCmds []*exec.Cmd
	var lastCmd *exec.Cmd

	for i, cmdStr := range rule.Commands {
		w.orchestrator.logDevloop("Worker %d: Running command: %s", w.id, cmdStr)
		cmd := createCrossPlatformCommand(cmdStr)

		// Setup output handling
		if err := w.setupCommandOutput(cmd, rule, logWriter); err != nil {
			return fmt.Errorf("failed to setup command output: %w", err)
		}

		// Set platform-specific process attributes
		setSysProcAttr(cmd)

		// Set working directory - default to config file directory if not specified
		workDir := rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(w.orchestrator.ConfigPath)
		}
		cmd.Dir = workDir

		// Set environment variables
		cmd.Env = os.Environ() // Inherit parent environment

		// Add environment variables to help subprocesses detect color support
		suppressColors := w.orchestrator.Config.Settings.SuppressSubprocessColors
		if w.orchestrator.ColorManager != nil && w.orchestrator.ColorManager.IsEnabled() && !suppressColors {
			cmd.Env = append(cmd.Env, "FORCE_COLOR=1")       // npm, chalk (Node.js)
			cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=1")    // many CLI tools
			cmd.Env = append(cmd.Env, "COLORTERM=truecolor") // general color support indicator
		}

		// Add rule-specific environment variables
		for key, value := range rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		if err := cmd.Start(); err != nil {
			utils.LogDevloop("Worker %d: Command %q failed to start for rule %q: %v", w.id, cmdStr, rule.Name, err)
			return fmt.Errorf("failed to start command: %w", err)
		}

		currentCmds = append(currentCmds, cmd)

		// For non-last commands, wait for completion before proceeding
		if i < len(rule.Commands)-1 {
			if err := cmd.Wait(); err != nil {
				utils.LogDevloop("Worker %d: Command failed for rule %q: %v", w.id, rule.Name, err)
				return fmt.Errorf("command failed: %w", err)
			}
		} else {
			// This is the last command - let it run and monitor it
			lastCmd = cmd
		}
	}

	// Update running commands
	w.commandsMutex.Lock()
	w.runningCommands = currentCmds
	w.commandsMutex.Unlock()

	// Monitor the last command if it exists
	if lastCmd != nil {
		err := lastCmd.Wait()
		if err != nil {
			utils.LogDevloop("Worker %d: Last command for rule %q failed: %v", w.id, rule.Name, err)
			return fmt.Errorf("last command failed: %w", err)
		}
	}

	// Signal log manager that rule finished
	w.orchestrator.LogManager.SignalFinished(rule.Name)

	return nil
}

// setupCommandOutput configures stdout/stderr for a command
func (w *Worker) setupCommandOutput(cmd *exec.Cmd, rule *pb.Rule, logWriter io.Writer) error {
	writers := []io.Writer{os.Stdout, logWriter}

	if w.orchestrator.Config.Settings.PrefixLogs {
		prefix := rule.Name
		if rule.Prefix != "" {
			prefix = rule.Prefix
		}

		// Apply prefix length constraints and left-align the text
		if w.orchestrator.Config.Settings.PrefixMaxLength > 0 {
			if uint32(len(prefix)) > w.orchestrator.Config.Settings.PrefixMaxLength {
				prefix = prefix[:w.orchestrator.Config.Settings.PrefixMaxLength]
			} else {
				// Left-align the prefix within the max length
				totalPadding := int(w.orchestrator.Config.Settings.PrefixMaxLength - uint32(len(prefix)))
				prefix = prefix + strings.Repeat(" ", totalPadding)
			}
		}

		// Use ColoredPrefixWriter for enhanced output with color support
		prefixStr := "[" + prefix + "] "
		coloredWriter := utils.NewColoredPrefixWriter(writers, prefixStr, w.orchestrator.ColorManager, rule)
		cmd.Stdout = coloredWriter
		cmd.Stderr = coloredWriter
	} else {
		// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
		if w.orchestrator.ColorManager != nil && w.orchestrator.ColorManager.IsEnabled() {
			coloredWriter := utils.NewColoredPrefixWriter(writers, "", w.orchestrator.ColorManager, rule)
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

// TerminateProcesses terminates all running processes for this worker
func (w *Worker) TerminateProcesses() error {
	w.commandsMutex.Lock()
	cmds := make([]*exec.Cmd, len(w.runningCommands))
	copy(cmds, w.runningCommands)
	w.runningCommands = []*exec.Cmd{} // Clear the slice
	w.commandsMutex.Unlock()

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
				utils.LogDevloop("Worker %d: Process %d already terminated", w.id, pid)
				return
			}

			// Try graceful termination first
			utils.LogDevloop("Worker %d: Terminating process group %d", w.id, pid)

			if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
				if !strings.Contains(err.Error(), "no such process") {
					utils.LogDevloop("Worker %d: Error sending SIGTERM to process group %d: %v", w.id, pid, err)
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
				utils.LogDevloop("Worker %d: Process group %d terminated gracefully", w.id, pid)
			case <-time.After(2 * time.Second):
				// Force kill
				utils.LogDevloop("Worker %d: Force killing process group %d", w.id, pid)
				syscall.Kill(-pid, syscall.SIGKILL)
				c.Process.Kill()
				<-done
			}

			// Verify termination
			if err := syscall.Kill(pid, 0); err == nil {
				utils.LogDevloop("Worker %d: WARNING: Process %d still exists after termination", w.id, pid)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}(cmd)
	}

	wg.Wait()
	return nil
}

// terminateCurrentJob terminates the current job if any
func (w *Worker) terminateCurrentJob() {
	w.currentMutex.RLock()
	currentJob := w.currentJob
	w.currentMutex.RUnlock()

	if currentJob != nil {
		utils.LogDevloop("Worker %d: Terminating current job for rule %q", w.id, currentJob.Rule.Name)
		w.TerminateProcesses()
		w.updateRuleStatus(currentJob.Rule, false, "TERMINATED")
	}
}

// updateRuleStatus updates the status for a rule via RuleRunner callback
func (w *Worker) updateRuleStatus(rule *pb.Rule, isRunning bool, buildStatus string) {
	// Use the cleaner callback approach
	if ruleRunner := w.orchestrator.GetRuleRunner(rule.Name); ruleRunner != nil {
		ruleRunner.UpdateStatus(isRunning, buildStatus)
	}
}

// WorkerPool manages a pool of workers and job distribution with deduplication
type WorkerPool struct {
	orchestrator *Orchestrator
	workers      []*Worker
	jobQueue     chan *RuleJob
	maxWorkers   int

	// Deduplication tracking
	executingRules map[string]*Worker  // ruleName -> worker executing it
	pendingRules   map[string]*RuleJob // ruleName -> latest pending job
	executionMutex sync.RWMutex

	// Pool lifecycle
	stopChan    chan struct{}
	stoppedChan chan struct{}
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(orchestrator *Orchestrator, maxWorkers int) *WorkerPool {
	// If maxWorkers is 0, set a reasonable default or unlimited
	if maxWorkers == 0 {
		maxWorkers = 100 // Reasonable default for "unlimited"
	}

	return &WorkerPool{
		orchestrator:   orchestrator,
		workers:        make([]*Worker, 0, maxWorkers),
		jobQueue:       make(chan *RuleJob, maxWorkers*2), // Buffer for queuing
		maxWorkers:     maxWorkers,
		executingRules: make(map[string]*Worker),
		pendingRules:   make(map[string]*RuleJob),
		stopChan:       make(chan struct{}),
		stoppedChan:    make(chan struct{}),
	}
}

// Start initializes and starts all workers
func (wp *WorkerPool) Start() error {
	utils.LogDevloop("Starting worker pool with %d workers", wp.maxWorkers)

	// Create and start workers
	for i := 0; i < wp.maxWorkers; i++ {
		worker := NewWorker(i+1, wp.orchestrator)
		wp.workers = append(wp.workers, worker)
		worker.Start(wp.jobQueue, wp)
	}

	utils.LogDevloop("Worker pool started successfully")
	return nil
}

// Stop gracefully stops all workers
func (wp *WorkerPool) Stop() error {
	utils.LogDevloop("Stopping worker pool...")
	close(wp.stopChan)

	// Close job queue to signal workers to stop
	close(wp.jobQueue)

	// Stop all workers
	var wg sync.WaitGroup
	for i, worker := range wp.workers {
		wg.Add(1)
		go func(idx int, w *Worker) {
			defer wg.Done()
			if err := w.Stop(); err != nil {
				utils.LogDevloop("Error stopping worker %d: %v", idx+1, err)
			}
		}(i, worker)
	}

	wg.Wait()
	close(wp.stoppedChan)
	utils.LogDevloop("Worker pool stopped")
	return nil
}

// EnqueueJob adds a job to the queue with deduplication logic
func (wp *WorkerPool) EnqueueJob(job *RuleJob) {
	wp.executionMutex.Lock()
	defer wp.executionMutex.Unlock()

	ruleName := job.Rule.Name

	// Check if rule is currently executing
	if executingWorker, isExecuting := wp.executingRules[ruleName]; isExecuting {
		// Rule is running - replace any existing pending job
		wp.pendingRules[ruleName] = job
		utils.LogDevloop("[%s] Rule executing on worker %d, job queued as pending (replacing previous)",
			ruleName, executingWorker.id)
		return
	}

	// Check if rule already has a pending job
	if _, hasPending := wp.pendingRules[ruleName]; hasPending {
		// Replace existing pending job with newer one
		wp.pendingRules[ruleName] = job
		utils.LogDevloop("[%s] Replacing pending job with newer one", ruleName)
		return
	}

	// Rule is free - queue normally
	select {
	case wp.jobQueue <- job:
		utils.LogDevloop("[%s] Job queued for execution", ruleName)
	default:
		// Queue is full - make it pending instead
		wp.pendingRules[ruleName] = job
		utils.LogDevloop("[%s] Queue full, job marked as pending", ruleName)
	}
}

// CompleteJob handles job completion and processes any pending jobs for the same rule
func (wp *WorkerPool) CompleteJob(job *RuleJob, worker *Worker) {
	wp.executionMutex.Lock()
	defer wp.executionMutex.Unlock()

	ruleName := job.Rule.Name

	// Remove from executing
	delete(wp.executingRules, ruleName)

	// Check for pending job
	if pendingJob, hasPending := wp.pendingRules[ruleName]; hasPending {
		delete(wp.pendingRules, ruleName)

		// Queue the pending job
		select {
		case wp.jobQueue <- pendingJob:
			utils.LogDevloop("[%s] Queued pending job after completion", ruleName)
		default:
			// Queue full - put it back as pending
			wp.pendingRules[ruleName] = pendingJob
			utils.LogDevloop("[%s] Queue full, keeping job as pending", ruleName)
		}
	}
}

// GetStatus returns the current status of the worker pool
func (wp *WorkerPool) GetStatus() (active int, capacity int, pending int, executing int) {
	wp.executionMutex.RLock()
	defer wp.executionMutex.RUnlock()

	return len(wp.jobQueue), cap(wp.jobQueue), len(wp.pendingRules), len(wp.executingRules)
}

// GetExecutingRules returns a copy of currently executing rules
func (wp *WorkerPool) GetExecutingRules() map[string]int {
	wp.executionMutex.RLock()
	defer wp.executionMutex.RUnlock()

	result := make(map[string]int)
	for ruleName, worker := range wp.executingRules {
		result[ruleName] = worker.id
	}
	return result
}

// GetPendingRules returns a list of rules with pending jobs
func (wp *WorkerPool) GetPendingRules() []string {
	wp.executionMutex.RLock()
	defer wp.executionMutex.RUnlock()

	result := make([]string, 0, len(wp.pendingRules))
	for ruleName := range wp.pendingRules {
		result = append(result, ruleName)
	}
	return result
}
