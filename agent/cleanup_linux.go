//go:build linux

package agent

// cleanupOrphanedProcesses is a no-op on Linux because Pdeathsig handles it
func (o *Orchestrator) cleanupOrphanedProcesses() {
	// On Linux, Pdeathsig should handle process cleanup when parent dies
}