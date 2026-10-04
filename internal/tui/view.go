package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/urls"
)

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

	unreadStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214"))

	articleUnreadStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15"))

	readStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	feedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))

	refreshStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))
)

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	switch m.screen {
	case screenArticleList:
		return m.articleListView()
	default:
		return m.feedListView()
	}
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

func (m Model) statusLine() string {
	if m.refreshing > 0 {
		return refreshStyle.Render(fmt.Sprintf("⟳ refreshing… %d left", m.refreshing))
	}
	if m.status != "" {
		return statusStyle.Render(truncate(m.status, maxInt(1, m.width)))
	}
	return ""
}

func (m Model) feedListView() string {
	var b strings.Builder

	header := "charss — feeds"
	if n := m.unreadTotal(); n > 0 {
		header += fmt.Sprintf("  (%d unread)", n)
	}
	b.WriteString(titleStyle.Render(header))
	b.WriteString("\n\n")

	if len(m.feeds) == 0 {
		b.WriteString(dimStyle.Render("no feeds — add URLs to your urls file"))
		b.WriteString("\n")
	}

	start, end := windowRange(m.cursor, len(m.feeds), m.listRows())
	for i := start; i < end; i++ {
		b.WriteString(m.feedLine(m.feeds[i], i == m.cursor))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(m.keys.feedListHelp()))
	return b.String()
}

func (m Model) feedLine(f urls.Feed, selected bool) string {
	title := feedTitle(f)
	if len(f.Tags) > 0 {
		title += " " + dimStyle.Render(strings.Join(f.Tags, " "))
	}
	n := m.states[f.URL].unread() // nil-safe receiver

	var count string
	if n > 0 {
		count = unreadStyle.Render(fmt.Sprintf("%4d", n))
	} else {
		count = dimStyle.Render(fmt.Sprintf("%4d", n))
	}
	line := count + "  " + title
	if selected {
		return selectedStyle.Render("▶ ") + line
	}
	return "  " + line
}

func (m Model) articleListView() string {
	var b strings.Builder

	st := m.states[m.active]
	header := "charss — " + m.feedTitle(m.active)
	if st != nil && len(st.articles) > 0 {
		if n := st.unread(); n > 0 {
			header += fmt.Sprintf("  (%d/%d unread)", n, len(st.articles))
		} else {
			header += fmt.Sprintf("  (%d articles, all read)", len(st.articles))
		}
	}
	b.WriteString(titleStyle.Render(header))
	b.WriteString("\n\n")

	arts := articlesOf(st)
	if len(arts) == 0 {
		b.WriteString(dimStyle.Render("no articles — press q and r to reload this feed"))
		b.WriteString("\n")
	}

	start, end := windowRange(m.acursor, len(arts), m.listRows())
	for i := start; i < end; i++ {
		b.WriteString(m.articleLine(st, arts[i], i == m.acursor))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(m.keys.articleListHelp()))
	return b.String()
}

func (m Model) articleLine(st *feedState, a feed.Article, selected bool) string {
	// Line layout: "N Jan 02  Title…". Keep the column budget in sync.
	const fixed = 2 + 2 + 6 + 2 // cursor, N mark, date, gaps
	mark := "  "
	if !st.read[a.ID] {
		mark = unreadStyle.Render("N ")
	}
	date := "      "
	if !a.Published.IsZero() {
		date = a.Published.Format("Jan 02")
	}
	budget := m.width - fixed - 2 // headroom for the cursor prefix
	if budget < 10 {
		budget = 10
	}
	line := mark + date + "  " + truncate(a.Title, budget)
	if !st.read[a.ID] {
		line = articleUnreadStyle.Render(line)
	} else {
		line = readStyle.Render(line)
	}
	if selected {
		return selectedStyle.Render("▶ ") + line
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
