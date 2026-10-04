//go:build unix

package render

import (
	"errors"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"
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

// runForeground runs wait with the terminal's foreground process group
// handed to pgid, and hands the group back afterwards.
//
// prepareCommand puts the child in its own process group, which also makes
// it a *background* group on the controlling terminal: its first read from
// stdin would raise SIGTTIN and stop it (state T) — the reader appears to
// open an article and freeze forever. Handing the foreground group over for
// the lifetime of the child fixes that; SIGTTOU/SIGTTIN are ignored in the
// parent for the duration, because each TIOCSPGRP makes the caller itself
// briefly a background group.
//
// Best-effort by design: without a controlling terminal (pipes, unit
// tests, CI) the handover is skipped and run executes unchanged.
func runForeground(fd uintptr, pgid int, wait func() error) error {
	old, err := foregroundPgid(fd)
	if err != nil || old == pgid {
		// ENOTTY and friends: nothing to hand over.
		return wait()
	}

	signal.Ignore(syscall.SIGTTOU, syscall.SIGTTIN)
	defer signal.Reset(syscall.SIGTTOU, syscall.SIGTTIN)

	if err := setForegroundPgid(fd, pgid); err != nil {
		// Could not hand over (unusual on a real tty); run anyway —
		// the pre-fix behavior is no worse.
		return wait()
	}

	waitErr := wait()

	// The child is gone; take the foreground group back before the
	// terminal is restored to the parent's. Errors are ignored: the
	// caller re-initializes the terminal regardless.
	_ = setForegroundPgid(fd, old)
	return waitErr
}

// foregroundPgid returns the terminal's current foreground process group.
func foregroundPgid(fd uintptr) (int, error) {
	var p int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&p)))
	if errno != 0 {
		return 0, errno
	}
	return int(p), nil
}

// setForegroundPgid sets the terminal's foreground process group.
func setForegroundPgid(fd uintptr, pgid int) error {
	p := int32(pgid)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&p)))
	if errno != 0 {
		return errno
	}
	return nil
}
