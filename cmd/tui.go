package cmd

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/podcast"
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

	// Podcast queue (same cache dir as the store). A failure to open it
	// is a warning: enqueue reports "queue unavailable" instead of
	// crashing the reader over a cache-dir problem.
	queue, qerr := podcast.OpenQueue("")
	if qerr != nil {
		warnQueue := fmt.Sprintf("podcast queue: %v", qerr)
		if warn != "" {
			warn = warn + "; " + warnQueue
		} else {
			warn = warnQueue
		}
		queue = nil
	}

	warnBrowser := checkBrowser(cfg.Browser)
	if warn != "" && warnBrowser != "" {
		warn = warn + "; " + warnBrowser
	} else if warnBrowser != "" {
		warn = warnBrowser
	}

	m := tui.New(tui.Options{
		Feeds:    feeds,
		Browser:  render.New(render.Options{Binary: cfg.Browser}),
		Fetcher:  feed.NewFetcher(),
		Store:    st,
		Terminal: term,
		Enqueue:  queue, // nil on open failure; the op degrades to a status
		Config:   cfg,   // bindings, colors and options drive the TUI
		Warning:  warn,  // missing urls file / missing browser: warnings, not errors
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	term.Attach(p)
	_, err = p.Run()
	return err
}

// checkBrowser returns a warning when the configured article browser is
// not on PATH. Articles are rendered externally (chawan); without it,
// opening an article can only fail — say so up front, not after the fact.
func checkBrowser(binary string) string {
	if binary == "" {
		binary = "chawan"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Sprintf("browser %q not found — install chawan (https://chawan.net) or set `browser` in the config", binary)
	}
	return ""
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
