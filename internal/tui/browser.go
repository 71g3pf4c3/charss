package tui

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// articleOpenedMsg is delivered after the external browser has exited and
// the terminal has been handed back — for both article display and URL
// opens. err is nil when the browser exited 0; otherwise it carries the
// browser error (which includes the install hint when the binary is
// missing) and/or a restore failure.
type articleOpenedMsg struct {
	err error
}

// handoffCmd implements the Bubble Tea ↔ external-browser terminal
// handoff shared by article display and URL opens:
//
//  1. release the terminal (Bubble Tea suspends its input reader and
//     renderer, and restores the underlying terminal state);
//  2. run run in the foreground — the browser owns the terminal while
//     it runs;
//  3. restore the terminal (Bubble Tea re-captures input, re-enters
//     the alt screen and repaints);
//  4. report the outcome as a message, never by panicking.
//
// The restore is best-effort on every path: even if the browser fails,
// the TUI must come back to draw the status line. Release failures abort
// the handoff before anything is spawned.
func handoffCmd(term Terminal, run func() error) tea.Cmd {
	return func() tea.Msg {
		if err := term.Release(); err != nil {
			return articleOpenedMsg{err: fmt.Errorf("releasing terminal: %w", err)}
		}

		// No timeout: reading time is the user's, as in newsboat.
		err := run()

		if rerr := term.Restore(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("restoring terminal: %w", rerr))
		}
		return articleOpenedMsg{err: err}
	}
}

// openArticleCmd hands the terminal to chawan to display an article.
func openArticleCmd(term Terminal, b Browser, html string) tea.Cmd {
	return handoffCmd(term, func() error {
		return b.ShowHTML(context.Background(), html)
	})
}

// openURLCmd hands the terminal to the browser to display a URL (feed
// link, article link, URL-view selection) — the same handoff, so the
// degenerate-WindowSizeMsg guard and status handling apply unchanged.
func openURLCmd(term Terminal, b Browser, url string) tea.Cmd {
	return handoffCmd(term, func() error {
		return b.ShowURL(context.Background(), url)
	})
}

// articleHTML assembles a minimal standalone HTML document for the
// external renderer. This is document assembly, not rendering — chawan
// does all layout. Empty content still yields a valid page with the title
// and source link so the user is not staring at a blank screen.
func articleHTML(a feed.Article, feedTitle string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>")
	b.WriteString(html.EscapeString(firstNonEmpty(a.Title, feedTitle, a.URL)))
	b.WriteString("</title>\n</head>\n<body>\n")

	if a.Title != "" {
		b.WriteString("<h1>")
		b.WriteString(html.EscapeString(a.Title))
		b.WriteString("</h1>\n")
	}

	var meta []string
	if a.Author != "" {
		meta = append(meta, html.EscapeString(a.Author))
	}
	if !a.Published.IsZero() {
		meta = append(meta, a.Published.Format(time.DateOnly))
	}
	if a.URL != "" {
		meta = append(meta, `<a href="`+html.EscapeString(a.URL)+`">source</a>`)
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(strings.Join(meta, " · "))
		b.WriteString("</p>\n")
	}

	content := a.ContentHTML
	if strings.TrimSpace(content) == "" {
		content = "<p>(no content)</p>"
	}
	b.WriteString(content)
	b.WriteString("\n</body>\n</html>\n")
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
