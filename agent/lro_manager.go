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

	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/panyam/devloop/utils"
)

// LROProcess represents a running long-running operation
type LROProcess struct {
	processInfo *ProcessInfo
	cancel      context.CancelFunc
	ctx         context.Context
}

// LROManager manages long-running operation processes
type LROManager struct {
	orchestrator   *Orchestrator
	processes      map[string]*LROProcess // ruleName -> running process
	mutex          sync.RWMutex
	processManager *ProcessManager
}

// NewLROManager creates a new LRO manager
func NewLROManager(orchestrator *Orchestrator) *LROManager {
	return &LROManager{
		orchestrator:   orchestrator,
		processes:      make(map[string]*LROProcess),
		processManager: NewProcessManager(orchestrator.Verbose),
	}
}

// RestartProcess kills any existing process for the rule and starts a new one
func (lro *LROManager) RestartProcess(rule *pb.Rule, triggerType string) error {
	ruleName := rule.Name
	verbose := lro.orchestrator.isVerboseForRule(rule)

	if verbose {
		utils.LogDevloop("LRO: Restarting process for rule %q (trigger: %s)", ruleName, triggerType)
	}

	// Prevent concurrent restarts for the same rule
	lro.mutex.Lock()
	existingProcess := lro.processes[ruleName]
	if existingProcess != nil {
		// Mark this process for termination but don't wait here to avoid blocking
		delete(lro.processes, ruleName)
	}
	lro.mutex.Unlock()

	// 1. Kill existing process if any (non-blocking)
	if existingProcess != nil {
		go func() {
			if existingProcess.cancel != nil {
				existingProcess.cancel()
			}
			lro.processManager.TerminateProcess(existingProcess.processInfo)
		}()
	}

	// 2. Wait for process cleanup (ports will be released when process tree dies)
	time.Sleep(200 * time.Millisecond)

	// 3. Start new process
	return lro.startNewProcess(rule, triggerType)
}

// killExistingProcess terminates the existing process for a rule
func (lro *LROManager) killExistingProcess(ruleName string) error {
	lro.mutex.Lock()
	lroProcess := lro.processes[ruleName]
	if lroProcess == nil {
		lro.mutex.Unlock()
		return nil // No existing process
	}

	// Remove from map immediately to prevent race conditions
	delete(lro.processes, ruleName)
	lro.mutex.Unlock()

	// Cancel context first
	if lroProcess.cancel != nil {
		lroProcess.cancel()
	}

	// Use improved ProcessManager for reliable termination
	return lro.processManager.TerminateProcess(lroProcess.processInfo)
}

// startNewProcess starts a new LRO process for the given rule
func (lro *LROManager) startNewProcess(rule *pb.Rule, triggerType string) error {
	ruleName := rule.Name
	verbose := lro.orchestrator.isVerboseForRule(rule)

	if verbose {
		utils.LogDevloop("LRO: Starting new process for rule %q", ruleName)
	}

	// Update rule status to running at start
	lro.updateLROStatus(rule, true, "RUNNING")

	// Create context for the process
	ctx, cancel := context.WithCancel(context.Background())

	// Get log writer for this rule
	logWriter, err := lro.orchestrator.LogManager.GetWriter(ruleName)
	if err != nil {
		cancel()
		lro.updateLROStatus(rule, false, "FAILED")
		return fmt.Errorf("error getting log writer: %w", err)
	}

	// Execute commands sequentially, but the last one runs as LRO
	var lroCmd *exec.Cmd

	for i, cmdStr := range rule.Commands {
		if verbose {
			utils.LogDevloop("LRO: Running command %d/%d for rule %q: %s", i+1, len(rule.Commands), ruleName, cmdStr)
		}

		cmd := createCrossPlatformCommand(cmdStr)

		// Setup output handling
		if err := lro.setupCommandOutput(cmd, rule, logWriter); err != nil {
			cancel()
			lro.updateLROStatus(rule, false, "FAILED")
			return fmt.Errorf("failed to setup command output: %w", err)
		}

		// Set platform-specific process attributes
		setSysProcAttr(cmd)

		// Set working directory
		workDir := rule.WorkDir
		if workDir == "" {
			workDir = filepath.Dir(lro.orchestrator.ConfigPath)
		}
		cmd.Dir = workDir

		// Set environment variables
		cmd.Env = os.Environ()

		// Add environment variables to help subprocesses detect color support
		suppressColors := lro.orchestrator.Config.Settings.SuppressSubprocessColors
		if lro.orchestrator.ColorManager != nil && lro.orchestrator.ColorManager.IsEnabled() && !suppressColors {
			cmd.Env = append(cmd.Env, "FORCE_COLOR=1")
			cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=1")
			cmd.Env = append(cmd.Env, "COLORTERM=truecolor")
		}

		// Add rule-specific environment variables
		for key, value := range rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			cancel()
			lro.updateLROStatus(rule, false, "FAILED")
			return fmt.Errorf("failed to start command %q: %w", cmdStr, err)
		}

		// For non-last commands, wait for completion
		if i < len(rule.Commands)-1 {
			if err := cmd.Wait(); err != nil {
				cancel()
				lro.updateLROStatus(rule, false, "FAILED")
				return fmt.Errorf("command %q failed: %w", cmdStr, err)
			}
		} else {
			// This is the last command - it becomes the LRO process
			lroCmd = cmd
		}
	}

	if lroCmd == nil {
		cancel()
		lro.updateLROStatus(rule, false, "FAILED")
		return fmt.Errorf("no commands to run for rule %q", ruleName)
	}

	// Store the LRO process
	processInfo := lro.processManager.GetProcessInfo(lroCmd, ruleName)
	lroProcess := &LROProcess{
		processInfo: processInfo,
		cancel:      cancel,
		ctx:         ctx,
	}

	// Double-check no process was created while we were starting (race condition protection)
	lro.mutex.Lock()
	if _, exists := lro.processes[ruleName]; exists {
		lro.mutex.Unlock()
		// A race occurred - kill our new process and let the existing one run
		if verbose {
			utils.LogDevloop("LRO: Race detected - killing new process %d, keeping existing for rule %q",
				lroProcess.processInfo.pid, ruleName)
		}
		lro.processManager.TerminateProcess(lroProcess.processInfo)
		cancel()
		return nil
	}
	lro.processes[ruleName] = lroProcess
	lro.mutex.Unlock()

	if verbose {
		utils.LogDevloop("LRO: Started process %d for rule %q", lroProcess.processInfo.pid, ruleName)
	}

	// Monitor the process in the background
	go lro.monitorProcess(lroProcess, rule)

	return nil
}

// monitorProcess monitors an LRO process for unexpected termination
func (lro *LROManager) monitorProcess(process *LROProcess, rule *pb.Rule) {
	defer func() {
		// Clean up when process exits
		lro.mutex.Lock()
		delete(lro.processes, rule.Name)
		lro.mutex.Unlock()

		if process.cancel != nil {
			process.cancel()
		}
	}()

	// Wait for the process to exit
	err := process.processInfo.cmd.Wait()

	verbose := lro.orchestrator.isVerboseForRule(rule)
	pid := process.processInfo.pid
	ruleName := process.processInfo.ruleName

	if err != nil {
		utils.LogDevloop("LRO: Process %d for rule %q exited with error: %v", pid, ruleName, err)
		lro.updateLROStatus(rule, false, "FAILED")
	} else {
		if verbose {
			utils.LogDevloop("LRO: Process %d for rule %q exited normally", pid, ruleName)
		}
		lro.updateLROStatus(rule, false, "COMPLETED")
	}

	// Signal log manager that rule finished
	lro.orchestrator.LogManager.SignalFinished(rule.Name)
}

// setupCommandOutput configures stdout/stderr for a command (similar to workers.go)
func (lro *LROManager) setupCommandOutput(cmd *exec.Cmd, rule *pb.Rule, logWriter io.Writer) error {
	writers := []io.Writer{os.Stdout, logWriter}

	if lro.orchestrator.Config.Settings.PrefixLogs {
		prefix := rule.Name
		if rule.Prefix != "" {
			prefix = rule.Prefix
		}

		// Apply prefix length constraints and left-align the text
		if lro.orchestrator.Config.Settings.PrefixMaxLength > 0 {
			if uint32(len(prefix)) > lro.orchestrator.Config.Settings.PrefixMaxLength {
				prefix = prefix[:lro.orchestrator.Config.Settings.PrefixMaxLength]
			} else {
				// Left-align the prefix within the max length
				totalPadding := int(lro.orchestrator.Config.Settings.PrefixMaxLength - uint32(len(prefix)))
				prefix = prefix + strings.Repeat(" ", totalPadding)
			}
		}

		// Use ColoredPrefixWriter for enhanced output with color support
		prefixStr := "[" + prefix + "] "
		coloredWriter := utils.NewColoredPrefixWriter(writers, prefixStr, lro.orchestrator.ColorManager, rule)
		cmd.Stdout = coloredWriter
		cmd.Stderr = coloredWriter
	} else {
		// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
		if lro.orchestrator.ColorManager != nil && lro.orchestrator.ColorManager.IsEnabled() {
			coloredWriter := utils.NewColoredPrefixWriter(writers, "", lro.orchestrator.ColorManager, rule)
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

// updateLROStatus updates the status for a rule via RuleRunner callback
func (lro *LROManager) updateLROStatus(rule *pb.Rule, isRunning bool, buildStatus string) {
	// Use the cleaner callback approach
	if ruleRunner := lro.orchestrator.GetRuleRunner(rule.Name); ruleRunner != nil {
		ruleRunner.UpdateStatus(isRunning, buildStatus)
	}
}

// Stop gracefully stops all LRO processes
func (lro *LROManager) Stop() error {
	lro.mutex.RLock()
	ruleNames := make([]string, 0, len(lro.processes))
	for ruleName := range lro.processes {
		ruleNames = append(ruleNames, ruleName)
	}
	lro.mutex.RUnlock()

	// Kill all processes
	for _, ruleName := range ruleNames {
		if err := lro.killExistingProcess(ruleName); err != nil {
			utils.LogDevloop("LRO: Error stopping process for rule %q: %v", ruleName, err)
		}
	}

	return nil
}

// GetRunningProcesses returns a map of currently running LRO processes
func (lro *LROManager) GetRunningProcesses() map[string]int {
	lro.mutex.RLock()
	defer lro.mutex.RUnlock()

	result := make(map[string]int)
	for ruleName, process := range lro.processes {
		if process.processInfo != nil {
			result[ruleName] = process.processInfo.pid
		}
	}
	return result
}

// IsProcessRunning checks if a process is running for the given rule
func (lro *LROManager) IsProcessRunning(ruleName string) bool {
	lro.mutex.RLock()
	defer lro.mutex.RUnlock()

	_, exists := lro.processes[ruleName]
	return exists
}
