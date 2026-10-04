package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/71g3pf4c3/charss/internal/config"
)

// styles is the resolved style set used by the views. It starts as the
// hardcoded default palette and is overlaid with the config's color
// rules (see applyColorRules).
type styles struct {
	title          lipgloss.Style // header bar
	selected       lipgloss.Style // focused row
	selectedUnread lipgloss.Style // focused unread row
	articleUnread  lipgloss.Style // unread article row
	unreadCount    lipgloss.Style // unread counter in the feed list
	read           lipgloss.Style // read article row
	feed           lipgloss.Style // feed row text
	dim            lipgloss.Style // hints, help footer
	status         lipgloss.Style // status line
	refresh        lipgloss.Style // refresh badge
	background     lipgloss.Color // `background` color rule ("" = none)
}

// defaultStyles is the built-in palette used when no color rules apply.
func defaultStyles() styles {
	return styles{
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1),
		selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")),
		selectedUnread: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")),
		articleUnread: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")),
		unreadCount: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")),
		read: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")),
		feed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")),
		status: lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")),
		refresh: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")),
	}
}

// applyColorRules overlays the config's `color` directives (in file
// order, last one wins per element) onto the default palette. Unknown
// elements and unparsable colors are ignored, like newsboat keeps
// going on unknown attributes.
func applyColorRules(rules []config.ColorRule) styles {
	sty := defaultStyles()
	for _, r := range rules {
		s := ruleStyle(r)
		switch r.Element {
		case config.ColorList:
			sty.read, sty.feed = s, s
		case config.ColorListNormal:
			sty.read, sty.feed = s, s
		case config.ColorListNormalUnread:
			sty.articleUnread = s
		case config.ColorListFocus:
			sty.selected, sty.selectedUnread = s, s
		case config.ColorListFocusUnread:
			sty.selectedUnread = s
		case config.ColorInfo:
			sty.status = s
		case config.ColorBackground:
			if c, ok := colorValue2(r.Fg); ok {
				sty.background = c
			}
		case config.ColorTitle:
			sty.title = s
		case config.ColorHint:
			sty.dim = s
		}
	}
	return sty
}

// ruleStyle converts one color rule to a lipgloss style.
func ruleStyle(r config.ColorRule) lipgloss.Style {
	s := lipgloss.NewStyle()
	if c, ok := colorValue2(r.Fg); ok {
		s = s.Foreground(c)
	}
	if c, ok := colorValue2(r.Bg); ok {
		s = s.Background(c)
	}
	for _, attr := range r.Attrs {
		switch attr {
		case "bold":
			s = s.Bold(true)
		case "underline":
			s = s.Underline(true)
		case "reverse", "standout":
			s = s.Reverse(true)
		case "blink":
			s = s.Blink(true)
		case "dim":
			s = s.Faint(true)
		case "italics":
			s = s.Italic(true)
		}
	}
	return s
}

// namedColors maps newsboat's basic color names to ANSI codes.
var namedColors = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3,
	"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
}

// colorValue2 parses a newsboat color value ("default", named, colorN,
// #rrggbb) into a lipgloss color; ok is false for "default" and
// unparsable values.
func colorValue2(v string) (lipgloss.Color, bool) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == "default":
		return "", false
	case strings.HasPrefix(v, "#"):
		return lipgloss.Color(v), true
	case strings.HasPrefix(v, "color"):
		if n, err := strconv.Atoi(v[len("color"):]); err == nil && n >= 0 && n <= 255 {
			return lipgloss.Color(strconv.Itoa(n)), true
		}
		return "", false
	}
	if n, ok := namedColors[v]; ok {
		return lipgloss.Color(strconv.Itoa(n)), true
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 255 {
		return lipgloss.Color(strconv.Itoa(n)), true
	}
	return "", false
}
