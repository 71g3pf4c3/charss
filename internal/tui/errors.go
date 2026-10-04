package tui

import "errors"

// errTerminalNotAttached is returned when the terminal adapter was never
// wired to a running program. Running chawan while Bubble Tea still owns
// the terminal would garble both, so we refuse instead.
var errTerminalNotAttached = errors.New("tui: terminal not attached to a program")

// errNoBrowser is surfaced as a status line when no browser is configured.
var errNoBrowser = errors.New("no browser configured (set `browser` in config)")
