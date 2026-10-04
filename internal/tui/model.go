// Package tui implements the charss terminal UI on Bubble Tea / Lip Gloss.
//
// The UI is a navigation/selection layer only: the feed list and article
// list live here, articles are displayed by the external chawan browser
// (via the handoff in browser.go) and images by chafa. Newsboat is the
// UX reference: keys are dispatched through the config's binding
// contexts, not through a hardcoded keymap.
package tui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/filter"
	"github.com/71g3pf4c3/charss/internal/search"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// screen is the currently visible list.
type screen int

const (
	screenFeedList screen = iota
	screenArticleList
	screenHelp
	screenURLView
)

// inputKind is what the single-line status-position input is collecting.
type inputKind int

const (
	inputNone inputKind = iota
	inputFilter
	inputSearch
	inputFlag     // one flag character for the article under the cursor
	inputSavePath // article save destination path
)

// Fallback terminal size used when the pty reports a degenerate 0x0
// size (ncurses convention: 80x24 when the size cannot be determined).
const (
	fallbackWidth  = 80
	fallbackHeight = 24
)

// Model is the full reader state. It handles all screens; the article
// list is a mode over the same data, not a separate program.
type Model struct {
	bindings config.Bindings // resolved key bindings (config.Bindings is a value copy)
	feeds    []urls.Feed     // urls-file order; the feed list shows this
	states   map[string]*feedState
	macros   map[string]config.Macro // `macro` definitions ("" = none)

	// Injected dependencies; nil entries disable the related features
	// (see the per-key guards) instead of crashing.
	browser  Browser
	fetcher  Fetcher
	store    Storer
	term     Terminal
	enqueuer Enqueuer

	// View state.
	screen   screen
	cursor   int    // feed list cursor (over the visible, filtered rows)
	acursor  int    // article list cursor (over the visible, filtered refs)
	active   string // feed URL (or query-feed URL) whose articles are shown
	helpFrom screen // screen ? was pressed on

	// URL view (show-urls) state: the snapshot is taken when the screen
	// opens, so later refreshes cannot shift entries under the cursor.
	urlEntries []string
	urlFrom    screen
	urlTitle   string // the article's title, for the view header
	ucursor    int

	// Prompt targets. Opening a prompt snapshots the article it applies
	// to: a refresh landing while the prompt is open must not retarget it.
	flagTarget articleRef
	saveTarget articleRef

	// Macro state. macroPending waits for the key selecting the macro;
	// macroRunning guards the synchronous execution loop against
	// re-entering run-macro from inside a macro.
	macroPending bool
	macroRunning bool

	// Feed list session state. Filters are per-screen session state and
	// are never persisted (newsboat behavior).
	feedFilter *filter.Filter

	// Article list session state.
	artFilter *filter.Filter

	// showRead mirrors newsboat's show-read-articles option (default
	// yes); the toggle-show-read-articles operation flips it.
	showRead bool

	// Search results (virtual article list).
	searchQuery   string
	searchResults []search.Result
	searchOn      bool

	// Single-line input at the status position (filter / search).
	input     textinput.Model
	inputKind inputKind

	hcursor int // help screen cursor

	sty styles // resolved color styles (defaults unless color rules exist)

	feedSortOrder feedSortOrder
	articleSort   articleSortOrder

	// Auto-reload and notifications. Parsed once at New from the config;
	// changing auto-reload/reload-time requires a restart, exactly like
	// newsboat does not re-arm a running timer mid-session.
	autoReload   bool
	reloadEvery  time.Duration // 0 disables the tick loop
	notifyScreen bool
	batchNew     int // new articles seen in the in-flight refresh batch

	opening    bool   // an external browser owns the terminal (handoff)
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
	// Enqueue is the persistent podcast download queue; nil disables
	// the enqueue operation with a status message instead of a crash.
	Enqueue Enqueuer
	// Config is the resolved configuration (bindings, colors,
	// options). nil disables config-driven behavior: no key is bound
	// and built-in defaults apply to everything else. cmd/tui.go always
	// passes the loaded config.
	Config *config.Config
	// Warning is shown once in the status line at startup (e.g. a missing
	// urls file, which is not an error).
	Warning string
}

// New returns the initial model.
func New(opts Options) Model {
	input := textinput.New()
	input.Prompt = ": "
	m := Model{
		feeds:    opts.Feeds,
		states:   make(map[string]*feedState, len(opts.Feeds)),
		browser:  opts.Browser,
		fetcher:  opts.Fetcher,
		store:    opts.Store,
		term:     opts.Terminal,
		enqueuer: opts.Enqueue,
		status:   opts.Warning,
		showRead: true,
		input:    input,
		sty:      defaultStyles(),
	}
	if opts.Config != nil {
		m.bindings = opts.Config.Bindings
		m.sty = applyColorRules(opts.Config.Colors)
		m.feedSortOrder = parseFeedSortOrder(opts.Config.Options["feed-sort-order"])
		m.articleSort = parseArticleSortOrder(opts.Config.Options["article-sort-order"])
		if opts.Config.Options["show-read-articles"] == "no" {
			m.showRead = false
		}
		m.parseRuntimeOptions(opts.Config.Options)
		m.macros = opts.Config.Macros
	}
	for _, f := range opts.Feeds {
		m.states[f.URL] = nil // lazily filled by loadStatesCmd / refreshes
	}
	return m
}

// parseRuntimeOptions resolves the auto-reload and notification options.
// reload-time is minutes with a floor of 1 (newsboat's documented
// minimum); an unparsable value falls back to the 60-minute default
// rather than silently disabling auto-reload.
func (m *Model) parseRuntimeOptions(opts map[string]string) {
	if opts["auto-reload"] == "yes" {
		mins := 60
		if v, err := strconv.Atoi(opts["reload-time"]); err == nil && v >= 1 {
			mins = v
		}
		if mins < 1 {
			mins = 1
		}
		m.autoReload = true
		m.reloadEvery = time.Duration(mins) * time.Minute
	}
	m.notifyScreen = opts["notify-screen"] == "yes"
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
		if msg.Width == 0 || msg.Height == 0 {
			// Degenerate size. Delivered by pty environments without
			// a size (e.g. `script` with no controlling terminal) and
			// re-delivered by tea's RestoreTerminal -> checkResize
			// right after the chawan handoff. Never let it clobber a
			// known-good size; if no size is known yet, fall back to
			// 80x24 so the UI renders at a sensible width instead of
			// truncating everything to "…" (the width=0 handoff bug).
			if !m.ready {
				m.width, m.height = fallbackWidth, fallbackHeight
				m.ready = true
			}
			return m, nil
		}
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
		// Background auto-refresh after the cached data is on screen,
		// and the periodic auto-reload loop after that (newsboat's
		// auto-reload keeps reloading every reload-time minutes).
		m2, cmd := m.startRefresh(realFeeds(m.feeds))
		if m.autoReload {
			cmd = tea.Batch(cmd, autoReloadCmd(m.reloadEvery))
		}
		return m2, cmd

	case autoReloadTickMsg:
		// The timer is only re-armed while auto-reload stays on; a
		// stale tick after a (hypothetical) runtime switch dies here.
		if !m.autoReload {
			return m, nil
		}
		m2, cmd := m.startRefresh(realFeeds(m.feeds))
		return m2, tea.Batch(cmd, autoReloadCmd(m.reloadEvery))

	case refreshedMsg:
		return m.applyRefreshed(msg)

	case stateSavedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("save: %v", msg.err)
		}
		return m, nil

	case articleSavedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("save: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("saved to %s", msg.path)
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
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.opening {
			// The external browser owns the terminal; swallow anything
			// queued.
			return m, nil
		}
		if m.macroPending {
			return m.selectMacro(msg)
		}
		if m.inputKind != inputNone {
			return m.updateInput(msg)
		}
		// run-macro is intercepted globally: newsboat binds its macro
		// prefix "," in every context, and the next key must select a
		// macro whatever screen is active.
		if m.lookupOp(msg) == config.OpRunMacro {
			if m.macroRunning {
				m.status = "macro: cannot nest run-macro"
				return m, nil
			}
			if len(m.macros) == 0 {
				m.status = "no macros defined"
				return m, nil
			}
			m.macroPending = true
			return m, nil
		}
		switch m.screen {
		case screenFeedList:
			return m.updateFeedList(msg)
		case screenArticleList:
			return m.updateArticleList(msg)
		case screenHelp:
			return m.updateHelp(msg)
		case screenURLView:
			return m.updateURLView(msg)
		}
	}
	return m, nil
}

// dispatchOp runs one operation in the currently active screen's handler.
// Macro execution drives every step through here, so a macro op that
// changes screens mid-sequence has its successors resolve in the NEW
// context — newsboat macro semantics.
func (m Model) dispatchOp(op, arg string) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenArticleList:
		return m.articleListOp(op, arg)
	case screenHelp:
		return m.helpOp(op, arg)
	case screenURLView:
		return m.urlViewOp(op, arg)
	default:
		return m.feedListOp(op, arg)
	}
}

// selectMacro resolves the key pressed after the macro prefix: the macro
// it names runs immediately, Esc cancels, an unknown key reports a status
// error.
func (m Model) selectMacro(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.macroPending = false
	if msg.Type == tea.KeyEsc {
		return m, nil
	}
	key := newsboatKeyName(msg)
	mac, ok := m.macros[key]
	if !ok {
		m.status = fmt.Sprintf("macro %q: not defined", key)
		return m, nil
	}
	return m.runMacro(mac.Ops)
}

// runMacro executes a macro's operations sequentially through the same
// op dispatch as key handling. Ops carrying an argument (save path, flag
// char) bypass their prompts; everything else behaves exactly as if the
// bound key had been pressed.
func (m Model) runMacro(ops []config.MacroOp) (tea.Model, tea.Cmd) {
	m.macroRunning = true
	var cmds []tea.Cmd
	for _, mo := range ops {
		if mo.Op == config.OpRunMacro {
			continue // no nested macro execution
		}
		var cmd tea.Cmd
		var tm tea.Model
		tm, cmd = m.dispatchOp(mo.Op, mo.Arg)
		m = tm.(Model)
		cmds = append(cmds, cmd)
	}
	m.macroRunning = false
	return m, tea.Batch(cmds...)
}

// updateFeedList dispatches one key on the feed list through the
// resolved bindings (feedlist context, "all" fallback).
func (m Model) updateFeedList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.feedListOp(m.lookupOp(msg), "")
}

// feedListOp applies one operation (plus an optional macro argument) to
// the feed list.
func (m Model) feedListOp(op, arg string) (tea.Model, tea.Cmd) {
	rows := m.feedRows()
	switch op {
	case config.OpOpen:
		if len(rows) == 0 {
			m.status = "no feeds — add URLs to your urls file"
			return m, nil
		}
		row := rows[m.cursor]
		if row.queryErr != nil {
			m.status = fmt.Sprintf("query feed: %v", row.queryErr)
			return m, nil
		}
		m.screen = screenArticleList
		m.active = row.feed.URL
		m.acursor = 0
		m.searchOn = false
		m.status = ""

	case config.OpQuit, config.OpHardQuit:
		return m, tea.Quit

	case config.OpUp, config.OpPrev:
		if m.cursor > 0 {
			m.cursor--
		}
		m.status = ""

	case config.OpDown, config.OpNext:
		if m.cursor < len(rows)-1 {
			m.cursor++
		}
		m.status = ""

	case config.OpFirst:
		m.cursor = 0

	case config.OpLast:
		if len(rows) > 0 {
			m.cursor = len(rows) - 1
		}

	case config.OpPageUp:
		m.cursor = clampInt(m.cursor-m.listRows(), 0, len(rows)-1)

	case config.OpPageDown:
		m.cursor = clampInt(m.cursor+m.listRows(), 0, len(rows)-1)

	case config.OpRedraw:
		return m, tea.ClearScreen

	case config.OpReload:
		if len(rows) == 0 {
			m.status = "no feeds"
			return m, nil
		}
		if rows[m.cursor].query != nil || rows[m.cursor].queryErr != nil {
			m.status = "cannot reload a query feed"
			return m, nil
		}
		return m.startRefresh([]urls.Feed{rows[m.cursor].feed})

	case config.OpReloadAll:
		return m.startRefresh(realFeeds(m.feeds))

	case config.OpMarkFeedRead:
		if len(rows) == 0 {
			return m, nil
		}
		row := rows[m.cursor]
		if row.query != nil {
			return m.markRead(m.queryArticles(row.query))
		}
		return m.markRead(m.feedArticles(row.feed.URL))

	case config.OpMarkAllFeedsRead:
		var refs []articleRef
		for _, f := range realFeeds(m.feeds) {
			refs = append(refs, m.feedArticles(f.URL)...)
		}
		return m.markRead(refs)

	case config.OpNextUnread:
		return m.jumpFeedUnread(rows, +1)

	case config.OpPrevUnread:
		return m.jumpFeedUnread(rows, -1)

	case config.OpSearch:
		return m.openInput(inputSearch)

	case config.OpSetFilter:
		return m.openInput(inputFilter)

	case config.OpClearFilter:
		m.feedFilter = nil
		m.cursor = 0
		m.status = "filter cleared"

	case config.OpHelp:
		m.helpFrom = m.screen
		m.screen = screenHelp
		m.status = ""

	case config.OpOpenInBrowser:
		if len(rows) == 0 {
			return m, nil
		}
		row := rows[m.cursor]
		if row.query != nil || row.queryErr != nil {
			m.status = "cannot open a query feed in the browser"
			return m, nil
		}
		if row.feed.URL == "" {
			m.status = "feed has no URL"
			return m, nil
		}
		if m.browser == nil || m.term == nil {
			m.status = errNoBrowser.Error()
			return m, nil
		}
		m.opening = true
		return m, openURLCmd(m.term, m.browser, row.feed.URL)

	case config.OpSave, config.OpShowURLs, config.OpToggleFlag, config.OpEnqueue:
		m.status = fmt.Sprintf("%s: only on the article list", op)

	default:
		// Ops that only make sense on the article list (or are not
		// implemented at all) are silently unbound here, like an
		// unbound key.
	}
	return m, nil
}

// updateArticleList dispatches one key on the article list through the
// resolved bindings (articlelist context, "all" fallback).
func (m Model) updateArticleList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.articleListOp(m.lookupOp(msg), "")
}

// articleListOp applies one operation (plus an optional macro argument)
// to the article list.
func (m Model) articleListOp(op, arg string) (tea.Model, tea.Cmd) {
	refs := m.articleRefs()

	switch op {
	case config.OpOpen:
		if len(refs) == 0 {
			m.status = "no articles — press q and r to reload this feed"
			return m, nil
		}
		if m.browser == nil || m.term == nil {
			m.status = errNoBrowser.Error()
			return m, nil
		}
		r := refs[m.acursor]
		st := m.stateFor(r.feedURL)
		st.read[r.art.ID] = true // newsboat marks articles read on open
		m.opening = true
		return m, tea.Batch(
			saveStateCmd(m.store, r.feedURL, st.toStore()),
			openArticleCmd(m.term, m.browser, articleHTML(r.art, r.feedTitle)),
		)

	case config.OpQuit:
		// Contextual quit: return to the feed list (newsboat's quit).
		m.screen = screenFeedList
		m.active = ""
		m.acursor = 0
		m.searchOn = false
		m.status = ""

	case config.OpHardQuit:
		return m, tea.Quit

	case config.OpUp, config.OpPrev:
		if m.acursor > 0 {
			m.acursor--
		}

	case config.OpDown, config.OpNext:
		if m.acursor < len(refs)-1 {
			m.acursor++
		}

	case config.OpFirst:
		m.acursor = 0

	case config.OpLast:
		if len(refs) > 0 {
			m.acursor = len(refs) - 1
		}

	case config.OpPageUp:
		m.acursor = clampInt(m.acursor-m.listRows(), 0, len(refs)-1)

	case config.OpPageDown:
		m.acursor = clampInt(m.acursor+m.listRows(), 0, len(refs)-1)

	case config.OpRedraw:
		return m, tea.ClearScreen

	case config.OpReload:
		if _, ok := parseQueryFeed(m.activeFeed()); ok {
			m.status = "cannot reload a query feed"
			return m, nil
		}
		if m.searchOn || m.active == "" {
			m.status = "cannot reload search results"
			return m, nil
		}
		return m.startRefresh([]urls.Feed{m.activeFeed()})

	case config.OpMarkFeedRead:
		return m.markRead(refs)

	case config.OpToggleArticleRead:
		if len(refs) == 0 {
			return m, nil
		}
		r := refs[m.acursor]
		st := m.stateFor(r.feedURL)
		if st.read[r.art.ID] {
			delete(st.read, r.art.ID)
		} else {
			st.read[r.art.ID] = true
		}
		return m, saveStateCmd(m.store, r.feedURL, st.toStore())

	case config.OpNextUnread:
		if u := m.findUnread(refs, +1); u >= 0 {
			m.acursor = u
		} else {
			m.status = "no unread articles"
		}

	case config.OpPrevUnread:
		if u := m.findUnread(refs, -1); u >= 0 {
			m.acursor = u
		} else {
			m.status = "no unread articles"
		}

	case config.OpToggleShowReadArticles:
		m.showRead = !m.showRead
		m.acursor = clampInt(m.acursor, 0, len(refs)-1)

	case config.OpToggleFlag:
		if len(refs) == 0 {
			return m, nil
		}
		if arg != "" {
			// Macro path: the argument is the flag character itself.
			return m.applyFlag(refs[m.acursor], arg)
		}
		return m.openFlagPrompt(refs[m.acursor])

	case config.OpSave:
		if len(refs) == 0 {
			return m, nil
		}
		if arg != "" {
			// Macro path: the argument is the destination path.
			return m.saveArticle(refs[m.acursor], arg)
		}
		return m.openSavePrompt(refs[m.acursor])

	case config.OpOpenInBrowser:
		if len(refs) == 0 {
			return m, nil
		}
		if refs[m.acursor].art.URL == "" {
			m.status = "article has no URL"
			return m, nil
		}
		if m.browser == nil || m.term == nil {
			m.status = errNoBrowser.Error()
			return m, nil
		}
		m.opening = true
		return m, openURLCmd(m.term, m.browser, refs[m.acursor].art.URL)

	case config.OpShowURLs:
		if len(refs) == 0 {
			return m, nil
		}
		m.urlEntries = extractURLs(refs[m.acursor].art)
		m.urlFrom = m.screen
		m.urlTitle = refs[m.acursor].art.Title
		m.ucursor = 0
		m.screen = screenURLView
		m.status = ""

	case config.OpEnqueue:
		if len(refs) == 0 {
			return m, nil
		}
		return m.enqueueEnclosure(refs[m.acursor])

	case config.OpSearch:
		return m.openInput(inputSearch)

	case config.OpSetFilter:
		return m.openInput(inputFilter)

	case config.OpClearFilter:
		m.artFilter = nil
		m.acursor = 0
		m.status = "filter cleared"

	case config.OpHelp:
		m.helpFrom = m.screen
		m.screen = screenHelp
		m.status = ""
	}
	return m, nil
}

// updateHelp dispatches one key on the help screen (help context).
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.helpOp(m.lookupOp(msg), "")
}

// helpOp applies one operation to the help screen.
func (m Model) helpOp(op, _ string) (tea.Model, tea.Cmd) {
	rows := m.helpRows()
	switch op {
	case config.OpQuit:
		m.screen = m.helpFrom
		m.status = ""

	case config.OpUp, config.OpPrev:
		if m.hcursor > 0 {
			m.hcursor--
		}

	case config.OpDown, config.OpNext:
		if m.hcursor < len(rows)-1 {
			m.hcursor++
		}

	case config.OpFirst:
		m.hcursor = 0

	case config.OpLast:
		if len(rows) > 0 {
			m.hcursor = len(rows) - 1
		}

	case config.OpPageUp:
		m.hcursor = clampInt(m.hcursor-m.listRows(), 0, len(rows)-1)

	case config.OpPageDown:
		m.hcursor = clampInt(m.hcursor+m.listRows(), 0, len(rows)-1)
	}
	return m, nil
}

// findUnread returns the index of the next (dir=+1) or previous (dir=-1)
// unread article in refs, starting after the cursor, without wrapping;
// -1 when there is none.
func (m Model) findUnread(refs []articleRef, dir int) int {
	return m.firstUnreadFrom(refs, m.acursor+dir, dir)
}

// firstUnreadFrom scans refs from start in direction dir and returns the
// index of the first unread article, or -1 when there is none.
func (m Model) firstUnreadFrom(refs []articleRef, start, dir int) int {
	for i := start; i >= 0 && i < len(refs); i += dir {
		if !m.readOf(refs[i].feedURL, refs[i].art.ID) {
			return i
		}
	}
	return -1
}

// jumpFeedUnread implements next-unread/prev-unread on the feed list:
// move to the next feed (wrapping once) that has unread articles, open
// its article list at the first unread article (newsboat's n jumps to the
// next unread article, not just the next feed).
func (m Model) jumpFeedUnread(rows []feedRow, dir int) (tea.Model, tea.Cmd) {
	if len(rows) == 0 {
		m.status = "no unread articles"
		return m, nil
	}
	for step := 1; step <= len(rows); step++ {
		i := m.cursor + dir*step
		for i < 0 {
			i += len(rows)
		}
		i %= len(rows)
		if rows[i].unread > 0 && rows[i].queryErr == nil {
			m.cursor = i
			m.screen = screenArticleList
			m.active = rows[i].feed.URL
			m.searchOn = false
			if u := m.firstUnreadFrom(m.articleRefs(), 0, +1); u >= 0 {
				m.acursor = u
			} else {
				m.acursor = 0
			}
			m.status = ""
			return m, nil
		}
	}
	m.status = "no unread articles"
	return m, nil
}

// markRead marks the given articles read in their owning feeds' states
// and returns a batched save command.
func (m Model) markRead(refs []articleRef) (tea.Model, tea.Cmd) {
	if len(refs) == 0 {
		return m, nil
	}
	marked := 0
	touched := make(map[string]bool)
	for _, r := range refs {
		st := m.stateFor(r.feedURL)
		if !st.read[r.art.ID] {
			st.read[r.art.ID] = true
			marked++
		}
		touched[r.feedURL] = true
	}
	m.status = fmt.Sprintf("marked %d articles read", marked)
	cmds := make([]tea.Cmd, 0, len(touched))
	for url := range touched {
		cmds = append(cmds, saveStateCmd(m.store, url, m.states[url].toStore()))
	}
	return m, tea.Batch(cmds...)
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
	if m.refreshing == len(feeds) {
		m.batchNew = 0 // a fresh batch starts here
	}
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

// feedTitle resolves the display title for a feed URL.
func (m Model) feedTitle(feedURL string) string {
	for _, f := range m.feeds {
		if f.URL == feedURL {
			return feedTitle(f)
		}
	}
	return feedURL
}

// activeTitle is the article list header title: the feed title, the
// query feed's name, or the search query.
func (m Model) activeTitle() string {
	if m.searchOn {
		return fmt.Sprintf("Search: %q", m.searchQuery)
	}
	if q, ok := parseQueryFeed(m.activeFeed()); ok && q.name != "" {
		return q.name
	}
	return m.feedTitle(m.active)
}

// unreadTotal sums the unread counts across all real feeds.
func (m Model) unreadTotal() int {
	n := 0
	for _, f := range realFeeds(m.feeds) {
		n += m.states[f.URL].unread() // nil-safe receiver
	}
	return n
}

// clampInt clamps v into [lo, hi]; a negative hi yields lo (empty list).
func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
