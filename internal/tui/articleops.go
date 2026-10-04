package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/podcast"
)

// articleSavedMsg reports the outcome of a save-article write.
type articleSavedMsg struct {
	path string
	err  error
}

// autoReloadTickMsg is delivered by the periodic auto-reload timer.
type autoReloadTickMsg struct{}

// autoReloadCmd schedules the next auto-reload tick. A non-positive
// interval disables the loop (the returned command is nil).
func autoReloadCmd(d time.Duration) tea.Cmd {
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg {
		return autoReloadTickMsg{}
	})
}

// ---- flags (newsboat ^) -------------------------------------------------

// openFlagPrompt opens the one-character flag prompt for r.
func (m Model) openFlagPrompt(r articleRef) (tea.Model, tea.Cmd) {
	m.flagTarget = r
	return m.openInput(inputFlag)
}

// applyFlagInput is the prompt submit: value must be exactly one valid
// flag character, which is toggled on the snapshotted article.
func (m Model) applyFlagInput(value string) (tea.Model, tea.Cmd) {
	if !validFlag(value) {
		m.status = "flag: expected one character a-z, A-Z or 0-9"
		return m, nil
	}
	return m.applyFlag(m.flagTarget, value)
}

// applyFlag toggles one flag character on r and persists the feed state.
func (m Model) applyFlag(r articleRef, char string) (tea.Model, tea.Cmd) {
	after := toggleFlag(m.flagOf(r.feedURL, r.art.ID), char[0])
	m.setFlagOf(r.feedURL, r.art.ID, after)
	st := m.stateFor(r.feedURL)
	if strings.Contains(after, char) {
		m.status = "flag " + char + " set"
	} else {
		m.status = "flag " + char + " cleared"
	}
	return m, saveStateCmd(m.store, r.feedURL, st.toStore())
}

// ---- save-article (newsboat s) -------------------------------------------

// openSavePrompt opens the destination prompt with newsboat's ~/ default.
func (m Model) openSavePrompt(r articleRef) (tea.Model, tea.Cmd) {
	m.saveTarget = r
	m.inputKind = inputSavePath
	m.input.Prompt = "Save to: "
	m.input.SetValue("~/")
	m.input.Focus()
	m.input.Width = maxInt(0, m.width-len([]rune(m.input.Prompt))-2)
	m.status = ""
	return m, textinput.Blink
}

// applySaveInput is the prompt submit: the article is written to the
// (expanded) path, overwriting silently like newsboat.
func (m Model) applySaveInput(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		return m, nil
	}
	return m.saveArticle(m.saveTarget, path)
}

// saveArticle writes r's text rendering to path (~ expanded) in a
// command, so Update stays free of file I/O.
func (m Model) saveArticle(r articleRef, path string) (tea.Model, tea.Cmd) {
	text := saveArticleText(r.art, r.feedTitle)
	return m, saveArticleCmd(path, text)
}

// saveArticleCmd expands ~, writes the file (mode 0600: saved articles
// are personal data) and reports back as a message.
func saveArticleCmd(path, text string) tea.Cmd {
	return func() tea.Msg {
		p := expandHome(path)
		if p == "" {
			return articleSavedMsg{err: os.ErrInvalid}
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			return articleSavedMsg{path: p, err: err}
		}
		return articleSavedMsg{path: p}
	}
}

// expandHome resolves a leading ~ to the user's home directory.
func expandHome(path string) string {
	if path == "~" || len(path) >= 2 && path[0] == '~' && (path[1] == '/' || path[1] == '\\') {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// ---- enqueue (newsboat e) -------------------------------------------------

// enqueueEnclosure adds r's first enclosure to the podcast download
// queue; an article without enclosures reports a status error.
func (m Model) enqueueEnclosure(r articleRef) (tea.Model, tea.Cmd) {
	if len(r.art.Enclosures) == 0 {
		m.status = "no enclosure to enqueue"
		return m, nil
	}
	if m.enqueuer == nil {
		m.status = "podcast queue unavailable"
		return m, nil
	}
	e := r.art.Enclosures[0]
	err := m.enqueuer.Add(podcast.Item{
		URL:      e.URL,
		Title:    r.art.Title,
		FeedURL:  r.feedURL,
		MimeType: e.MimeType,
		Size:     e.Size,
	})
	if err != nil {
		m.status = "enqueue: " + err.Error()
		return m, nil
	}
	m.status = "queued: " + e.URL
	return m, nil
}

// ---- notifications ---------------------------------------------------------

// countNewArticles returns how many of fresh's IDs are absent from old.
func countNewArticles(old, fresh []feed.Article) int {
	if len(fresh) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(old))
	for _, a := range old {
		seen[a.ID] = struct{}{}
	}
	n := 0
	for _, a := range fresh {
		if _, ok := seen[a.ID]; !ok {
			n++
		}
	}
	return n
}
