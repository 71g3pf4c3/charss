//go:build unix

package render

import (
	"errors"
	"os/exec"
	"syscall"
)

// prepareCommand isolates chawan in its own process group (Setpgid) so that
// signals aimed at the child do not take down charss, and replaces the
// default context cancellation with a SIGKILL of the whole process group
// (chawan may spawn helper processes).
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 0 {
			return nil
		}
		// Negative pid addresses the process group created by Setpgid.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			// Group already gone — nothing to kill.
			return nil
		}
		return err
	}
}
