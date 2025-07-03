//go:build darwin

package agent

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// cleanupOrphanedProcesses attempts to find and kill any processes that might have been orphaned
func (o *Orchestrator) cleanupOrphanedProcesses() {
	// Look for processes that might be ours based on common patterns
	patterns := []string{"auth-server", "user-server", "gateway", "main", "http-server"}

	for _, pattern := range patterns {
		// Use pgrep to find processes
		cmd := exec.Command("pgrep", "-f", pattern)
		output, err := cmd.Output()
		if err != nil {
			continue // No processes found
		}

		pids := strings.Fields(string(output))
		for _, pidStr := range pids {
			pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
			if err != nil {
				continue
			}

			// Check if this process is a child of our commands
			for _, cmds := range o.runningCommands {
				for _, cmd := range cmds {
					if cmd != nil && cmd.Process != nil && cmd.Process.Pid == pid {
						log.Printf("[devloop] Found orphaned process %d (%s), killing it", pid, pattern)
						exec.Command("kill", "-9", pidStr).Run()
						break
					}
				}
			}
		}
	}
}
