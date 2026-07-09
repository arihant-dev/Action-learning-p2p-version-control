//go:build !windows

package sync

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd, graceful bool) error {
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	sig := syscall.SIGKILL
	if graceful {
		sig = syscall.SIGTERM
	}
	if err == nil && pgid > 0 {
		return syscall.Kill(-pgid, sig)
	}
	return cmd.Process.Signal(sig)
}
