package agent

import (
	"fmt"

	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/panyam/devloop/utils"
)

// ProcessInfo contains information about a running process
type ProcessInfo struct {
	cmd       *exec.Cmd
	pid       int
	pgid      int // Process group ID
	ruleName  string
	startTime time.Time
}

// ProcessManager handles reliable process termination
type ProcessManager struct {
	verbose bool
}

// NewProcessManager creates a new process manager
func NewProcessManager(verbose bool) *ProcessManager {
	return &ProcessManager{verbose: verbose}
}

// TerminateProcess reliably terminates a process and all its children
func (pm *ProcessManager) TerminateProcess(processInfo *ProcessInfo) error {
	if processInfo == nil || processInfo.cmd == nil {
		return nil
	}

	pid := processInfo.pid
	pgid := processInfo.pgid
	ruleName := processInfo.ruleName

	if pm.verbose {
		utils.LogDevloop("ProcessManager: Terminating process %d (group %d) for rule %q", pid, pgid, ruleName)
	}

	// Step 1: Check if process is still alive
	if !pm.isProcessAlive(pid) {
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Process %d already dead", pid)
		}
		return nil
	}

	// Step 2: Send SIGTERM to process group for graceful shutdown
	if err := pm.sendSignalToGroup(pgid, syscall.SIGTERM); err != nil {
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Error sending SIGTERM to group %d: %v", pgid, err)
		}
	}

	// Step 3: Wait for graceful shutdown with timeout
	gracefulDone := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Ignore panics from Wait() on already-reaped processes
				if pm.verbose {
					utils.LogDevloop("ProcessManager: Wait() panicked (process already reaped): %v", r)
				}
			}
			gracefulDone <- true
		}()

		if processInfo.cmd != nil && processInfo.cmd.Process != nil {
			processInfo.cmd.Wait()
		}
	}()

	// Step 4: Handle graceful vs forceful termination
	select {
	case <-gracefulDone:
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Process %d terminated gracefully", pid)
		}
		return pm.verifyTermination(pid, pgid, ruleName)

	case <-time.After(5 * time.Second):
		// Force kill
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Force killing process group %d for rule %q", pgid, ruleName)
		}
		return pm.forceKillProcess(processInfo, gracefulDone)
	}
}

// forceKillProcess performs forceful termination
func (pm *ProcessManager) forceKillProcess(processInfo *ProcessInfo, gracefulDone chan bool) error {
	pid := processInfo.pid
	pgid := processInfo.pgid
	ruleName := processInfo.ruleName

	// Send SIGKILL to entire process group
	if err := pm.sendSignalToGroup(pgid, syscall.SIGKILL); err != nil {
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Error sending SIGKILL to group %d: %v", pgid, err)
		}
	}

	// Also kill the main process directly as backup
	if processInfo.cmd != nil && processInfo.cmd.Process != nil {
		processInfo.cmd.Process.Kill()
	}

	// Wait for the graceful goroutine to complete (should be quick after SIGKILL)
	select {
	case <-gracefulDone:
		// Good
	case <-time.After(2 * time.Second):
		if pm.verbose {
			utils.LogDevloop("ProcessManager: WARNING: Wait() goroutine didn't complete after SIGKILL")
		}
	}

	return pm.verifyTermination(pid, pgid, ruleName)
}

// sendSignalToGroup sends a signal to a process group
func (pm *ProcessManager) sendSignalToGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return fmt.Errorf("invalid process group ID: %d", pgid)
	}

	if pm.verbose {
		sigName := "UNKNOWN"
		switch sig {
		case syscall.SIGTERM:
			sigName = "SIGTERM"
		case syscall.SIGKILL:
			sigName = "SIGKILL"
		}
		utils.LogDevloop("ProcessManager: Sending %s to process group %d", sigName, pgid)
	}

	// Send signal to process group (negative PID)
	err := syscall.Kill(-pgid, sig)
	if err != nil && !strings.Contains(err.Error(), "no such process") {
		return fmt.Errorf("failed to send signal to process group %d: %w", pgid, err)
	}

	return nil
}

// isProcessAlive checks if a process is still running
func (pm *ProcessManager) isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Use kill(pid, 0) to check process existence
	err := syscall.Kill(pid, 0)
	return err == nil
}

// verifyTermination ensures the process and its children are actually dead
func (pm *ProcessManager) verifyTermination(pid, pgid int, ruleName string) error {
	// Check main process
	if pm.isProcessAlive(pid) {
		utils.LogDevloop("ProcessManager: WARNING: Main process %d still alive for rule %q", pid, ruleName)

		// Final SIGKILL attempt to process group AND individual process
		if err := pm.sendSignalToGroup(pgid, syscall.SIGKILL); err != nil {
			if pm.verbose {
				utils.LogDevloop("ProcessManager: Error in final group kill: %v", err)
			}
		}

		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			if !strings.Contains(err.Error(), "no such process") {
				utils.LogDevloop("ProcessManager: Error in final individual kill: %v", err)
			}
		}

		// Wait a bit and check again
		time.Sleep(200 * time.Millisecond)
		if pm.isProcessAlive(pid) {
			return fmt.Errorf("process %d for rule %q refuses to die", pid, ruleName)
		}
	}

	// Verify process group is clean
	if err := pm.verifyProcessGroupClean(pgid, ruleName); err != nil {
		utils.LogDevloop("ProcessManager: Warning - process group cleanup issue: %v", err)
		// Don't return error - just warn
	}

	if pm.verbose {
		utils.LogDevloop("ProcessManager: Successfully terminated process %d (group %d) for rule %q", pid, pgid, ruleName)
	}

	return nil
}

// verifyProcessGroupClean checks if any processes remain in the process group
func (pm *ProcessManager) verifyProcessGroupClean(pgid int, ruleName string) error {
	// On Unix systems, we can check if the process group still exists
	// If kill(-pgid, 0) succeeds, there are still processes in the group
	err := syscall.Kill(-pgid, 0)
	if err == nil {
		// Process group still has processes
		if pm.verbose {
			utils.LogDevloop("ProcessManager: WARNING: Process group %d still has processes for rule %q", pgid, ruleName)
		}

		// Final cleanup attempt
		syscall.Kill(-pgid, syscall.SIGKILL)
		time.Sleep(100 * time.Millisecond)

		// Check again
		if syscall.Kill(-pgid, 0) == nil {
			return fmt.Errorf("process group %d still has processes after cleanup", pgid)
		}
	}

	return nil
}

// GetProcessInfo extracts process information from an exec.Cmd
func (pm *ProcessManager) GetProcessInfo(cmd *exec.Cmd, ruleName string) *ProcessInfo {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid

	// Get actual process group ID (setSysProcAttr should have set Setpgid: true)
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// Fallback to PID if we can't get PGID
		if pm.verbose {
			utils.LogDevloop("ProcessManager: Warning - could not get PGID for %d, using PID: %v", pid, err)
		}
		pgid = pid
	}

	if pm.verbose {
		utils.LogDevloop("ProcessManager: Process %d has PGID %d for rule %q", pid, pgid, ruleName)
	}

	return &ProcessInfo{
		cmd:       cmd,
		pid:       pid,
		pgid:      pgid,
		ruleName:  ruleName,
		startTime: time.Now(),
	}
}
