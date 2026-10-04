package image

import (
	"os"
	"strings"
)

// Detect inspects the environment to pick the best output format.
//
// Order of truth (highest first):
//
//  1. force, when non-nil: true → FormatSixel, false → FormatSymbols.
//  2. $TERM ends in "sixel" (e.g. xterm-sixel) or names a terminal known
//     to support sixel: foot*, xterm*, st*, mintty, rio, wezterm, contour.
//  3. $COLORTERM is deliberately ignored — truecolor says nothing about
//     sixel support.
//
// It returns FormatSixel when sixel is believed available and
// FormatSymbols otherwise (safe on any terminal). FormatKitty is never
// auto-selected: kitty support cannot be inferred from TERM reliably
// (kitty's TERM is xterm-kitty), so it must be requested explicitly.
//
// Future work: query the terminal directly with DA1 (Primary Device
// Attributes, ESC [ c) and check for the sixel feature byte instead of
// relying on TERM heuristics. That requires raw-mode termios handling and
// is out of scope here.
func Detect(force *bool) Format {
	if force != nil {
		if *force {
			return FormatSixel
		}
		return FormatSymbols
	}
	return detectTERM(os.Getenv("TERM"))
}

// knownSixelTERMs lists TERM values (matched as exact name, or as a
// prefix followed by "-") of terminals with sixel support.
var knownSixelTERMs = []string{
	"foot",
	"xterm",
	"st",
	"mintty",
	"rio",
	"wezterm",
	"contour",
}

// detectTERM applies the $TERM heuristics. A TERM ending in "sixel"
// always wins. Known sixel terminals match by exact name or "<name>-<...>"
// prefix, except xterm-kitty: kitty reports that TERM and does not
// implement sixel (it has its own graphics protocol), so it is excluded.
func detectTERM(term string) Format {
	if term == "" {
		return FormatSymbols
	}
	if strings.HasSuffix(term, "sixel") {
		return FormatSixel
	}
	if term == "xterm-kitty" {
		return FormatSymbols
	}
	for _, name := range knownSixelTERMs {
		if term == name || strings.HasPrefix(term, name+"-") {
			return FormatSixel
		}
	}
	return FormatSymbols
}
