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
)

// Execute runs the commands for this rule
func (r *ruleRunner) Execute() error {
	r.updateStatus(true, "RUNNING")

	if r.isVerbose() {
		log.Printf("[devloop] Executing commands for rule %q", r.rule.Name)
	}

	// Terminate any previously running commands
	if err := r.TerminateProcesses(); err != nil {
		log.Printf("[devloop] Error terminating previous processes for rule %q: %v", r.rule.Name, err)
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
		log.Printf("[devloop]   Running command: %s", cmdStr)
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
		for key, value := range r.rule.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		if err := cmd.Start(); err != nil {
			log.Printf("[devloop] Command %q failed to start for rule %q: %v", cmdStr, r.rule.Name, err)
			r.updateStatus(false, "FAILED")
			return fmt.Errorf("failed to start command: %w", err)
		}

		currentCmds = append(currentCmds, cmd)

		// For non-last commands, wait for completion before proceeding
		if i < len(r.rule.Commands)-1 {
			if err := cmd.Wait(); err != nil {
				log.Printf("[devloop] Command failed for rule %q: %v", r.rule.Name, err)
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
		log.Printf("[devloop] Rule %q commands finished.", r.rule.Name)
	}

	return nil
}

// setupCommandOutput configures stdout/stderr for a command
func (r *ruleRunner) setupCommandOutput(cmd *exec.Cmd, logWriter io.Writer) error {
	writers := []io.Writer{os.Stdout, logWriter}

	if r.orchestrator.Config.Settings.PrefixLogs {
		prefix := r.rule.Name
		if r.rule.Prefix != "" {
			prefix = r.rule.Prefix
		}

		// Apply prefix length constraints and center the text
		if r.orchestrator.Config.Settings.PrefixMaxLength > 0 {
			if len(prefix) > r.orchestrator.Config.Settings.PrefixMaxLength {
				prefix = prefix[:r.orchestrator.Config.Settings.PrefixMaxLength]
			} else {
				// Center the prefix within the max length
				totalPadding := r.orchestrator.Config.Settings.PrefixMaxLength - len(prefix)
				leftPadding := totalPadding / 2
				rightPadding := totalPadding - leftPadding

				prefix = strings.Repeat(" ", leftPadding) + prefix + strings.Repeat(" ", rightPadding)
			}
		}

		// Use ColoredPrefixWriter for enhanced output with color support
		prefixStr := "[" + prefix + "] "
		coloredWriter := NewColoredPrefixWriter(writers, prefixStr, r.orchestrator.ColorManager, &r.rule)
		cmd.Stdout = coloredWriter
		cmd.Stderr = coloredWriter
	} else {
		// For non-prefixed output, still use ColoredPrefixWriter but with empty prefix
		// This ensures consistent color handling even without prefixes
		if r.orchestrator.ColorManager != nil && r.orchestrator.ColorManager.IsEnabled() {
			coloredWriter := NewColoredPrefixWriter(writers, "", r.orchestrator.ColorManager, &r.rule)
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
func (r *ruleRunner) monitorLastCommand(cmd *exec.Cmd) {
	err := cmd.Wait()

	if err != nil {
		log.Printf("[devloop] Command for rule %q failed: %v", r.rule.Name, err)
		r.updateStatus(false, "FAILED")
	} else {
		r.updateStatus(false, "SUCCESS")
	}

	r.orchestrator.LogManager.SignalFinished(r.rule.Name)
	log.Printf("[devloop] Rule %q commands finished.", r.rule.Name)
}

// TerminateProcesses terminates all running processes for this rule
func (r *ruleRunner) TerminateProcesses() error {
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
					log.Printf("[devloop] Process %d for rule %q already terminated", pid, r.rule.Name)
				}
				return
			}

			// Try graceful termination first
			if r.isVerbose() {
				log.Printf("[devloop] Terminating process group %d for rule %q", pid, r.rule.Name)
			}

			if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
				if !strings.Contains(err.Error(), "no such process") {
					log.Printf("Error sending SIGTERM to process group %d for rule %q: %v",
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
					log.Printf("[devloop] Process group %d for rule %q terminated gracefully",
						pid, r.rule.Name)
				}
			case <-time.After(2 * time.Second):
				// Force kill
				log.Printf("[devloop] Force killing process group %d for rule %q", pid, r.rule.Name)
				syscall.Kill(-pid, syscall.SIGKILL)
				c.Process.Kill()
				<-done
			}

			// Verify termination
			if err := syscall.Kill(pid, 0); err == nil {
				log.Printf("WARNING: Process %d for rule %q still exists after termination",
					pid, r.rule.Name)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}(cmd)
	}

	wg.Wait()
	return nil
}
