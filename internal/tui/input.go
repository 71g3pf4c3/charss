package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/filter"
	"github.com/71g3pf4c3/charss/internal/search"
)

// openInput starts the single-line status-position input collecting
// kind. The prompt mirrors newsboat's ("Filter: " / "Search: ").
func (m Model) openInput(kind inputKind) (tea.Model, tea.Cmd) {
	m.inputKind = kind
	m.input.SetValue("")
	m.input.Prompt = ": "
	switch kind {
	case inputFilter:
		m.input.Prompt = "Filter: "
	case inputSearch:
		m.input.Prompt = "Search: "
	case inputFlag:
		m.input.Prompt = "Flag: "
	case inputSavePath:
		m.input.Prompt = "Save to: "
	}
	m.input.Focus()
	m.input.Width = maxInt(0, m.width-len([]rune(m.input.Prompt))-2)
	m.status = ""
	return m, textinput.Blink
}

// closeInput dismisses the input without applying anything.
func (m *Model) closeInput() {
	m.inputKind = inputNone
	m.input.Blur()
}

// updateInput handles keys while the filter/search/flag/save input is
// open: Enter submits, Esc cancels, everything else goes to the text
// input.
func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		value := m.input.Value()
		kind := m.inputKind
		m.closeInput()
		switch kind {
		case inputFilter:
			return m.applyFilterInput(value)
		case inputSearch:
			return m.applySearchInput(value)
		case inputFlag:
			return m.applyFlagInput(value)
		case inputSavePath:
			return m.applySaveInput(value)
		}
		return m, nil

	case tea.KeyEsc:
		m.closeInput()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// applyFilterInput compiles a filter expression typed into the input
// and installs it on the active screen's list. An invalid expression
// reports a status error and keeps the previous filter (newsboat
// behavior); an empty value clears the filter.
func (m Model) applyFilterInput(expr string) (tea.Model, tea.Cmd) {
	if expr == "" {
		if m.screen == screenFeedList {
			m.feedFilter = nil
			m.cursor = 0
		} else {
			m.artFilter = nil
			m.acursor = 0
		}
		m.status = "filter cleared"
		return m, nil
	}
	flt, err := filter.Compile(expr)
	if err != nil {
		m.status = err.Error()
		return m, nil // keep the previous filter
	}
	if m.screen == screenFeedList {
		m.feedFilter = flt
		m.cursor = clampInt(m.cursor, 0, len(m.feedRows())-1)
	} else {
		m.artFilter = flt
		m.acursor = clampInt(m.acursor, 0, len(m.articleRefs())-1)
	}
	m.status = ""
	return m, nil
}

// applySearchInput parses and runs a search over all loaded feeds and
// shows the results as a virtual article list titled Search: "<query>".
// A syntax error (e.g. a bad regex) reports a status line error; an
// empty query cancels.
func (m Model) applySearchInput(query string) (tea.Model, tea.Cmd) {
	if query == "" {
		return m, nil
	}
	q, err := search.Parse(query)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	results := search.Run(q, m.searchSources())
	m.searchQuery = query
	m.searchResults = results // hit ranges kept on the state for highlighting
	m.searchOn = true
	m.screen = screenArticleList
	m.active = ""
	m.acursor = 0
	if len(results) == 0 {
		m.status = "no results"
	} else {
		m.status = fmt.Sprintf("%d results", len(results))
	}
	return m, nil
}

// searchSources adapts the loaded real feeds to search sources.
func (m Model) searchSources() []search.Source {
	feeds := realFeeds(m.feeds)
	sources := make([]search.Source, 0, len(feeds))
	for _, f := range feeds {
		st := m.states[f.URL]
		if st == nil {
			continue
		}
		read := st.read
		if read == nil {
			read = map[string]bool{}
		}
		sources = append(sources, search.Source{
			FeedURL:   f.URL,
			FeedTitle: feedTitle(f),
			Articles:  st.articles,
			Read:      read,
		})
	}
	return sources
}
