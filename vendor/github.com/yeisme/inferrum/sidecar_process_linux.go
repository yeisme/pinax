//go:build linux

package inferrum

import (
	"os/exec"
	"syscall"
	"time"
)

func configureSidecarProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = time.Second
}

// cleanupSidecarProcess also handles a sidecar parent that exits while leaving
// background children in its process group.
func cleanupSidecarProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
