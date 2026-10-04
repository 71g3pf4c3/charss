package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
)

// newsboatKeyName converts a Bubble Tea key message into the newsboat
// bind-key key-name space that config.Bindings is addressed by:
// ENTER, ESC, UP, DOWN, LEFT, RIGHT, PPAGE, NPAGE, HOME, END, TAB, ...,
// control keys as caret notation (^F, matching newsboat's bind-key
// syntax), single runes as themselves (case-sensitive: "a" and "A" are
// different keys).
func newsboatKeyName(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyEnter:
		return "ENTER"
	case tea.KeyEsc:
		return "ESC"
	case tea.KeyUp:
		return "UP"
	case tea.KeyDown:
		return "DOWN"
	case tea.KeyLeft:
		return "LEFT"
	case tea.KeyRight:
		return "RIGHT"
	case tea.KeyHome:
		return "HOME"
	case tea.KeyEnd:
		return "END"
	case tea.KeyPgUp:
		return "PPAGE"
	case tea.KeyPgDown:
		return "NPAGE"
	case tea.KeyTab:
		return "TAB"
	case tea.KeyBackspace:
		return "BACKSPACE"
	case tea.KeyDelete:
		return "DEL"
	case tea.KeyInsert:
		return "INS"
	case tea.KeySpace:
		return " "
	}
	if msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ {
		// The ctrl+a..z key codes are contiguous; newsboat writes them
		// in caret notation ("ctrl+f" -> "^F").
		return "^" + strings.ToUpper(strings.TrimPrefix(msg.String(), "ctrl+"))
	}
	if msg.Type == tea.KeyRunes && !msg.Alt {
		return string(msg.Runes)
	}
	return msg.String() // unmapped combination: nothing will be bound to it
}

// contextOf returns the newsboat binding context of the currently
// visible screen. The URL view uses the dialog context (newsboat's
// urlview equivalent).
func (m Model) contextOf() string {
	switch m.screen {
	case screenArticleList:
		return config.CtxArticleList
	case screenHelp:
		return config.CtxHelp
	case screenURLView:
		return config.CtxDialog
	default:
		return config.CtxFeedList
	}
}

// lookupOp resolves the operation bound to msg in the active screen's
// context (with the "all" fallback applied by config.Bindings).
func (m Model) lookupOp(msg tea.KeyMsg) string {
	return m.bindings.Lookup(m.contextOf(), newsboatKeyName(msg))
}

// footerHelp renders the one-line keymap footer for a context: for
// each operation (in order) the first key bound to it in context, with
// its short description. Ops with no bound key are skipped.
func (m Model) footerHelp(context string, ops ...string) string {
	var parts []string
	for _, op := range ops {
		if k := m.firstBoundKey(context, op); k != "" {
			key := k
			if key == " " {
				key = "SPACE"
			}
			parts = append(parts, key+" "+shortDesc(op))
		}
	}
	return strings.Join(parts, "  ")
}

// feedListHelp is the feed list footer.
func (m Model) feedListHelp() string {
	return m.footerHelp(config.CtxFeedList,
		config.OpUp, config.OpDown, config.OpFirst, config.OpLast,
		config.OpOpen, config.OpReload, config.OpReloadAll,
		config.OpNextUnread, config.OpSetFilter, config.OpSearch, config.OpHelp, config.OpQuit,
	)
}

// articleListHelp is the article list footer.
func (m Model) articleListHelp() string {
	return m.footerHelp(config.CtxArticleList,
		config.OpUp, config.OpDown, config.OpFirst, config.OpLast,
		config.OpOpen, config.OpToggleArticleRead, config.OpMarkFeedRead,
		config.OpSetFilter, config.OpSearch, config.OpHelp, config.OpQuit,
	)
}

// firstBoundKey returns the first candidate key bound to op in context
// ("" when none is).
func (m Model) firstBoundKey(context, op string) string {
	for _, k := range candidateKeys {
		if m.bindings.Lookup(context, k) == op {
			return k
		}
	}
	return ""
}

// shortDesc is a one-word-ish footer description (shorter than the
// help table's opDescription).
func shortDesc(op string) string {
	if d, ok := shortOpDesc[op]; ok {
		return d
	}
	return op
}

var shortOpDesc = map[string]string{
	config.OpOpen:              "open",
	config.OpQuit:              "quit",
	config.OpHardQuit:          "quit!",
	config.OpReload:            "reload",
	config.OpReloadAll:         "reload all",
	config.OpRedraw:            "redraw",
	config.OpHelp:              "help",
	config.OpUp:                "up",
	config.OpDown:              "down",
	config.OpPageUp:            "pgup",
	config.OpPageDown:          "pgdn",
	config.OpFirst:             "first",
	config.OpLast:              "last",
	config.OpNext:              "next",
	config.OpPrev:              "prev",
	config.OpNextUnread:        "next unread",
	config.OpPrevUnread:        "prev unread",
	config.OpMarkFeedRead:      "all read",
	config.OpMarkAllFeedsRead:  "all feeds read",
	config.OpToggleArticleRead: "read",
	config.OpToggleFlag:        "flag",
	config.OpSearch:            "search",
	config.OpSetFilter:         "filter",
	config.OpClearFilter:       "clear filter",
	config.OpSave:              "save",
	config.OpOpenInBrowser:     "browser",
	config.OpShowURLs:          "urls",
	config.OpEnqueue:           "enqueue",
	config.OpRunMacro:          "macro",
}

// opDescription returns a short human-readable description of an
// operation for the help screen; unimplemented operations fall back to
// the operation name itself so the table never lies about what is bound.
func opDescription(op string) string {
	if d, ok := opHelp[op]; ok {
		return d
	}
	return op
}

// opHelp describes the operations the TUI implements. Everything else
// that may be bound (enqueue, delete-article, ...) is accepted by the
// config but does nothing yet and shows as its bare op name in help.
var opHelp = map[string]string{
	config.OpOpen:                   "Open feed/article",
	config.OpQuit:                   "Return to previous dialog/quit",
	config.OpHardQuit:               "Quit program, no confirmation",
	config.OpReload:                 "Reload currently selected feed",
	config.OpReloadAll:              "Reload all feeds",
	config.OpRedraw:                 "Redraw screen",
	config.OpHelp:                   "Open help screen",
	config.OpUp:                     "Go up one item",
	config.OpDown:                   "Go down one item",
	config.OpPageUp:                 "Go up one page",
	config.OpPageDown:               "Go down one page",
	config.OpFirst:                  "Jump to first item",
	config.OpLast:                   "Jump to last item",
	config.OpNext:                   "Go to next entry",
	config.OpPrev:                   "Go to previous entry",
	config.OpNextUnread:             "Go to next unread article",
	config.OpPrevUnread:             "Go to previous unread article",
	config.OpMarkFeedRead:           "Mark feed read",
	config.OpMarkAllFeedsRead:       "Mark all feeds read",
	config.OpToggleArticleRead:      "Toggle read status for article",
	config.OpToggleShowReadArticles: "Toggle showing read articles",
	config.OpSearch:                 "Search articles",
	config.OpSetFilter:              "Set a filter",
	config.OpClearFilter:            "Clear currently set filter",
	config.OpSave:                   "Save article to a file",
	config.OpOpenInBrowser:          "Open URL in browser",
	config.OpShowURLs:               "Show URLs in article",
	config.OpToggleFlag:             "Toggle a flag on the article",
	config.OpEnqueue:                "Add enclosure to podcast queue",
	config.OpRunMacro:               "Run a macro",
}
