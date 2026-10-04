package tui

import (
	"fmt"
	"html"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/feed"
)

// The show-urls screen (newsboat's urlview): a numbered list of the
// article's URLs — the article link first, then every <a href> from the
// content in document order, then enclosure URLs. Duplicates collapse to
// their first occurrence. Enter opens the selection in the external
// browser through the same handoff as article display; q/Esc returns.

// extractURLs builds the numbered URL list for the show-urls screen.
func extractURLs(a feed.Article) []string {
	var urls []string
	seen := map[string]bool{}
	add := func(u string) {
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		urls = append(urls, u)
	}
	add(a.URL)
	for _, u := range extractHrefs(a.ContentHTML) {
		add(u)
	}
	for _, e := range a.Enclosures {
		add(e.URL)
	}
	return urls
}

// extractHrefs collects href attribute values from anchor tags in
// document order, using the same quote-aware tag scanner as stripHTML.
func extractHrefs(s string) []string {
	var hrefs []string
	i := 0
	for i < len(s) {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			break
		}
		i += lt
		end := scanTagEnd(s, i)
		if end < 0 {
			break
		}
		if name, _ := tagName(s[i+1 : end]); name == "a" {
			if href := attrValue(s[i+1:end], "href"); href != "" {
				hrefs = append(hrefs, href)
			}
		}
		i = end + 1
	}
	return hrefs
}

// attrValue extracts an attribute's value from a tag body, accepting
// single or double quotes and unquoted values; "" when absent.
func attrValue(tagBody, name string) string {
	for i := 0; i+len(name) < len(tagBody); i++ {
		if !attrNameStart(tagBody, i, name) {
			continue
		}
		j := i + len(name)
		for j < len(tagBody) && (tagBody[j] == ' ' || tagBody[j] == '\t' || tagBody[j] == '\n' || tagBody[j] == '\r') {
			j++
		}
		if j >= len(tagBody) || tagBody[j] != '=' {
			continue
		}
		j++
		for j < len(tagBody) && (tagBody[j] == ' ' || tagBody[j] == '\t' || tagBody[j] == '\n' || tagBody[j] == '\r') {
			j++
		}
		if j >= len(tagBody) {
			return ""
		}
		switch q := tagBody[j]; q {
		case '"', '\'':
			if k := strings.IndexByte(tagBody[j+1:], q); k >= 0 {
				return html.UnescapeString(tagBody[j+1 : j+1+k])
			}
			return ""
		default:
			k := j
			for k < len(tagBody) && !strings.ContainsRune(" \t\n\r", rune(tagBody[k])) {
				k++
			}
			return html.UnescapeString(tagBody[j:k])
		}
	}
	return ""
}

// attrNameStart reports whether the attribute name starts at tagBody[i],
// excluding hyphenated look-alikes (data-href must not match "href").
func attrNameStart(tagBody string, i int, name string) bool {
	if !strings.EqualFold(tagBody[i:i+len(name)], name) {
		return false
	}
	boundary := func(c byte) bool { return isNameByte(c) || c == '-' }
	if i > 0 && boundary(tagBody[i-1]) {
		return false
	}
	next := i + len(name)
	return next >= len(tagBody) || !boundary(tagBody[next])
}

// updateURLView dispatches one key on the URL view through the dialog
// context (newsboat's urlview equivalent).
func (m Model) updateURLView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.urlViewOp(m.lookupOp(msg), "")
}

// urlViewOp applies one operation to the URL view; macro arguments do
// not apply here (no URL-view op takes one), arg is accepted for
// dispatchOp-shape parity.
func (m Model) urlViewOp(op, arg string) (tea.Model, tea.Cmd) {
	_ = arg
	switch op {
	case config.OpOpen:
		if len(m.urlEntries) == 0 {
			return m, nil
		}
		if m.browser == nil || m.term == nil {
			m.status = errNoBrowser.Error()
			return m, nil
		}
		m.opening = true
		return m, openURLCmd(m.term, m.browser, m.urlEntries[m.ucursor])

	case config.OpQuit:
		m.screen = m.urlFrom
		m.status = ""

	case config.OpUp, config.OpPrev:
		if m.ucursor > 0 {
			m.ucursor--
		}

	case config.OpDown, config.OpNext:
		if m.ucursor < len(m.urlEntries)-1 {
			m.ucursor++
		}

	case config.OpFirst:
		m.ucursor = 0

	case config.OpLast:
		if len(m.urlEntries) > 0 {
			m.ucursor = len(m.urlEntries) - 1
		}

	case config.OpPageUp:
		m.ucursor = clampInt(m.ucursor-m.listRows(), 0, len(m.urlEntries)-1)

	case config.OpPageDown:
		m.ucursor = clampInt(m.ucursor+m.listRows(), 0, len(m.urlEntries)-1)
	}
	return m, nil
}

// urlView renders the numbered URL list, windowed like the help screen.
func (m Model) urlView() string {
	var b strings.Builder
	title := "URLs"
	if m.urlTitle != "" {
		title = "URLs — " + m.urlTitle
	}
	b.WriteString(m.sty.title.Render("charss — " + title))
	b.WriteString("\n\n")

	if len(m.urlEntries) == 0 {
		b.WriteString(m.sty.dim.Render("no URLs"))
		b.WriteString("\n")
	}

	start, end := windowRange(m.ucursor, len(m.urlEntries), m.listRows())
	for i := start; i < end; i++ {
		line := fmt.Sprintf("%2d  %s", i+1, m.urlEntries[i])
		if i == m.ucursor {
			b.WriteString(m.sty.selected.Render("▶ ") + m.sty.selected.Render(truncate(line, maxInt(1, m.width-2))))
		} else {
			b.WriteString("  " + m.sty.dim.Render(truncate(line, maxInt(1, m.width-2))))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.sty.dim.Render("ENTER opens · q/Esc closes"))
	return b.String()
}
