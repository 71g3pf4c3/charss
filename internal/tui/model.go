// Package tui implements the charss terminal UI on Bubble Tea / Lip Gloss.
//
// The UI is a navigation/selection layer only: the feed list and article
// list live here, articles are displayed by the external chawan browser
// (via the handoff in browser.go) and images by chafa. Newsboat is the
// UX reference.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// screen is the currently visible list.
type screen int

const (
	screenFeedList screen = iota
	screenArticleList
)

// Model is the full reader state. It handles both screens; the article
// list is a mode over the same data, not a separate program.
type Model struct {
	keys   KeyMap
	feeds  []urls.Feed // urls-file order; the feed list shows this
	states map[string]*feedState

	// Injected dependencies; nil entries disable the related features
	// (see the per-key guards) instead of crashing.
	browser Browser
	fetcher Fetcher
	store   Storer
	term    Terminal

	screen  screen
	cursor  int    // feed list cursor
	acursor int    // article list cursor
	active  string // feed URL whose articles are shown

	opening    bool   // chawan is running (terminal handed off)
	refreshing int    // in-flight refreshes
	status     string // transient status line
	width      int
	height     int
	ready      bool
}

// Options configures New.
type Options struct {
	Feeds    []urls.Feed
	Browser  Browser
	Fetcher  Fetcher
	Store    Storer
	Terminal Terminal
	// Warning is shown once in the status line at startup (e.g. a missing
	// urls file, which is not an error).
	Warning string
}

// New returns the initial model.
func New(opts Options) Model {
	keys := DefaultKeyMap()
	states := make(map[string]*feedState, len(opts.Feeds))
	for _, f := range opts.Feeds {
		states[f.URL] = nil // lazily filled by loadStatesCmd / refreshes
	}
	return Model{
		keys:    keys,
		feeds:   opts.Feeds,
		states:  states,
		browser: opts.Browser,
		fetcher: opts.Fetcher,
		store:   opts.Store,
		term:    opts.Terminal,
		status:  opts.Warning,
	}
}

// Init implements tea.Model. Loading cached state only touches local
// disk, so the first render never waits on the network.
func (m Model) Init() tea.Cmd {
	if m.store == nil || len(m.feeds) == 0 {
		return nil
	}
	return loadStatesCmd(m.store, m.feeds)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m, nil

	case statesLoadedMsg:
		for u, st := range msg.states {
			m.states[u] = st
		}
		if len(msg.errs) > 0 {
			m.status = fmt.Sprintf("cache: %d feeds failed to load", len(msg.errs))
		}
		// Background auto-refresh after the cached data is on screen.
		return m.startRefresh(m.feeds)

	case refreshedMsg:
		return m.applyRefreshed(msg)

	case stateSavedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("save: %v", msg.err)
		}
		return m, nil

	case articleOpenedMsg:
		m.opening = false
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = ""
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.opening {
			// chawan owns the terminal; swallow anything queued.
			return m, nil
		}
		switch m.screen {
		case screenFeedList:
			return m.updateFeedList(msg)
		case screenArticleList:
			return m.updateArticleList(msg)
		}
	}
	return m, nil
}

// updateFeedList handles keys on the feed list.
func (m Model) updateFeedList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	case key.Matches(msg, m.keys.First):
		m.cursor = 0

	case key.Matches(msg, m.keys.Last):
		if len(m.feeds) > 0 {
			m.cursor = len(m.feeds) - 1
		}

	case key.Matches(msg, m.keys.Redraw):
		return m, tea.ClearScreen

	case key.Matches(msg, m.keys.Reload):
		if len(m.feeds) == 0 {
			m.status = "no feeds"
			return m, nil
		}
		return m.startRefresh([]urls.Feed{m.feeds[m.cursor]})

	case key.Matches(msg, m.keys.ReloadAll):
		return m.startRefresh(m.feeds)

	case key.Matches(msg, m.keys.OpenFeed):
		if len(m.feeds) == 0 {
			m.status = "no feeds — add URLs to your urls file"
			return m, nil
		}
		m.screen = screenArticleList
		m.active = m.feeds[m.cursor].URL
		m.acursor = 0
		m.status = ""
	}
	return m, nil
}

// updateArticleList handles keys on the article list.
func (m Model) updateArticleList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.states[m.active]
	arts := articlesOf(st)

	switch {
	case key.Matches(msg, m.keys.Back):
		m.screen = screenFeedList
		m.active = ""
		m.acursor = 0
		m.status = ""

	case key.Matches(msg, m.keys.Down):
		if m.acursor < len(arts)-1 {
			m.acursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.acursor > 0 {
			m.acursor--
		}

	case key.Matches(msg, m.keys.First):
		m.acursor = 0

	case key.Matches(msg, m.keys.Last):
		if len(arts) > 0 {
			m.acursor = len(arts) - 1
		}

	case key.Matches(msg, m.keys.Redraw):
		return m, tea.ClearScreen

	case key.Matches(msg, m.keys.ToggleRead):
		if len(arts) == 0 {
			break
		}
		a := arts[m.acursor]
		if st.read[a.ID] {
			delete(st.read, a.ID)
		} else {
			st.read[a.ID] = true
		}
		return m, saveStateCmd(m.store, m.active, st.toStore())

	case key.Matches(msg, m.keys.MarkAllRead):
		if st == nil || len(st.articles) == 0 {
			break
		}
		for _, a := range st.articles {
			st.read[a.ID] = true
		}
		m.status = fmt.Sprintf("marked %d articles read", len(st.articles))
		return m, saveStateCmd(m.store, m.active, st.toStore())

	case key.Matches(msg, m.keys.OpenArticle):
		if len(arts) == 0 {
			m.status = "no articles — press q and r to reload this feed"
			break
		}
		if m.browser == nil || m.term == nil {
			m.status = errNoBrowser.Error()
			break
		}
		a := arts[m.acursor]
		st.read[a.ID] = true // newsboat marks articles read on open
		m.opening = true
		return m, tea.Batch(
			saveStateCmd(m.store, m.active, st.toStore()),
			openArticleCmd(m.term, m.browser, articleHTML(a, m.feedTitle(m.active))),
		)
	}
	return m, nil
}

// startRefresh dispatches one fetch command per feed. Commands run
// concurrently as tea.Cmds; network concurrency is bounded by a shared
// semaphore inside the batch (goroutines are cheap, sockets are not).
func (m Model) startRefresh(feeds []urls.Feed) (tea.Model, tea.Cmd) {
	if m.fetcher == nil || len(feeds) == 0 {
		return m, nil
	}
	sem := make(chan struct{}, maxConcurrentFetches)
	cmds := make([]tea.Cmd, 0, len(feeds))
	for _, f := range feeds {
		var etag, lastModified string
		if st := m.states[f.URL]; st != nil {
			etag, lastModified = st.etag, st.lastModified
		}
		cmds = append(cmds, refreshFeedCmd(m.fetcher, sem, f, etag, lastModified))
	}
	m.refreshing += len(feeds)
	if len(feeds) > 1 {
		m.status = fmt.Sprintf("refreshing %d feeds…", len(feeds))
	} else {
		m.status = "refreshing…"
	}
	return m, tea.Batch(cmds...)
}

// stateFor returns the state of feedURL, creating an empty one when none
// exists yet.
func (m *Model) stateFor(feedURL string) *feedState {
	if st := m.states[feedURL]; st != nil {
		return st
	}
	st := &feedState{read: make(map[string]bool)}
	if m.states == nil {
		m.states = make(map[string]*feedState)
	}
	m.states[feedURL] = st
	return st
}

// articlesOf returns the article list (nil-safe).
func articlesOf(st *feedState) []feed.Article {
	if st == nil {
		return nil
	}
	return st.articles
}

// feedTitle resolves the display title for a feed URL.
func (m Model) feedTitle(feedURL string) string {
	for _, f := range m.feeds {
		if f.URL == feedURL {
			return feedTitle(f)
		}
	}
	return feedURL
}

// unreadTotal sums the unread counts across all feeds.
func (m Model) unreadTotal() int {
	n := 0
	for _, f := range m.feeds {
		n += m.states[f.URL].unread() // nil-safe receiver
	}
	return n
}
