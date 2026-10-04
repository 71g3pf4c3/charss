//go:build !unix

package render

import "os/exec"

// prepareCommand is a no-op on platforms without process groups. Non-unix
// builds are excluded from releases (see .goreleaser.yaml); on them the
// default CommandContext cancellation (killing just the process) applies.
func prepareCommand(cmd *exec.Cmd) {}

// runForeground has no foreground-group concept to hand over on non-unix
// platforms; wait runs unchanged.
func runForeground(fd uintptr, pgid int, wait func() error) error {
	return wait()
}
