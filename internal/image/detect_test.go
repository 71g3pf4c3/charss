package image

import "testing"

func TestDetect(t *testing.T) {
	no := false
	yes := true

	tests := []struct {
		name  string
		term  string
		force *bool
		want  Format
	}{
		{name: "empty TERM", term: "", want: FormatSymbols},
		{name: "foot", term: "foot", want: FormatSixel},
		{name: "foot-extra", term: "foot-extra", want: FormatSixel},
		{name: "xterm-256color", term: "xterm-256color", want: FormatSixel},
		{name: "plain xterm", term: "xterm", want: FormatSixel},
		{name: "st", term: "st", want: FormatSixel},
		{name: "st-256color", term: "st-256color", want: FormatSixel},
		{name: "mintty", term: "mintty", want: FormatSixel},
		{name: "rio", term: "rio", want: FormatSixel},
		{name: "wezterm", term: "wezterm", want: FormatSixel},
		{name: "contour", term: "contour", want: FormatSixel},
		{name: "sixel suffix wins", term: "vt340-sixel", want: FormatSixel},
		{name: "alacritty has no sixel", term: "alacritty", want: FormatSymbols},
		{name: "alacritty-direct", term: "alacritty-direct", want: FormatSymbols},
		{name: "kitty TERM excluded", term: "xterm-kitty", want: FormatSymbols},
		{name: "screen", term: "screen-256color", want: FormatSymbols},
		{name: "tmux", term: "tmux-256color", want: FormatSymbols},
		{name: "force true overrides bad TERM", term: "alacritty", force: &yes, want: FormatSixel},
		{name: "force false overrides good TERM", term: "foot", force: &no, want: FormatSymbols},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", tt.term)
			if got := Detect(tt.force); got != tt.want {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}
