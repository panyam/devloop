//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr sets the appropriate SysProcAttr for Windows systems
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Windows doesn't support Pdeathsig
		// We can still create a new process group
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
