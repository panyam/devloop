//go:build linux

package agent

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr sets the appropriate SysProcAttr for Linux systems
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,            // Set process group ID
		Pdeathsig: syscall.SIGTERM, // Kill child when parent dies
	}
}
