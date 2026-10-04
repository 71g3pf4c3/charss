package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	var v string
	switch m.screen {
	case screenArticleList:
		v = m.articleListView()
	case screenHelp:
		v = m.helpView()
	case screenURLView:
		v = m.urlView()
	default:
		v = m.feedListView()
	}
	if m.sty.background != "" {
		return lipgloss.NewStyle().Background(m.sty.background).Render(v)
	}
	return v
}

// listRows is the number of list lines that fit given the fixed header
// (title + blank), status and help lines.
func (m Model) listRows() int {
	rows := m.height - 4
	if rows < 1 {
		return 1
	}
	return rows
}

// statusLine renders the status area: the filter/search input while it
// is open, otherwise the transient status message. The status line
// belongs to messages and errors only; refresh progress lives in the
// header (refreshBadge) so a long refresh can never hide an error like
// "chawan not found".
func (m Model) statusLine() string {
	if m.inputKind != inputNone {
		return m.input.View()
	}
	if m.status != "" {
		return m.sty.status.Render(truncate(m.status, maxInt(1, m.width)))
	}
	return ""
}

// refreshBadge is the in-flight refresh counter shown in the screen header.
func (m Model) refreshBadge() string {
	if m.refreshing > 0 {
		return " " + m.sty.refresh.Render(fmt.Sprintf("⟳ %d", m.refreshing))
	}
	return ""
}

func (m Model) feedListView() string {
	var b strings.Builder

	rows := m.feedRows()
	header := "charss — feeds"
	if n := m.unreadTotal(); n > 0 {
		header += fmt.Sprintf("  (%d unread)", n)
	}
	if m.feedFilter != nil {
		header += "  [filtered]"
	}
	b.WriteString(m.sty.title.Render(header))
	b.WriteString(m.refreshBadge())
	b.WriteString("\n\n")

	if len(rows) == 0 {
		if m.feedFilter != nil {
			b.WriteString(m.sty.dim.Render("no feeds match the filter"))
		} else {
			b.WriteString(m.sty.dim.Render("no feeds — add URLs to your urls file"))
		}
		b.WriteString("\n")
	}

	start, end := windowRange(m.cursor, len(rows), m.listRows())
	for i := start; i < end; i++ {
		b.WriteString(m.feedLine(rows[i], i == m.cursor))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(m.sty.dim.Render(m.feedListHelp()))
	return b.String()
}

func (m Model) feedLine(r feedRow, selected bool) string {
	title := r.title
	if len(r.tags) > 0 {
		title += " " + m.sty.dim.Render(strings.Join(r.tags, " "))
	}
	if r.queryErr != nil {
		title += " " + m.sty.status.Render("(invalid filter)")
	}

	var count string
	if r.unread > 0 {
		count = m.sty.unreadCount.Render(fmt.Sprintf("%4d", r.unread))
	} else {
		count = m.sty.dim.Render(fmt.Sprintf("%4d", r.unread))
	}
	line := count + "  " + title
	if selected {
		return m.sty.selected.Render("▶ ") + line
	}
	return "  " + line
}

func (m Model) articleListView() string {
	var b strings.Builder

	refs := m.articleRefs()
	header := "charss — " + m.activeTitle()
	if m.searchOn {
		if len(refs) > 0 {
			header += fmt.Sprintf("  (%d results)", len(refs))
		}
	} else if n := m.activeUnread(); n > 0 {
		if m.artFilter != nil {
			header += fmt.Sprintf("  (filtered %d/%d)", len(refs), m.activeTotal())
		} else {
			header += fmt.Sprintf("  (%d/%d unread)", n, m.activeTotal())
		}
	} else if m.activeTotal() > 0 {
		if m.artFilter != nil {
			header += fmt.Sprintf("  (filtered %d/%d)", len(refs), m.activeTotal())
		} else {
			header += fmt.Sprintf("  (%d articles, all read)", m.activeTotal())
		}
	} else if m.artFilter != nil {
		header += fmt.Sprintf("  (filtered %d/%d)", len(refs), m.activeTotal())
	}
	b.WriteString(m.sty.title.Render(header))
	b.WriteString(m.refreshBadge())
	b.WriteString("\n\n")

	if len(refs) == 0 {
		switch {
		case m.artFilter != nil:
			b.WriteString(m.sty.dim.Render("no articles match the filter"))
		case m.searchOn:
			b.WriteString(m.sty.dim.Render("no articles match the search"))
		default:
			b.WriteString(m.sty.dim.Render("no articles — press q and r to reload this feed"))
		}
		b.WriteString("\n")
	}

	start, end := windowRange(m.acursor, len(refs), m.listRows())
	for i := start; i < end; i++ {
		b.WriteString(m.articleLine(refs[i], i == m.acursor))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(m.sty.dim.Render(m.articleListHelp()))
	return b.String()
}

// activeUnread counts unread articles in the unfiltered active list.
func (m Model) activeUnread() int {
	n := 0
	for _, r := range m.activeAllRefs() {
		if !m.readOf(r.feedURL, r.art.ID) {
			n++
		}
	}
	return n
}

// activeTotal is the size of the unfiltered active list (the M in
// "(filtered N/M)").
func (m Model) activeTotal() int {
	return len(m.activeAllRefs())
}

// activeAllRefs builds the active article list without the filter and
// the show-read toggle (but with the configured sort), so filtered
// counts can be compared against the full list.
func (m Model) activeAllRefs() []articleRef {
	savedFilter, savedRead := m.artFilter, m.showRead
	m.artFilter, m.showRead = nil, true
	refs := m.articleRefs()
	m.artFilter, m.showRead = savedFilter, savedRead
	return refs
}

func (m Model) articleLine(r articleRef, selected bool) string {
	// Line layout: "N Jan 02  [a] Title…". Keep the column budget in
	// sync. The flag marker is newsboat's [a] style; unflagged articles
	// render without it (newsboat marks them "!", a deliberate task
	// deviation recorded in the wave-3 commit).
	const fixed = 2 + 2 + 6 + 2 // cursor, N mark, date, gaps
	unread := !m.readOf(r.feedURL, r.art.ID)
	mark := "  "
	if unread {
		mark = m.sty.unreadCount.Render("N ")
	}
	date := "      "
	if !r.art.Published.IsZero() {
		date = r.art.Published.Format("Jan 02")
	}
	title := r.art.Title
	if flags := m.flagOf(r.feedURL, r.art.ID); flags != "" {
		title = "[" + flags + "] " + title
	}
	budget := m.width - fixed - 2 // headroom for the cursor prefix
	if budget < 10 {
		budget = 10
	}
	line := mark + date + "  " + truncate(title, budget)
	if unread {
		line = m.sty.articleUnread.Render(line)
	} else {
		line = m.sty.read.Render(line)
	}
	if selected {
		if unread {
			return m.sty.selectedUnread.Render("▶ ") + line
		}
		return m.sty.selected.Render("▶ ") + line
	}
	return "  " + line
}

// windowRange returns the [start, end) slice of lines to render for a
// list of total items with the given cursor and visible rows, keeping
// the cursor centered. Stateless, so resizes cannot leave a stale offset.
func windowRange(cursor, total, rows int) (int, int) {
	if rows < 1 {
		rows = 1
	}
	if total <= 0 {
		return 0, 0
	}
	if total <= rows {
		return 0, total
	}
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	if maxStart := total - rows; start > maxStart {
		start = maxStart
	}
	return start, start + rows
}

// truncate cuts s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
