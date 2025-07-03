//go:build windows

package agent

// cleanupOrphanedProcesses attempts to clean up orphaned processes on Windows
func (o *Orchestrator) cleanupOrphanedProcesses() {
	// Windows-specific cleanup if needed
}