// Package tui implements the charss terminal UI on Bubble Tea / Lip Gloss.
//
// The main view is the feed list (newsboat's first screen). Articles are
// rendered externally via chawan and images via chafa; the UI therefore
// stays a navigation/selection layer and never renders article HTML itself.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/71g3pf4c3/charss/internal/urls"
)

// KeyMap mirrors the newsboat feed-list bindings we support so far.
// Unhandled bindings must be added here, not inlined in Update.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Quit    key.Binding
	Reload  key.Binding
	Open    key.Binding
	Refresh key.Binding
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
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Reload: key.NewBinding(
			key.WithKeys("r", "R"),
			key.WithHelp("r", "reload"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter", "l", "right"),
			key.WithHelp("Enter", "open"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("Ctrl+R", "redraw"),
		),
	}
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62"))

	feedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
)

// Model is the feed-list screen state.
type Model struct {
	keys   KeyMap
	feeds  []urls.Feed
	cursor int
	status string // transient status line (errors, "not implemented" notes)
	width  int
	height int
	ready  bool
}

// New returns the initial feed-list model.
func New(feeds []urls.Feed) Model {
	return Model{keys: DefaultKeyMap(), feeds: feeds}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.feeds)-1 {
				m.cursor++
			}
			m.status = ""
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			m.status = ""
		case key.Matches(msg, m.keys.Reload):
			m.status = "reload: not implemented yet"
		case key.Matches(msg, m.keys.Open):
			if len(m.feeds) == 0 {
				m.status = "no feeds"
				break
			}
			m.status = fmt.Sprintf("opening %q in browser: not implemented yet", m.feeds[m.cursor].URL)
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("charss — feeds"))
	b.WriteString("\n\n")

	if len(m.feeds) == 0 {
		b.WriteString(dimStyle.Render("no feeds — add URLs to your urls file"))
		b.WriteString("\n")
	}

	for i, f := range m.feeds {
		title := f.Title
		if title == "" {
			title = f.URL
		}
		if len(f.Tags) > 0 {
			title += " " + dimStyle.Render(strings.Join(f.Tags, " "))
		}
		if i == m.cursor {
			title = selectedStyle.Render("▶ " + title)
		} else {
			title = feedStyle.Render("  " + title)
		}
		b.WriteString(title)
		b.WriteString("\n")
	}

	// Status line at the bottom; view is rebuilt on every update, so
	// transient status does not need separate lifecycle handling.
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.status))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(m.keys.help()))
	return b.String()
}

func (k KeyMap) help() string {
	bindings := []key.Binding{k.Up, k.Down, k.Open, k.Reload, k.Quit}
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, "  ")
}
