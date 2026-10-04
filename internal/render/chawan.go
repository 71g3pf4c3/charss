// Package render drives the external chawan browser (https://chawan.net) to
// display article HTML.
//
// charss never renders HTML itself: articles are shown by running chawan as
// a child process that inherits the terminal — the same model newsboat uses
// when it spawns an external browser/pager. This package owns only the
// process plumbing: resolving the binary, temp files for inline HTML, argv
// construction, and clean teardown (including killing the whole process
// group on context cancellation).
package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// DefaultBinary is the binary looked up on $PATH when Options.Binary is
// empty. The chawan distribution installs its executable as `cha`.
const DefaultBinary = "cha"

// Options configures the chawan driver.
type Options struct {
	// Binary is the path to the chawan executable. Empty means look up
	// "cha" on $PATH; no other default guessing is done.
	Binary string

	// Args are extra arguments passed to chawan before the target
	// (temp file path or URL).
	Args []string
}

// Chawan drives the external chawan browser process. It holds no runtime
// state and is safe for concurrent use.
type Chawan struct {
	opts Options
}

// New returns a driver with the given options.
func New(opts Options) *Chawan {
	return &Chawan{opts: opts}
}

// LookPath resolves the browser binary and verifies it is executable,
// returning the resolved path. If the binary is missing it returns a
// descriptive error that includes the install hint for chawan.
func (c *Chawan) LookPath() (string, error) {
	name := c.opts.Binary
	if name == "" {
		name = DefaultBinary
	}

	path, err := exec.LookPath(name)
	if errors.Is(err, exec.ErrDot) {
		// exec.LookPath refuses to return relative paths as-is, but a
		// relative path that exists and is executable is a usable browser.
		err = nil
	}
	if err != nil {
		return "", fmt.Errorf("browser %q not found — install chawan (https://chawan.net, binary `cha`) or set `browser` in the config: %w", name, err)
	}
	return path, nil
}

// ShowHTML displays html in chawan, writing it to a temp file first. The
// temp file and its directory are always removed when chawan exits,
// regardless of the outcome.
func (c *Chawan) ShowHTML(ctx context.Context, html string) error {
	dir, err := os.MkdirTemp("", "charss-*")
	if err != nil {
		return fmt.Errorf("render: creating temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "article.html")
	if err := os.WriteFile(file, []byte(html), 0o600); err != nil {
		return fmt.Errorf("render: writing temp file: %w", err)
	}

	return c.show(ctx, file)
}

// ShowURL opens url directly in chawan (streaming case — no temp file).
func (c *Chawan) ShowURL(ctx context.Context, url string) error {
	return c.show(ctx, url)
}

// show runs chawan on target, inheriting stdio, and always calls Wait.
func (c *Chawan) show(ctx context.Context, target string) error {
	bin, err := c.LookPath()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin, slices.Concat(c.opts.Args, []string{target})...)
	// chawan is interactive and takes over the screen: it must draw in
	// our terminal.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	prepareCommand(cmd)

	if err := cmd.Start(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("render: starting chawan: %w", err)
	}

	// chawan reads stdin interactively, but Setpgid put it in a
	// background process group — hand the terminal's foreground group
	// over for the duration of the run (see runForeground).
	waitErr := runForeground(os.Stdin.Fd(), cmd.Process.Pid, cmd.Wait)
	if waitErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		// We killed the process group; report the reason, not the
		// resulting "signal: killed" ExitError.
		return ctxErr
	}
	var execErr *exec.ExitError
	if errors.As(waitErr, &execErr) {
		return &ExitError{Code: execErr.ExitCode(), Err: waitErr}
	}
	return fmt.Errorf("render: waiting for chawan: %w", waitErr)
}

// ExitError is returned when chawan exits with a non-zero status.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("chawan exited with status %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("chawan exited with status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }
