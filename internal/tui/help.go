package tui

import (
	"fmt"
	"strings"

	"github.com/71g3pf4c3/charss/internal/config"
)

// helpRow is one line of the help table: a key and the operation it is
// bound to in the active context.
type helpRow struct {
	key string
	op  string
}

// candidateKeys is the key space the help screen enumerates: newsboat's
// special key names, control keys in caret notation and printable
// ASCII. config.Bindings only exposes Lookup, so the help table is
// built by probing every name that could be bound. The order defines
// the table order.
var candidateKeys = buildCandidateKeys()

func buildCandidateKeys() []string {
	keys := []string{
		"ENTER", "ESC", "TAB", "UP", "DOWN", "LEFT", "RIGHT",
		"PPAGE", "NPAGE", "HOME", "END", "DEL", "INS", "BACKSPACE",
		"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
	}
	for c := 'A'; c <= 'Z'; c++ {
		keys = append(keys, "^"+string(c))
	}
	for c := ' '; c <= '~'; c++ {
		keys = append(keys, string(c))
	}
	return keys
}

// helpRows lists the effective bindings of the context the help screen
// was opened from (the "all" fallback included, as Lookup applies it).
func (m Model) helpRows() []helpRow {
	context := m.helpContext()
	var rows []helpRow
	for _, k := range candidateKeys {
		if op := m.bindings.Lookup(context, k); op != "" {
			rows = append(rows, helpRow{key: k, op: op})
		}
	}
	return rows
}

// helpContext is the binding context whose keys the help screen shows:
// the screen ? was pressed on.
func (m Model) helpContext() string {
	switch m.helpFrom {
	case screenArticleList:
		return config.CtxArticleList
	case screenHelp:
		return config.CtxHelp
	default:
		return config.CtxFeedList
	}
}

// helpView renders the help table: two columns (key, description),
// windowed around the cursor like the other lists, truncated to the
// screen width.
func (m Model) helpView() string {
	var b strings.Builder
	b.WriteString(m.sty.title.Render("charss — help"))
	b.WriteString("\n\n")

	rows := m.helpRows()
	if len(rows) == 0 {
		b.WriteString(m.sty.dim.Render("no bindings"))
		b.WriteString("\n")
	}

	start, end := windowRange(m.hcursor, len(rows), m.listRows())
	for i := start; i < end; i++ {
		key := rows[i].key
		if key == " " {
			key = "SPACE"
		}
		line := fmt.Sprintf("%-10s %s", key, opDescription(rows[i].op))
		if i == m.hcursor {
			b.WriteString(m.sty.selected.Render("▶ ") + m.sty.selected.Render(truncate(line, maxInt(1, m.width-2))))
		} else {
			b.WriteString("  " + m.sty.dim.Render(truncate(line, maxInt(1, m.width-2))))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.sty.dim.Render("q/Esc closes"))
	return b.String()
}
