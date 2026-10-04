package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/tui"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// runTUI loads the feed list and starts the Bubble Tea program.
func runTUI(cfg *config.Config) error {
	feeds, warn, err := loadFeeds(cfg.URLsFile)
	if err != nil {
		return err
	}

	m := tui.New(feeds)
	if warn != "" {
		// A missing urls file is not fatal (newsboat treats it the same way
		// and shows an empty list); surface it in the UI later.
		_ = warn
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
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
