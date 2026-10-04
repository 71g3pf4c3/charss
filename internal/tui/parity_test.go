package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// ---- key name mapping ----------------------------------------------------

func TestNewsboatKeyName(t *testing.T) {
	cases := map[string]string{
		"enter": "ENTER", "esc": "ESC", "up": "UP", "down": "DOWN",
		"left": "LEFT", "right": "RIGHT", "home": "HOME", "end": "END",
		"ctrl+c": "^C", "ctrl+f": "^F", "ctrl+l": "^L",
		"j": "j", "G": "G", "?": "?", "/": "/",
	}
	for in, want := range cases {
		if got := newsboatKeyName(keyMsg(in)); got != want {
			t.Errorf("newsboatKeyName(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- binding-driven dispatch ----------------------------------------------

func TestBindingDrivenDispatch(t *testing.T) {
	feeds := []urls.Feed{{URL: "a", Title: "A"}, {URL: "b", Title: "B"}}
	cfg := testConfig(t,
		"bind-key x open feedlist",
		"unbind-key q feedlist",
		"bind-key X toggle-article-read articlelist",
	)
	m := newTUI(t, Options{Feeds: feeds, Config: cfg})

	// Custom binding drives open.
	m, _ = press(m, "x")
	if m.screen != screenArticleList || m.active != "a" {
		t.Fatalf("custom open binding: screen=%v active=%q", m.screen, m.active)
	}

	// q is still bound in articlelist (contextual back).
	m, _ = press(m, "q")
	if m.screen != screenFeedList {
		t.Fatal("q in articlelist should go back")
	}

	// q was explicitly unbound in feedlist: it must not quit.
	m, cmd := press(m, "q")
	if cmd != nil {
		t.Fatal("unbound q must produce no command")
	}

	// Custom articlelist binding replaces the default toggle key. The
	// list is newest first, so the cursor sits on art "2".
	setState(&m, "a", []feed.Article{art("1", t1, "one"), art("2", t2, "two")}, map[string]bool{})
	m, _ = press(m, "x")
	m, _ = press(m, "X")
	if !m.states["a"].read["2"] || m.states["a"].read["1"] {
		t.Fatalf("custom toggle binding should toggle the article under the cursor: %v", m.states["a"].read)
	}
}

// ---- filter mode -----------------------------------------------------------

func typeInput(m Model, text string) Model {
	m, _ = press(m, text) // one KeyRunes message; the textinput inserts it
	return m
}

func TestFilterArticleList(t *testing.T) {
	st := newFakeStore()
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}, Store: st})
	m = sized(m)
	setState(&m, "a", []feed.Article{
		{ID: "1", Title: "golang release", Published: t1},
		{ID: "2", Title: "python release", Published: t2},
	}, map[string]bool{})
	m, _ = press(m, "enter")

	// set-filter prompt, valid expression.
	m, _ = press(m, "F")
	if m.inputKind != inputFilter {
		t.Fatalf("inputKind = %v, want inputFilter", m.inputKind)
	}
	m = typeInput(m, `title # "golang"`)
	m, _ = press(m, "enter")
	if m.inputKind != inputNone {
		t.Fatal("input should close after submit")
	}
	if m.artFilter == nil {
		t.Fatal("valid expression should install the article filter")
	}
	refs := m.articleRefs()
	if len(refs) != 1 || refs[0].art.ID != "1" {
		t.Fatalf("filtered refs = %v, want only article 1", idsOfRefs(refs))
	}
	v := m.View()
	if !contains(v, "(filtered 1/2)") {
		t.Errorf("header should show (filtered N/M):\n%s", v)
	}

	// Invalid expression: status error, previous filter kept.
	m, _ = press(m, "F")
	m = typeInput(m, `title #`)
	m, _ = press(m, "enter")
	if m.status == "" {
		t.Error("invalid expression should report a status error")
	}
	if m.artFilter == nil {
		t.Error("invalid expression must keep the previous filter")
	}

	// clear-filter resets.
	m, _ = press(m, "ctrl+f")
	if m.artFilter != nil {
		t.Error("clear-filter should reset the article filter")
	}
	if got := len(m.articleRefs()); got != 2 {
		t.Errorf("after clear: %d refs, want 2", got)
	}

	// Empty input clears too.
	m, _ = press(m, "F")
	m, _ = press(m, "enter")
	if m.artFilter != nil {
		t.Error("empty filter input should clear the filter")
	}
}

func TestFilterFeedList(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{
		{URL: "https://a", Title: "Alpha", Tags: []string{"tech"}},
		{URL: "https://b", Title: "Beta"},
	}})
	setState(&m, "https://a", []feed.Article{art("1", t1, "")}, map[string]bool{})

	// Filter by feed title.
	m, _ = press(m, "F")
	m = typeInput(m, `title # "alp"`)
	m, _ = press(m, "enter")
	rows := m.feedRows()
	if len(rows) != 1 || rows[0].feed.URL != "https://a" {
		t.Fatalf("filtered rows = %v, want only feed a", rows)
	}

	// Filter by tag.
	m, _ = press(m, "F")
	m = typeInput(m, `tags # "tech"`)
	m, _ = press(m, "enter")
	if got := len(m.feedRows()); got != 1 {
		t.Errorf("tag filter: %d rows, want 1", got)
	}

	// Filter that matches nothing.
	m, _ = press(m, "F")
	m = typeInput(m, `title # "zzz"`)
	m, _ = press(m, "enter")
	if got := len(m.feedRows()); got != 0 {
		t.Errorf("non-matching filter: %d rows, want 0", got)
	}

	// clear-filter restores the full list.
	m, _ = press(m, "ctrl+f")
	if got := len(m.feedRows()); got != 2 {
		t.Errorf("after clear: %d rows, want 2", got)
	}
}

// ---- search -----------------------------------------------------------------

func TestSearchFlow(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{
		{URL: "a", Title: "Feed A"},
		{URL: "b", Title: "Feed B"},
	}})
	m = sized(m)
	setState(&m, "a", []feed.Article{
		{ID: "1", Title: "golang release", ContentHTML: "<p>hello</p>", Published: t1},
		{ID: "2", Title: "python release", ContentHTML: "<p>world</p>", Published: t2},
	}, map[string]bool{})
	setState(&m, "b", []feed.Article{
		{ID: "3", Title: "golang weekly", ContentHTML: "<p>hi</p>", Published: t0},
	}, map[string]bool{})

	// "/" opens the search prompt; the query runs over ALL feeds.
	m, _ = press(m, "/")
	if m.inputKind != inputSearch {
		t.Fatalf("inputKind = %v, want inputSearch", m.inputKind)
	}
	m = typeInput(m, "golang")
	m, _ = press(m, "enter")
	if !m.searchOn {
		t.Fatal("search should activate the virtual results list")
	}
	if m.screen != screenArticleList {
		t.Fatalf("screen = %v, want article list", m.screen)
	}
	refs := m.articleRefs()
	if len(refs) != 2 {
		t.Fatalf("results = %v, want articles 1 and 3", idsOfRefs(refs))
	}
	// Newest first.
	if refs[0].art.ID != "1" {
		t.Errorf("first result = %s, want 1 (newest first)", refs[0].art.ID)
	}
	// Hit ranges are stored on the state for later highlighting.
	if len(m.searchResults[0].TitleHits) == 0 {
		t.Error("title hit ranges should be stored on the state")
	}
	v := m.View()
	if !contains(v, `Search: "golang"`) {
		t.Errorf("header should show the search title:\n%s", v)
	}

	// quit returns to the feed list and leaves the search mode.
	m, _ = press(m, "q")
	if m.screen != screenFeedList || m.searchOn {
		t.Fatal("q should leave the search results")
	}

	// A bad regex reports a status error and changes nothing.
	m, _ = press(m, "/")
	m = typeInput(m, "/[[/")
	m, _ = press(m, "enter")
	if m.status == "" {
		t.Error("invalid regex should report a status error")
	}
	if m.searchOn {
		t.Error("failed search must not open the results list")
	}
}

// ---- query feeds --------------------------------------------------------------

func TestQueryFeeds(t *testing.T) {
	feeds := []urls.Feed{
		{URL: `query:Go:title # "golang"`, Title: "ignored"},
		{URL: "query:Broken:title #"},
		{URL: "a", Title: "Feed A"},
		{URL: "b", Title: "Feed B"},
	}
	st := newFakeStore()
	m := newTUI(t, Options{Feeds: feeds, Store: st})
	setState(&m, "a", []feed.Article{
		{ID: "1", Title: "golang release", Published: t1},
		{ID: "2", Title: "python release", Published: t0},
	}, map[string]bool{})
	setState(&m, "b", []feed.Article{
		{ID: "3", Title: "golang weekly", Published: t2},
		{ID: "4", Title: "golang read already", Published: t0},
	}, map[string]bool{"4": true})

	rows := m.feedRows()
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (query feeds are listed too)", len(rows))
	}
	if rows[0].title != "Go" {
		t.Errorf("query feed title = %q, want the declared name", rows[0].title)
	}
	// Unread = matching unread articles across all feeds: 1 and 3.
	if rows[0].unread != 2 {
		t.Errorf("query unread = %d, want 2", rows[0].unread)
	}
	// The broken query feed is listed with an error marker.
	if rows[1].queryErr == nil {
		t.Error("broken expression should be listed with a compile error")
	}

	// Enter opens the query feed's virtual article list: all matching
	// articles across feeds, newest first (read ones included).
	m, _ = press(m, "enter")
	if m.screen != screenArticleList || m.active != `query:Go:title # "golang"` {
		t.Fatalf("screen=%v active=%q, want the query feed open", m.screen, m.active)
	}
	refs := m.articleRefs()
	if ids := idsOfRefs(refs); len(ids) != 3 || ids[0] != "3" || ids[1] != "1" {
		t.Fatalf("query refs = %v, want [3 1 4] (newest first)", ids)
	}

	// mark-feed-read on the query feed marks the underlying feeds.
	m, cmd := press(m, "A")
	_ = runCmd(cmd)
	if !m.states["a"].read["1"] || m.states["a"].read["2"] || !m.states["b"].read["3"] {
		t.Errorf("mark-feed-read should mark the query's matches (1 and 3, not 2): a=%v b=%v",
			m.states["a"].read, m.states["b"].read)
	}
	saved := map[string]bool{}
	for _, u := range st.saves {
		saved[u] = true
	}
	if !saved["a"] || !saved["b"] {
		t.Errorf("saves = %v, want both underlying feeds saved", st.saves)
	}

	// The broken query feed refuses to open with a status error.
	m, _ = press(m, "q")
	m, _ = press(m, "j") // cursor onto the broken query feed
	m, _ = press(m, "enter")
	if m.screen != screenFeedList || m.status == "" {
		t.Fatal("broken query feed should not open and should explain why")
	}

	// Query feeds are never refreshed: reload-all only touches real feeds.
	var fetched []string
	m.fetcher = fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		fetched = append(fetched, f.URL)
		return feed.Fetched{Feed: f, Articles: []feed.Article{art("n", t1, "")}}, nil
	})
	m, cmd = press(m, "R")
	_ = runCmd(cmd)
	for _, u := range fetched {
		if u == `query:Go:title # "golang"` {
			t.Errorf("query feed was fetched: %v", fetched)
		}
	}
}

// ---- sort orders ----------------------------------------------------------------

func TestSortOrders(t *testing.T) {
	cfg := testConfig(t, "feed-sort-order title", "article-sort-order date")
	m := newTUI(t, Options{
		Feeds: []urls.Feed{
			{URL: "https://b", Title: "Beta"},
			{URL: "https://a", Title: "Alpha"},
		},
		Config: cfg,
	})
	setState(&m, "https://a", []feed.Article{
		{ID: "new", Title: "zzz", Published: t2},
		{ID: "old", Title: "aaa", Published: t0},
	}, map[string]bool{})

	rows := m.feedRows()
	if rows[0].feed.URL != "https://a" {
		t.Errorf("feed sort by title: first = %s, want Alpha", rows[0].feed.URL)
	}

	m, _ = press(m, "enter")
	refs := m.articleRefs()
	if ids := idsOfRefs(refs); ids[0] != "old" {
		t.Errorf("article sort by date (asc): %v, want old first", ids)
	}

	// Default (no options): urls order, newest first.
	m2 := newTUI(t, Options{Feeds: []urls.Feed{{URL: "https://b", Title: "Beta"}, {URL: "https://a", Title: "Alpha"}}})
	if m2.feedSortOrder != feedSortNone || m2.articleSort != articleSortDateDesc {
		t.Errorf("default sort orders = %v/%v, want none/date-desc", m2.feedSortOrder, m2.articleSort)
	}

	// unreadcount sorts feeds by unread articles, most first.
	cfg3 := testConfig(t, "feed-sort-order unreadcount")
	m3 := newTUI(t, Options{
		Feeds:  []urls.Feed{{URL: "x", Title: "X"}, {URL: "y", Title: "Y"}},
		Config: cfg3,
	})
	setState(&m3, "x", []feed.Article{art("1", t1, "")}, map[string]bool{})
	setState(&m3, "y", []feed.Article{art("2", t1, ""), art("3", t1, "")}, map[string]bool{})
	rows3 := m3.feedRows()
	if rows3[0].feed.URL != "y" {
		t.Errorf("unreadcount sort: first = %s, want y (2 unread)", rows3[0].feed.URL)
	}
}

// ---- help screen ------------------------------------------------------------------

func TestHelpScreen(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	m = sized(m)

	m, _ = press(m, "?")
	if m.screen != screenHelp || m.helpFrom != screenFeedList {
		t.Fatalf("screen=%v helpFrom=%v, want help over the feed list", m.screen, m.helpFrom)
	}
	v := m.View()
	if !contains(v, "ENTER") || !contains(v, "Open feed/article") {
		t.Errorf("help should list the feedlist bindings:\n%s", v)
	}
	// q closes back to the screen it was opened from.
	m, _ = press(m, "q")
	if m.screen != screenFeedList {
		t.Fatal("q should close the help screen")
	}

	// From the article list the shown context changes.
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")
	m, _ = press(m, "?")
	rows := m.helpRows()
	found := false
	for _, r := range rows {
		if r.op == "toggle-article-read" {
			found = true
		}
	}
	if !found {
		t.Errorf("help should list the articlelist bindings: %v", rows)
	}
	m, _ = press(m, "esc") // ESC closes help too
	if m.screen != screenArticleList {
		t.Fatal("esc should close the help screen")
	}
}

// ---- show-read-articles toggle ------------------------------------------------------

func TestToggleShowReadArticles(t *testing.T) {
	cfg := testConfig(t, "bind-key t toggle-show-read-articles articlelist")
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Config: cfg})
	setState(&m, "a", []feed.Article{art("1", t1, "one"), art("2", t2, "two")}, map[string]bool{"1": true})
	m, _ = press(m, "enter")

	if got := len(m.articleRefs()); got != 2 {
		t.Fatalf("show-read default yes: %d refs, want 2", got)
	}
	m, _ = press(m, "t")
	if got := len(m.articleRefs()); got != 1 {
		t.Fatalf("after toggle: %d refs, want 1 (unread only)", got)
	}
	if refs := m.articleRefs(); refs[0].art.ID != "2" {
		t.Errorf("remaining ref = %s, want the unread one", refs[0].art.ID)
	}
	m, _ = press(m, "t")
	if got := len(m.articleRefs()); got != 2 {
		t.Errorf("after toggle back: %d refs, want 2", got)
	}
}

// ---- next-unread / prev-unread -------------------------------------------------------

func TestNextUnreadArticleList(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	// Newest first: [3, 2, 1] with only the oldest ("1", index 2) unread.
	setState(&m, "a", []feed.Article{art("1", t0, ""), art("2", t1, ""), art("3", t2, "")}, map[string]bool{"2": true, "3": true})
	m, _ = press(m, "enter")

	m, _ = press(m, "n")
	if m.acursor != 2 {
		t.Fatalf("next-unread: acursor = %d, want 2", m.acursor)
	}
	m, _ = press(m, "p")
	if m.acursor != 2 {
		t.Fatalf("prev-unread with none before: acursor = %d, want 2 (unchanged)", m.acursor)
	}
	if !contains(m.status, "no unread") {
		t.Errorf("status = %q, want the no-unread hint", m.status)
	}
}

func TestNextUnreadFeedList(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}, {URL: "b"}, {URL: "c"}}})
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{"1": true})
	setState(&m, "b", []feed.Article{art("2", t1, "")}, map[string]bool{})
	setState(&m, "c", []feed.Article{art("3", t1, "")}, map[string]bool{})

	// n jumps to the next feed with unread articles and opens it at the
	// first unread article.
	m, _ = press(m, "n")
	if m.screen != screenArticleList || m.active != "b" {
		t.Fatalf("next-unread: screen=%v active=%q, want feed b open", m.screen, m.active)
	}
	if m.acursor != 0 {
		t.Fatalf("acursor = %d, want 0 (first unread)", m.acursor)
	}

	// From inside b, n goes to c.
	m, _ = press(m, "q")
	m, _ = press(m, "n")
	if m.active != "c" {
		t.Fatalf("second next-unread: active = %q, want c", m.active)
	}
}

// ---- width=0 regression (handoff repaint bug) ----------------------------------------

// A degenerate WindowSizeMsg (0x0) arrives around the chawan handoff
// (tea's RestoreTerminal re-queries the pty) and in ptys without a size
// (e.g. `script` with no controlling terminal). It must never shrink
// the layout to a width that truncates the status line to "…".
func TestDegenerateWindowSizeIgnored(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}})

	// Before any real size: fall back to 80x24 instead of 0x0.
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = tm.(Model)
	if !m.ready || m.width != fallbackWidth || m.height != fallbackHeight {
		t.Fatalf("after degenerate size: ready=%v %dx%d, want fallback %dx%d",
			m.ready, m.width, m.height, fallbackWidth, fallbackHeight)
	}
	m.status = "a long status message that width 0 would truncate to an ellipsis"
	v := m.View()
	if contains(v, "…\n") || !contains(v, "a long status") {
		t.Errorf("degenerate size should not truncate the status line:\n%s", v)
	}

	// After a real size, a degenerate message must not clobber it.
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(Model)
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = tm.(Model)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("degenerate size clobbered the known size: %dx%d", m.width, m.height)
	}

	// A real resize still applies.
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = tm.(Model)
	if m.width != 100 || m.height != 30 {
		t.Fatalf("real resize not applied: %dx%d", m.width, m.height)
	}
}

// ---- stubs ---------------------------------------------------------------------------

func TestStubOpsReportStatus(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	for _, key := range []string{"s", "o", "u"} {
		m.status = ""
		m, _ = press(m, key)
		if !contains(m.status, "not implemented") {
			t.Errorf("key %q should report a not-implemented status, got %q", key, m.status)
		}
	}
}

// ---- colors -------------------------------------------------------------------------

func TestColorRules(t *testing.T) {
	cfg := testConfig(t,
		"color listnormal red black",
		"color info color250 default bold",
		"color nosuchelement green default",
	)
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Config: cfg})

	if fg := colorSeq(m.sty.read.GetForeground()); fg != "1" {
		t.Errorf("listnormal fg = %q, want red (ANSI 1)", fg)
	}
	if bg := colorSeq(m.sty.read.GetBackground()); bg != "0" {
		t.Errorf("listnormal bg = %q, want black (ANSI 0)", bg)
	}
	// listnormal maps onto both read article rows and feed rows.
	if fg := colorSeq(m.sty.feed.GetForeground()); fg != "1" {
		t.Errorf("feed row should share the listnormal color, fg = %q", fg)
	}
	if fg := colorSeq(m.sty.status.GetForeground()); fg != "250" {
		t.Errorf("info fg = %q, want color250", fg)
	}
	if !m.sty.status.GetBold() {
		t.Error("info bold attribute not applied")
	}
	// Elements without rules keep the default palette.
	if fg := colorSeq(m.sty.selected.GetForeground()); fg != "15" {
		t.Errorf("listfocus without a rule should keep the default, fg = %q", fg)
	}
	// Unknown elements are ignored without breaking anything.
	if m.sty.background != "" {
		t.Errorf("background = %q, want none", m.sty.background)
	}

	// The background element is applied when present.
	cfg2 := testConfig(t, "color background color234 default")
	m2 := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Config: cfg2})
	if m2.sty.background == "" {
		t.Error("background rule should set the view background color")
	}
}

// colorSeq extracts the color value for assertion: lipgloss.Color is a
// plain string type, so the configured value round-trips as-is ("" when
// no color is set).
func colorSeq(c lipgloss.TerminalColor) string {
	if v, ok := c.(lipgloss.Color); ok {
		return string(v)
	}
	if c == nil {
		return ""
	}
	return "?"
}

// idsOfRefs is ids() for article refs.
func idsOfRefs(refs []articleRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.art.ID
	}
	return out
}
