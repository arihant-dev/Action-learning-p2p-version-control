//go:build windows

package sync

import (
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {
	// No setpgid on Windows. Processes are spawned in a default group or controlled via Job Objects,
	// but direct cmd.Process.Kill() handles child process teardown robustly.
}

func killProcessGroup(cmd *exec.Cmd, graceful bool) error {
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
