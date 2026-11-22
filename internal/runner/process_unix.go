//go:build darwin || linux

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) func() {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	// CommandContext stops watching once the immediate child exits. Always
	// terminate our owned group on return, including ordinary surviving children
	// whose parent exited successfully or whose output pipes hit WaitDelay.
	return func() { _ = cmd.Cancel() }
}
