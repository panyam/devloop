//go:build darwin

package agent

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr sets the appropriate SysProcAttr for Darwin (macOS) systems
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Set process group ID
		// Note: Pdeathsig is not available on Darwin
	}
}
