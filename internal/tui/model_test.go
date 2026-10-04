package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/store"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// ---- fakes -------------------------------------------------------------

// fetchFunc adapts a function to the Fetcher interface.
type fetchFunc func(ctx context.Context, f urls.Feed, etag, lastModified string) (feed.Fetched, error)

func (fn fetchFunc) FetchIfModified(ctx context.Context, f urls.Feed, etag, lastModified string) (feed.Fetched, error) {
	return fn(ctx, f, etag, lastModified)
}

// fakeStore records saves in memory.
type fakeStore struct {
	states  map[string]store.State
	saves   []string
	saveErr error
}

func newFakeStore() *fakeStore { return &fakeStore{states: map[string]store.State{}} }

func (s *fakeStore) Load(feedURL string) (store.State, bool, error) {
	st, ok := s.states[feedURL]
	return st, ok, nil
}

func (s *fakeStore) Save(feedURL string, st store.State) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.states[feedURL] = st
	s.saves = append(s.saves, feedURL)
	return nil
}

// fakeTerminal records the handoff calls.
type fakeTerminal struct {
	released, restored int
	releaseErr         error
	restoreErr         error
}

func (t *fakeTerminal) Release() error {
	t.released++
	return t.releaseErr
}

func (t *fakeTerminal) Restore() error {
	t.restored++
	return t.restoreErr
}

// fakeBrowser records the HTML shown to chawan.
type fakeBrowser struct {
	htmls []string
	err   error
}

func (b *fakeBrowser) ShowHTML(_ context.Context, html string) error {
	b.htmls = append(b.htmls, html)
	return b.err
}

// ---- helpers -----------------------------------------------------------

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "ctrl+l":
		return tea.KeyMsg{Type: tea.KeyCtrlL}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

// testConfig loads a config from extra directives (none = pure defaults
// via an empty file, so the test never reads the developer's own
// ~/.config/charss/config).
func testConfig(t *testing.T, extra ...string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if len(extra) > 0 {
		if err := os.WriteFile(path, []byte(strings.Join(extra, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(path, filepath.Join(dir, "urls"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// newTUI builds a model with the default bindings applied, like the
// real cmd/tui.go always passes the loaded config.
func newTUI(t *testing.T, opts Options) Model {
	t.Helper()
	if opts.Config == nil {
		opts.Config = testConfig(t)
	}
	return New(opts)
}

// press feeds keys to the model in order, returning the model and the
// command produced by the last key.
func press(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var tm tea.Model = m
		tm, cmd = tm.Update(keyMsg(k))
		m = tm.(Model)
	}
	return m, cmd
}

// runCmd executes a command (recursively through tea.Batch) and returns
// the leaf messages, mimicking how the Program would run them.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, runCmd(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

// pump runs cmd and keeps feeding the resulting messages back into the
// model until no commands are produced, like the Program's event loop.
func pump(m Model, cmd tea.Cmd) Model {
	msgs := runCmd(cmd)
	for len(msgs) > 0 {
		var next []tea.Msg
		for _, msg := range msgs {
			var c tea.Cmd
			var tm tea.Model = m
			tm, c = tm.Update(msg)
			m = tm.(Model)
			next = append(next, runCmd(c)...)
		}
		msgs = next
	}
	return m
}

func sized(m Model) Model {
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm.(Model)
}

func setState(m *Model, feedURL string, arts []feed.Article, read map[string]bool) {
	m.states[feedURL] = &feedState{articles: arts, read: read}
}

// ---- feed list ---------------------------------------------------------

func TestFeedListNavigation(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}, {URL: "b"}, {URL: "c"}}})

	m, _ = press(m, "j", "j")
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}
	m, _ = press(m, "j") // clamp at bottom
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 (clamped)", m.cursor)
	}
	m, _ = press(m, "k")
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m, _ = press(m, "G")
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}
	m, _ = press(m, "g")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	m, _ = press(m, "k") // clamp at top
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (clamped)", m.cursor)
	}
}

func TestQuit(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	m, cmd := press(m, "q")
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q produced %T, want tea.QuitMsg", cmd())
	}

	// ctrl+c must quit from any screen.
	m = newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	m, _ = press(m, "enter")
	if m.screen != screenArticleList {
		t.Fatal("precondition: should be on the article list")
	}
	m, cmd = press(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("ctrl+c should quit from the article list")
	}
}

func TestEmptyFeedListIsSafe(t *testing.T) {
	m := newTUI(t, Options{})
	m = sized(m)

	m, cmd := press(m, "enter")
	if m.screen != screenFeedList {
		t.Error("Enter on an empty feed list must not switch screens")
	}
	if m.status == "" {
		t.Error("Enter on an empty feed list should explain itself in the status line")
	}
	m, _ = press(m, "j", "k", "r", "R", "q")
	_ = m
	_ = cmd
	if v := m.View(); v == "" {
		t.Error("View must render for an empty feed list")
	}
}

// ---- article list ------------------------------------------------------

func TestOpenArticleListAndBack(t *testing.T) {
	feeds := []urls.Feed{{URL: "https://a/feed", Title: "Feed A"}, {URL: "https://b/feed"}}
	m := newTUI(t, Options{Feeds: feeds})

	m, _ = press(m, "l") // same as Enter
	if m.screen != screenArticleList || m.active != "https://a/feed" {
		t.Fatalf("screen = %v active = %q, want article list of feed A", m.screen, m.active)
	}

	for _, key := range []string{"esc", "left", "q"} {
		m, _ = press(m, "enter") // back to feed list first
		m, _ = press(m, "enter") // open again
		m, _ = press(m, key)
		if m.screen != screenFeedList || m.active != "" {
			t.Fatalf("%s should return to the feed list, got screen=%v active=%q", key, m.screen, m.active)
		}
	}

	// cursor resets between visits
	m, _ = press(m, "enter")
	setState(&m, "https://a/feed", []feed.Article{art("1", t1, "one"), art("2", t2, "two")}, map[string]bool{})
	m, _ = press(m, "j")
	if m.acursor != 1 {
		t.Fatalf("acursor = %d, want 1", m.acursor)
	}
	m, _ = press(m, "q", "enter")
	if m.acursor != 0 {
		t.Fatalf("acursor = %d, want 0 after re-entering", m.acursor)
	}
}

func TestArticleListNavigation(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	setState(&m, "a", []feed.Article{art("1", t1, ""), art("2", t2, ""), art("3", t0, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	m, _ = press(m, "j", "j", "j") // clamp at bottom
	if m.acursor != 2 {
		t.Fatalf("acursor = %d, want 2", m.acursor)
	}
	m, _ = press(m, "k")
	if m.acursor != 1 {
		t.Fatalf("acursor = %d, want 1", m.acursor)
	}
	m, _ = press(m, "G")
	if m.acursor != 2 {
		t.Fatalf("acursor = %d, want 2", m.acursor)
	}
	m, _ = press(m, "g")
	if m.acursor != 0 {
		t.Fatalf("acursor = %d, want 0", m.acursor)
	}
}

func TestToggleRead(t *testing.T) {
	st := newFakeStore()
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Store: st})
	setState(&m, "a", []feed.Article{art("1", t1, "one"), art("2", t2, "two")}, map[string]bool{})
	m, _ = press(m, "enter")

	if got := m.states["a"].unread(); got != 2 {
		t.Fatalf("unread = %d, want 2", got)
	}

	m, cmd := press(m, "m")
	runCmd(cmd)
	if len(st.saves) != 1 || st.saves[0] != "a" {
		t.Fatalf("store saves = %v, want one save for feed a", st.saves)
	}
	if got := m.states["a"].unread(); got != 1 {
		t.Fatalf("unread = %d, want 1 after marking the first article read", got)
	}

	m, _ = press(m, "m") // toggle back to unread
	if got := m.states["a"].unread(); got != 2 {
		t.Fatalf("unread = %d, want 2 after untoggling", got)
	}
}

func TestMarkAllRead(t *testing.T) {
	st := newFakeStore()
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Store: st})
	setState(&m, "a", []feed.Article{art("1", t1, ""), art("2", t2, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	m, cmd := press(m, "A")
	runCmd(cmd)
	if got := m.states["a"].unread(); got != 0 {
		t.Fatalf("unread = %d, want 0 after mark-all-read", got)
	}
	if len(st.saves) != 1 {
		t.Fatalf("store saves = %d, want 1", len(st.saves))
	}
	if !st.states["a"].Read["1"] || !st.states["a"].Read["2"] {
		t.Fatal("read flags not persisted")
	}
}

func TestOpenArticleOnEmptyList(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Browser: &fakeBrowser{}, Terminal: &fakeTerminal{}})
	m, _ = press(m, "enter") // no state: empty article list

	m, cmd := press(m, "enter")
	if cmd != nil {
		t.Fatal("opening with no articles must not run anything")
	}
	if m.opening {
		t.Error("opening must stay false")
	}
	if m.status == "" {
		t.Error("should hint how to reload")
	}
}

// ---- chawan handoff ----------------------------------------------------

func openReadyModel(t *testing.T) (Model, *fakeBrowser, *fakeTerminal) {
	t.Helper()
	b := &fakeBrowser{}
	term := &fakeTerminal{}
	m := newTUI(t, Options{
		Feeds:    []urls.Feed{{URL: "a", Title: "Feed A"}},
		Browser:  b,
		Terminal: term,
		Store:    newFakeStore(),
	})
	setState(&m, "a", []feed.Article{
		{ID: "x1", Title: "Hello", URL: "https://a/hello", ContentHTML: "<p>hello world</p>", Published: t1},
	}, map[string]bool{})
	m, _ = press(m, "enter")
	return m, b, term
}

func TestOpenArticleHandoffSuccess(t *testing.T) {
	m, b, term := openReadyModel(t)

	m, cmd := press(m, "enter")
	if !m.opening {
		t.Fatal("model should be in the opening state")
	}
	if m.states["a"].read["x1"] != true {
		t.Fatal("opening an article should mark it read (newsboat behavior)")
	}

	msgs := runCmd(cmd)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (articleOpenedMsg)", len(msgs))
	}
	opened, ok := msgs[0].(articleOpenedMsg)
	if !ok {
		t.Fatalf("got %T, want articleOpenedMsg", msgs[0])
	}
	if opened.err != nil {
		t.Fatalf("articleOpenedMsg.err = %v, want nil", opened.err)
	}

	if term.released != 1 || term.restored != 1 {
		t.Fatalf("terminal release/restore = %d/%d, want 1/1", term.released, term.restored)
	}
	if len(b.htmls) != 1 {
		t.Fatalf("browser called %d times, want 1", len(b.htmls))
	}
	if want := "<p>hello world</p>"; !contains(b.htmls[0], want) {
		t.Errorf("html does not contain %q:\n%s", want, b.htmls[0])
	}
	if want := "Hello"; !contains(b.htmls[0], want) {
		t.Errorf("html does not contain the title %q:\n%s", want, b.htmls[0])
	}

	// Close the loop: the message clears the opening state.
	m = pump(m, nil)
	tm, _ := m.Update(opened)
	m = tm.(Model)
	if m.opening {
		t.Error("opening should be false after articleOpenedMsg")
	}
}

func TestOpenArticleHandoffBrowserError(t *testing.T) {
	m, b, term := openReadyModel(t)
	b.err = errors.New("chawan not found — install from https://chawan.net: exec: not found")

	m, cmd := press(m, "enter")
	msgs := runCmd(cmd)
	opened, ok := msgs[0].(articleOpenedMsg)
	if !ok {
		t.Fatalf("got %T, want articleOpenedMsg", msgs[0])
	}
	if opened.err == nil {
		t.Fatal("browser error must be reported")
	}
	// The terminal must be restored even when chawan fails.
	if term.restored != 1 {
		t.Fatalf("restored = %d, want 1", term.restored)
	}

	tm, _ := m.Update(opened)
	m = tm.(Model)
	if m.opening {
		t.Error("opening should be false after a failed open")
	}
	if !contains(m.status, "chawan") {
		t.Errorf("status %q should surface the chawan error", m.status)
	}
}

func TestOpenArticleHandoffReleaseError(t *testing.T) {
	m, b, term := openReadyModel(t)
	term.releaseErr = errors.New("cannot suspend input reader")

	m, cmd := press(m, "enter")
	msgs := runCmd(cmd)
	opened := msgs[0].(articleOpenedMsg)
	if opened.err == nil || !contains(opened.err.Error(), "releasing terminal") {
		t.Fatalf("err = %v, want a release failure", opened.err)
	}
	if len(b.htmls) != 0 {
		t.Error("browser must not run when the terminal could not be released")
	}
	if term.restored != 0 {
		t.Errorf("restored = %d, want 0 (nothing was released)", term.restored)
	}
}

func TestOpeningStateSwallowsKeys(t *testing.T) {
	m, _, _ := openReadyModel(t)
	m, _ = press(m, "enter") // now opening

	m2, cmd := press(m, "j", "enter", "q")
	if cmd != nil {
		t.Error("keys must produce no commands while chawan owns the terminal")
	}
	if m2.acursor != m.acursor || m2.screen != screenArticleList {
		t.Error("keys must not change state while opening")
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// ---- refresh -----------------------------------------------------------

func TestRefreshSingleFeed(t *testing.T) {
	st := newFakeStore()
	var gotETag, gotLM string
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, etag, lastModified string) (feed.Fetched, error) {
		gotETag, gotLM = etag, lastModified
		return feed.Fetched{
			Feed:         f,
			Articles:     []feed.Article{art("n1", t2, "new"), art("n2", t1, "newer")},
			ETag:         `"v2"`,
			LastModified: "Mon, 02 Jan 2026 00:00:00 GMT",
		}, nil
	})

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}, Fetcher: fetch, Store: st})
	setState(&m, "a", []feed.Article{art("o1", t0, "old")}, map[string]bool{"o1": true})

	m, cmd := press(m, "r")
	if m.refreshing != 1 {
		t.Fatalf("refreshing = %d, want 1", m.refreshing)
	}
	m = pump(m, cmd)

	if gotETag != "" || gotLM != "" {
		t.Errorf("first refresh should send empty validators, got etag=%q lm=%q", gotETag, gotLM)
	}
	s := m.states["a"]
	if len(s.articles) != 3 {
		t.Fatalf("articles = %v, want 3 (2 fresh + 1 old)", ids(s.articles))
	}
	if ids(s.articles)[0] != "n1" { // newest first
		t.Errorf("ordering = %v, want n1 first", ids(s.articles))
	}
	if !s.read["o1"] || s.read["n1"] || s.read["n2"] {
		t.Errorf("read = %v, want o1 read and new articles unread", s.read)
	}
	if s.etag != `"v2"` {
		t.Errorf("etag = %q, want stored for the next conditional request", s.etag)
	}
	if len(st.saves) != 1 || st.saves[0] != "a" {
		t.Fatalf("store saves = %v, want one save for a", st.saves)
	}
	if st.states["a"].ETag != `"v2"` {
		t.Errorf("saved etag = %q", st.states["a"].ETag)
	}
}

func TestRefreshSendsValidatorsAndMergesRead(t *testing.T) {
	st := newFakeStore()
	var gotETag string
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, etag, _ string) (feed.Fetched, error) {
		gotETag = etag
		return feed.Fetched{
			Feed:     f,
			Articles: []feed.Article{art("o1", t0, "old refreshed"), art("n1", t2, "")},
			ETag:     `"v3"`,
		}, nil
	})

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Fetcher: fetch, Store: st})
	setState(&m, "a", []feed.Article{art("o1", t0, "old")}, map[string]bool{"o1": true})
	m.states["a"].etag = `"v2"`

	m, cmd := press(m, "r")
	m = pump(m, cmd)

	if gotETag != `"v2"` {
		t.Errorf("fetch got etag %q, want the stored validator", gotETag)
	}
	s := m.states["a"]
	if !s.read["o1"] {
		t.Error("read flag must survive a refresh that still returns the article")
	}
	if len(s.articles) != 2 {
		t.Fatalf("articles = %v, want [n1 o1]", ids(s.articles))
	}
}

func TestRefreshNotModified(t *testing.T) {
	st := newFakeStore()
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		return feed.Fetched{Feed: f, ETag: `"v3"`}, feed.ErrNotModified
	})

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}, Fetcher: fetch, Store: st})
	old := []feed.Article{art("o1", t1, "old")}
	setState(&m, "a", old, map[string]bool{"o1": true})
	m.states["a"].etag = `"v2"`

	m, cmd := press(m, "r")
	m = pump(m, cmd)

	s := m.states["a"]
	if len(s.articles) != 1 || s.articles[0].ID != "o1" {
		t.Fatalf("304 must keep the cached articles, got %v", ids(s.articles))
	}
	if s.etag != `"v3"` {
		t.Errorf("etag = %q, want updated to the 304 header value", s.etag)
	}
	if !contains(m.status, "up to date") {
		t.Errorf("status = %q, want an up-to-date note", m.status)
	}
	if m.refreshing != 0 {
		t.Errorf("refreshing = %d, want 0", m.refreshing)
	}
}

func TestRefreshErrorKeepsCache(t *testing.T) {
	st := newFakeStore()
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		return feed.Fetched{}, &feed.HTTPError{URL: f.URL, Status: 500}
	})

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}, Fetcher: fetch, Store: st})
	setState(&m, "a", []feed.Article{art("o1", t1, "old")}, map[string]bool{})

	m, cmd := press(m, "r")
	m = pump(m, cmd)

	s := m.states["a"]
	if len(s.articles) != 1 || s.articles[0].ID != "o1" {
		t.Fatalf("a failed refresh must keep the cache, got %v", ids(s.articles))
	}
	if !contains(m.status, "500") {
		t.Errorf("status = %q, want the HTTP error surfaced", m.status)
	}
	if len(st.saves) != 0 {
		t.Errorf("a failed refresh must not save, saves = %v", st.saves)
	}
}

func TestRefreshAll(t *testing.T) {
	st := newFakeStore()
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		return feed.Fetched{Feed: f, Articles: []feed.Article{art("x-"+f.URL, t2, "")}}, nil
	})
	feeds := []urls.Feed{{URL: "a"}, {URL: "b"}, {URL: "c"}}
	m := newTUI(t, Options{Feeds: feeds, Fetcher: fetch, Store: st})

	m, cmd := press(m, "R")
	if m.refreshing != 3 {
		t.Fatalf("refreshing = %d, want 3", m.refreshing)
	}
	m = pump(m, cmd)

	for _, f := range feeds {
		if got := ids(m.states[f.URL].articles); len(got) != 1 || got[0] != "x-"+f.URL {
			t.Errorf("feed %s articles = %v", f.URL, got)
		}
	}
	if m.refreshing != 0 {
		t.Errorf("refreshing = %d, want 0 after all refreshes land", m.refreshing)
	}
	if len(st.saves) != 3 {
		t.Errorf("store saves = %d, want 3", len(st.saves))
	}
}

// ---- startup -----------------------------------------------------------

func TestStartupLoadsCacheThenAutoRefreshes(t *testing.T) {
	st := newFakeStore()
	if err := st.Save("a", store.State{
		ETag:     `"v1"`,
		Articles: []feed.Article{art("cached", t1, "cached")},
		Read:     map[string]bool{"cached": true},
	}); err != nil {
		t.Fatal(err)
	}

	var fetched bool
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, etag, _ string) (feed.Fetched, error) {
		fetched = true
		if etag != `"v1"` {
			t.Errorf("auto-refresh etag = %q, want the cached validator", etag)
		}
		return feed.Fetched{Feed: f, ETag: `"v1"`}, feed.ErrNotModified
	})

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}, Fetcher: fetch, Store: st})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should load cached state")
	}
	m = pump(m, cmd)

	if !fetched {
		t.Error("startup should auto-refresh in the background")
	}
	s := m.states["a"]
	if len(s.articles) != 1 || s.articles[0].ID != "cached" {
		t.Fatalf("articles = %v, want the cached set", ids(s.articles))
	}
	if !s.read["cached"] {
		t.Error("cached read flags should be restored")
	}
	if m.unreadTotal() != 0 {
		t.Errorf("unread total = %d, want 0", m.unreadTotal())
	}
}

func TestStartupWithoutStore(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	if cmd := m.Init(); cmd != nil {
		t.Error("no store: Init should be a no-op, not crash or fetch")
	}
}

// ---- unread counting / view --------------------------------------------

func TestUnreadCounting(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}, {URL: "b"}, {URL: "c"}}})
	if got := m.unreadTotal(); got != 0 {
		t.Fatalf("unread total = %d, want 0 with no state", got)
	}
	setState(&m, "a", []feed.Article{art("1", t1, ""), art("2", t2, "")}, map[string]bool{"1": true})
	setState(&m, "b", []feed.Article{art("3", t1, "")}, map[string]bool{})
	// c has no state at all (nil entry) — must be nil-safe.

	if got := m.unreadTotal(); got != 2 {
		t.Fatalf("unread total = %d, want 2", got)
	}
	if got := m.states["a"].unread(); got != 1 {
		t.Errorf("feed a unread = %d, want 1", got)
	}
}

func TestViewSmoke(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "https://a/feed", Title: "Feed A"}}})
	m = sized(m)
	v := m.View()
	if !contains(v, "Feed A") {
		t.Errorf("feed list view does not show the feed title:\n%s", v)
	}

	setState(&m, "https://a/feed", []feed.Article{{ID: "1", Title: "Read one"}, {ID: "2", Title: "Unread two"}}, map[string]bool{"1": true})
	m, _ = press(m, "enter")
	v = m.View()
	if !contains(v, "Feed A") {
		t.Errorf("article list header should show the feed title:\n%s", v)
	}
	if !contains(v, "N ") {
		t.Errorf("unread articles should carry the N prefix:\n%s", v)
	}
}

func TestStatusLineSurvivesRefresh(t *testing.T) {
	m := newTUI(t, Options{})
	m = sized(m)
	m.status = "chawan not found — install from https://chawan.net"
	m.refreshing = 3

	v := m.View()
	if !contains(v, "chawan not found") {
		t.Error("an in-flight refresh must not hide the status message")
	}
	if !contains(v, "⟳ 3") {
		t.Error("the refresh counter should render in the header badge")
	}
}

func TestViewLongListScrolls(t *testing.T) {
	feeds := make([]urls.Feed, 50)
	for i := range feeds {
		feeds[i] = urls.Feed{URL: fmt.Sprintf("https://f/%d", i), Title: fmt.Sprintf("Feed %02d", i)}
	}
	m := newTUI(t, Options{Feeds: feeds})
	m = sized(m)
	m, _ = press(m, "G") // jump to the last feed

	v := m.View()
	if !contains(v, "Feed 49") {
		t.Error("the last feed should be visible after jumping to the end")
	}
	if contains(v, "Feed 00") {
		t.Error("the first feed should have scrolled out at the bottom")
	}
}
