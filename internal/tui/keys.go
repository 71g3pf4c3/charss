package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap holds the newsboat-compatible bindings for both screens.
// Unhandled bindings must be added here, not inlined in Update.
type KeyMap struct {
	// Shared.
	Up     key.Binding
	Down   key.Binding
	First  key.Binding // g / Home — jump to first line
	Last   key.Binding // G / End — jump to last line
	Redraw key.Binding

	// Feed list.
	Quit      key.Binding // q
	Reload    key.Binding // r — reload current feed
	ReloadAll key.Binding // R — reload all feeds
	OpenFeed  key.Binding // Enter / l / Right — open article list

	// Article list.
	Back        key.Binding // q / Esc / Left / h
	OpenArticle key.Binding // Enter / o — open in chawan
	ToggleRead  key.Binding // m
	MarkAllRead key.Binding // A
}

// DefaultKeyMap is the newsboat-compatible default binding set.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		First: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "first"),
		),
		Last: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "last"),
		),
		Redraw: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("Ctrl+R", "redraw"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		Reload: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reload"),
		),
		ReloadAll: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "reload all"),
		),
		OpenFeed: key.NewBinding(
			key.WithKeys("enter", "l", "right"),
			key.WithHelp("Enter", "open"),
		),
		Back: key.NewBinding(
			key.WithKeys("q", "esc", "left", "h"),
			key.WithHelp("q/Esc", "back"),
		),
		OpenArticle: key.NewBinding(
			key.WithKeys("enter", "o"),
			key.WithHelp("Enter/o", "open"),
		),
		ToggleRead: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "read"),
		),
		MarkAllRead: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "all read"),
		),
	}
}

// feedListHelp renders the one-line keymap footer for the feed list.
func (k KeyMap) feedListHelp() string {
	return joinHelp(k.Up, k.Down, k.First, k.Last, k.OpenFeed, k.Reload, k.ReloadAll, k.Quit)
}

// articleListHelp renders the one-line keymap footer for the article list.
func (k KeyMap) articleListHelp() string {
	return joinHelp(k.Up, k.Down, k.First, k.Last, k.OpenArticle, k.ToggleRead, k.MarkAllRead, k.Back)
}

func joinHelp(bindings ...key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, "  ")
}
