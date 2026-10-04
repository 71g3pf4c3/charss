// Package image fetches images and converts them to terminal-ready output
// via the external chafa binary (https://hpjansson.org/chafa/).
//
// The TUI never decodes pixels itself: [Renderer.Convert] shells out to
// chafa with the image bytes on stdin and returns the raw terminal byte
// stream (sixel escape sequences by default) for printing.
//
// Sixel is the primary and default format; kitty graphics protocol and
// symbol-based (halfblock/unicode) output exist only as fallbacks.
package image

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
)

// Format is the terminal graphics protocol chafa is asked to emit.
type Format int

const (
	// FormatSixel is the primary output format. Full sixel support is a
	// hard product requirement; everything else is a fallback.
	FormatSixel Format = iota
	// FormatKitty is the kitty graphics protocol. Fallback only.
	FormatKitty
	// FormatSymbols is halfblock/unicode art. Fallback only; works
	// universally on any terminal.
	FormatSymbols
)

// String returns the chafa --format name for the format.
func (f Format) String() string {
	switch f {
	case FormatSixel:
		return "sixel"
	case FormatKitty:
		return "kitty"
	case FormatSymbols:
		return "symbols"
	default:
		return "symbols"
	}
}

// ErrNoChafa is returned when the chafa binary cannot be found, either in
// PATH or at Options.Binary.
var ErrNoChafa = errors.New("chafa not found: install it (https://hpjansson.org/chafa/, e.g. apt install chafa) or set Options.Binary")

// Options configures a Renderer.
type Options struct {
	// Format is the chafa output format. The zero value is FormatSixel,
	// which is the intended default.
	Format Format

	// Columns is the target image width in terminal cells. 0 lets chafa
	// decide (it sizes after the terminal, or 80x80 by default).
	Columns int

	// MaxColors caps the palette size. 0 uses the chafa default for the
	// chosen format.
	MaxColors int

	// Binary is the path to the chafa executable. Empty means
	// exec.LookPath("chafa").
	Binary string
}

// Renderer converts image bytes to terminal output by shelling out to
// chafa. A Renderer is safe for concurrent use.
type Renderer struct {
	opts Options
}

// New returns a Renderer with the given options.
func New(opts Options) *Renderer {
	return &Renderer{opts: opts}
}

// Convert runs chafa on image data and returns the raw terminal output
// (sixel byte stream / kitty escapes / unicode art) for the TUI to print.
//
// chafa is invoked as:
//
//	chafa --format <fmt> [--size <cols>] [--colors N] -
//
// with the image bytes on stdin and stdout/stderr captured (stdout is the
// returned payload, stderr feeds error messages). Convert is idempotent
// and safe to retry; it mutates nothing but the spawned process.
func (r *Renderer) Convert(ctx context.Context, data []byte) ([]byte, error) {
	bin := r.opts.Binary
	if bin == "" {
		p, err := exec.LookPath("chafa")
		if err != nil {
			return nil, fmt.Errorf("%w (%v)", ErrNoChafa, err)
		}
		bin = p
	}

	cmd := exec.CommandContext(ctx, bin, r.args()...)
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNoChafa, bin)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				return nil, fmt.Errorf("chafa exited with status %d", ee.ExitCode())
			}
			return nil, fmt.Errorf("chafa exited with status %d: %s", ee.ExitCode(), msg)
		}
		return nil, fmt.Errorf("running chafa: %w", err)
	}
	return stdout.Bytes(), nil
}

// args assembles the chafa command line; order is fixed so tests (and
// humans) can rely on it.
func (r *Renderer) args() []string {
	args := []string{"--format", r.opts.Format.String()}
	if r.opts.Columns > 0 {
		args = append(args, "--size", strconv.Itoa(r.opts.Columns))
	}
	if r.opts.MaxColors > 0 {
		args = append(args, "--colors", strconv.Itoa(r.opts.MaxColors))
	}
	return append(args, "-")
}
