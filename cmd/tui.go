package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/render"
	"github.com/71g3pf4c3/charss/internal/store"
	"github.com/71g3pf4c3/charss/internal/tui"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// runTUI loads the feed list and starts the Bubble Tea program.
func runTUI(cfg *config.Config) error {
	feeds, warn, err := loadFeeds(cfg.URLsFile)
	if err != nil {
		return err
	}

	// Per-feed cache under $XDG_CACHE_HOME/charss/feeds (store.Open("")
	// resolves it via os.UserCacheDir). Creation failure is fatal: the
	// reader is useless without persistence.
	st, err := store.Open("")
	if err != nil {
		return err
	}

	// The terminal adapter is attached to the program after NewProgram:
	// tea.NewProgram copies the model, so the adapter must be a shared
	// pointer for every model copy to see it.
	term := &tui.ProgramTerminal{}

	m := tui.New(tui.Options{
		Feeds:    feeds,
		Browser:  render.New(render.Options{Binary: cfg.Browser}),
		Fetcher:  feed.NewFetcher(),
		Store:    st,
		Terminal: term,
		Warning:  warn, // missing urls file: empty list, not an error
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	term.Attach(p)
	_, err = p.Run()
	return err
}

// loadFeeds reads and parses the urls file. A missing file yields an empty
// feed list and a warning; a malformed file is a hard error.
func loadFeeds(path string) (feeds []urls.Feed, warn string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Sprintf("urls file %s not found", path), nil
		}
		return nil, "", fmt.Errorf("reading urls file: %w", err)
	}
	feeds, err = urls.Parse(string(data))
	if err != nil {
		return nil, "", err
	}
	return feeds, "", nil
}
