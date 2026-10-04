package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/podcast"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// ---- fakes ---------------------------------------------------------------

// fakeQueue records enqueued podcast items.
type fakeQueue struct {
	items []podcast.Item
	err   error
}

func (q *fakeQueue) Add(it podcast.Item) error {
	if q.err != nil {
		return q.err
	}
	q.items = append(q.items, it)
	return nil
}

// ---- flags ----------------------------------------------------------------

func TestToggleFlagPrompt(t *testing.T) {
	st := newFakeStore()
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Store: st})
	// Same timestamp: the stable sort keeps [1, 2] order, so article 1
	// sits under the cursor.
	setState(&m, "a", []feed.Article{art("1", t1, "one"), art("2", t1, "two")}, map[string]bool{})
	m, _ = press(m, "enter")

	// "^" opens the one-character prompt.
	m, _ = press(m, "^")
	if m.inputKind != inputFlag {
		t.Fatalf("inputKind = %v, want inputFlag", m.inputKind)
	}
	m = typeInput(m, "a")
	m, cmd := press(m, "enter")
	runCmd(cmd)

	if got := m.flagOf("a", "1"); got != "a" {
		t.Fatalf("flags = %q, want a", got)
	}
	if len(st.saves) != 1 || st.states["a"].Flags["1"] != "a" {
		t.Fatalf("store saves = %v, flags = %v — flag must persist", st.saves, st.states["a"].Flags)
	}

	// A second flag accumulates, sorted.
	m, _ = press(m, "^")
	m = typeInput(m, "Z")
	m, cmd = press(m, "enter")
	runCmd(cmd)
	if got := m.flagOf("a", "1"); got != "Za" {
		t.Fatalf("flags = %q, want Za (sorted)", got)
	}

	// Toggling an existing flag removes it.
	m, _ = press(m, "^")
	m = typeInput(m, "a")
	m, cmd = press(m, "enter")
	runCmd(cmd)
	if got := m.flagOf("a", "1"); got != "Z" {
		t.Fatalf("flags = %q, want Z after untoggling a", got)
	}

	// Esc cancels the prompt without touching flags.
	m, _ = press(m, "^")
	m, _ = press(m, "esc")
	if m.inputKind != inputNone {
		t.Fatal("Esc should close the flag prompt")
	}
	if got := m.flagOf("a", "1"); got != "Z" {
		t.Fatalf("flags = %q, want Z untouched after Esc", got)
	}
}

func TestToggleFlagRejectsInvalidInput(t *testing.T) {
	cases := []string{"", "ab", "!", "é"}
	for _, in := range cases {
		m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
		setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
		m, _ = press(m, "enter")
		m, _ = press(m, "^")
		m = typeInput(m, in)
		m, _ = press(m, "enter")
		if m.status == "" {
			t.Errorf("input %q should be rejected with a status error", in)
		}
		if got := m.flagOf("a", "1"); got != "" {
			t.Errorf("input %q must not set flags, got %q", in, got)
		}
	}
}

func TestFlagDisplayMarker(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}})
	setState(&m, "a", []feed.Article{art("1", t1, "Flagged one")}, map[string]bool{})
	m = sized(m)
	m, _ = press(m, "enter")

	m.setFlagOf("a", "1", "a")
	if v := m.View(); !contains(v, "[a]") {
		t.Errorf("flagged article should show [a], view:\n%s", v)
	}
	m.setFlagOf("a", "1", "")
	if v := m.View(); contains(v, "[a]") {
		t.Errorf("unflagged article must not show [a], view:\n%s", v)
	}
}

func TestFlagsFeedFilterAttribute(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "A"}}})
	setState(&m, "a", []feed.Article{art("1", t1, "one"), art("2", t2, "two")}, map[string]bool{})
	m.setFlagOf("a", "1", "a")
	m, _ = press(m, "enter")

	m, _ = press(m, "F")
	m = typeInput(m, `flags # "a"`)
	m, _ = press(m, "enter")
	refs := m.articleRefs()
	if len(refs) != 1 || refs[0].art.ID != "1" {
		t.Fatalf("filtered refs = %v, want only the flagged article 1", idsOfRefs(refs))
	}
}

// ---- save-article -----------------------------------------------------------

func TestSaveArticleWritesStrippedText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "article.txt")

	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a", Title: "Feed A"}}, Store: newFakeStore()})
	setState(&m, "a", []feed.Article{{
		ID:          "1",
		Title:       "Hello & <World>",
		URL:         "https://a/hello",
		Author:      "Alice",
		ContentHTML: `<p>First &amp; paragraph</p><script>evil()</script><p>Second <a href="https://x/">link</a></p>`,
		Published:   t1,
	}}, map[string]bool{})
	m, _ = press(m, "enter")

	m, _ = press(m, "s")
	if m.inputKind != inputSavePath {
		t.Fatalf("inputKind = %v, want inputSavePath", m.inputKind)
	}
	if got := m.input.Value(); got != "~/" {
		t.Fatalf("save prompt default = %q, want ~/ (newsboat default)", got)
	}
	m.input.SetValue(path)
	m, cmd := press(m, "enter")
	m = pump(m, cmd)

	if !contains(m.status, path) {
		t.Fatalf("status = %q, want the saved path", m.status)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("saved file unreadable: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"Title: Hello & <World>",
		"Feed: Feed A",
		"Author: Alice",
		"Date: 2026-01-02",
		"Link: https://a/hello",
		"First & paragraph",
		"Second link",
	} {
		if !contains(text, want) {
			t.Errorf("saved text missing %q:\n%s", want, text)
		}
	}
	if contains(text, "evil()") || contains(text, "<p>") || contains(text, "href") {
		t.Errorf("saved text still contains markup or script body:\n%s", text)
	}

	// Overwrite is silent (newsboat behavior) and still succeeds.
	m, _ = press(m, "s")
	m.input.SetValue(path)
	m, cmd = press(m, "enter")
	m = pump(m, cmd)
	if !contains(m.status, path) {
		t.Fatalf("status = %q, want the overwritten path", m.status)
	}
}

func TestSaveArticleError(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	m, _ = press(m, "s")
	m.input.SetValue(filepath.Join(t.TempDir(), "no-such-dir", "x", "article.txt"))
	m, cmd := press(m, "enter")
	m = pump(m, cmd)
	if m.status == "" || contains(m.status, "saved to") {
		t.Fatalf("status = %q, want a save error", m.status)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"~":             home,
		"~/news/a.txt":  filepath.Join(home, "news/a.txt"),
		"/abs/path":     "/abs/path",
		"relative.txt":  "relative.txt",
		"notilde/a.txt": "notilde/a.txt",
		"~otheruser/x":  "~otheruser/x", // unsupported, kept as-is
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text", "hello", "hello\n"},
		{"drops inline tags", "a<b>bold</b>c", "aboldc\n"},
		{"entities", "a &amp; b &#39;c&#39; &lt;x&gt;", "a & b 'c' <x>\n"},
		{"br and p break lines", "one<br>two<p>three</p>", "one\ntwo\nthree\n"},
		{"list items break", "<ul><li>a</li><li>b</li></ul>", "a\nb\n"},
		{"script body dropped", "x<script>var a = '<p>';</script>y", "xy\n"},
		{"style body dropped", "<style>p { color: red }</style>text", "text\n"},
		{"gt inside quoted attr", `<a title="a>b" href="u">k</a>`, "k\n"},
		{"comments dropped", "a<!-- hidden -->b", "ab\n"},
		{"unterminated tag", "a<b", "ab\n"},
		{"empty paragraphs collapse to one blank", "x<p></p><p></p><p>y</p>", "x\n\ny\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripHTML(tc.in); got != tc.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---- URL extraction / show-urls ---------------------------------------------

func TestExtractURLs(t *testing.T) {
	a := feed.Article{
		URL: "https://example.com/article",
		ContentHTML: `<p>See <a href="https://a.example/x">one</a> and ` +
			`<A HREF='https://b.example/y'>two</A>; again <a href="https://a.example/x">dupe</a>` +
			` and <a data-href="nope" href="https://c.example/z" target=_blank>three</a></p>`,
		Enclosures: []feed.Enclosure{{URL: "https://media.example/ep1.mp3"}},
	}
	got := extractURLs(a)
	want := []string{
		"https://example.com/article", // article URL first
		"https://a.example/x",         // then hrefs in document order
		"https://b.example/y",
		"https://c.example/z",
		"https://media.example/ep1.mp3", // enclosures last
	}
	if len(got) != len(want) {
		t.Fatalf("urls = %v, want %v (dupe must collapse)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("urls[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestExtractURLsEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		art  feed.Article
		want []string
	}{
		{"no urls at all", feed.Article{}, nil},
		{"article url deduped against href", feed.Article{
			URL:         "https://x/",
			ContentHTML: `<a href="https://x/">self</a>`,
		}, []string{"https://x/"}},
		{"unquoted href", feed.Article{
			ContentHTML: `<a href=https://unquoted.example/a>u</a>`,
		}, []string{"https://unquoted.example/a"}},
		{"entity in href", feed.Article{
			ContentHTML: `<a href="https://x/?a=1&amp;b=2">e</a>`,
		}, []string{"https://x/?a=1&b=2"}},
		{"multiple enclosures kept in order", feed.Article{
			Enclosures: []feed.Enclosure{{URL: "https://m/2"}, {URL: "https://m/1"}},
		}, []string{"https://m/2", "https://m/1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractURLs(tc.art)
			if len(got) != len(tc.want) {
				t.Fatalf("urls = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("urls[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestShowURLsScreen(t *testing.T) {
	b := &fakeBrowser{}
	term := &fakeTerminal{}
	m := newTUI(t, Options{
		Feeds:    []urls.Feed{{URL: "a", Title: "A"}},
		Browser:  b,
		Terminal: term,
		Store:    newFakeStore(),
	})
	setState(&m, "a", []feed.Article{{
		ID:          "1",
		Title:       "Linky",
		URL:         "https://example.com/a",
		ContentHTML: `<a href="https://example.com/b">b</a>`,
	}}, map[string]bool{})
	m = sized(m)
	m, _ = press(m, "enter")

	m, _ = press(m, "u")
	if m.screen != screenURLView {
		t.Fatalf("screen = %v, want screenURLView", m.screen)
	}
	if len(m.urlEntries) != 2 {
		t.Fatalf("urlEntries = %v, want 2", m.urlEntries)
	}
	v := m.View()
	if !contains(v, "1") || !contains(v, "https://example.com/a") {
		t.Errorf("URL view should list the URLs:\n%s", v)
	}

	// Navigation and selection through the dialog context.
	m, _ = press(m, "j")
	if m.ucursor != 1 {
		t.Fatalf("ucursor = %d, want 1", m.ucursor)
	}
	m, _ = press(m, "k")
	if m.ucursor != 0 {
		t.Fatalf("ucursor = %d, want 0", m.ucursor)
	}

	m, cmd := press(m, "enter")
	if !m.opening {
		t.Fatal("selecting a URL should start the browser handoff")
	}
	msgs := runCmd(cmd)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if len(b.urls) != 1 || b.urls[0] != "https://example.com/a" {
		t.Fatalf("browser urls = %v, want the first URL", b.urls)
	}
	if term.released != 1 || term.restored != 1 {
		t.Fatalf("terminal release/restore = %d/%d, want 1/1", term.released, term.restored)
	}
	tm, _ := m.Update(msgs[0]) // articleOpenedMsg clears opening
	m = tm.(Model)
	if m.opening {
		t.Fatal("opening should be false after the handoff completed")
	}

	// q returns to the article list.
	m, _ = press(m, "q")
	if m.screen != screenArticleList {
		t.Fatalf("screen = %v, want screenArticleList after q", m.screen)
	}
	m, _ = press(m, "u")
	m, _ = press(m, "esc")
	if m.screen != screenArticleList {
		t.Fatal("Esc should close the URL view too")
	}
}

// ---- open-in-browser ---------------------------------------------------------

func TestOpenInBrowserFromArticleAndFeedList(t *testing.T) {
	b := &fakeBrowser{}
	term := &fakeTerminal{}
	m := newTUI(t, Options{
		Feeds:    []urls.Feed{{URL: "https://feed.example/", Title: "F"}},
		Browser:  b,
		Terminal: term,
		Store:    newFakeStore(),
	})
	setState(&m, "https://feed.example/", []feed.Article{
		{ID: "1", Title: "One", URL: "https://feed.example/one"},
	}, map[string]bool{})

	// Article list: opens the article URL.
	m, _ = press(m, "enter")
	m, cmd := press(m, "o")
	if !m.opening {
		t.Fatal("o should start the browser handoff")
	}
	m = pump(m, cmd)
	if len(b.urls) != 1 || b.urls[0] != "https://feed.example/one" {
		t.Fatalf("browser urls = %v, want the article URL", b.urls)
	}
	if len(b.htmls) != 0 {
		t.Fatalf("ShowHTML must not be used for open-in-browser, htmls = %v", b.htmls)
	}

	// Feed list: opens the feed URL.
	m, _ = press(m, "q")
	m, cmd = press(m, "o")
	m = pump(m, cmd)
	if len(b.urls) != 2 || b.urls[1] != "https://feed.example/" {
		t.Fatalf("browser urls = %v, want the feed URL second", b.urls)
	}
}

func TestOpenInBrowserQueryFeedBlocked(t *testing.T) {
	m := newTUI(t, Options{Feeds: []urls.Feed{
		{URL: `query:Unread:unread = "yes"`},
		{URL: "https://real/"},
	}})
	m, _ = press(m, "g")
	m, _ = press(m, "o")
	if m.opening {
		t.Fatal("a query feed must not be opened in the browser")
	}
	if m.status == "" {
		t.Error("a blocked query feed should explain itself in the status")
	}
}

// ---- enqueue ------------------------------------------------------------------

func TestEnqueueFirstEnclosure(t *testing.T) {
	q := &fakeQueue{}
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "https://feed/", Title: "F"}}, Enqueue: q})
	setState(&m, "https://feed/", []feed.Article{{
		ID:    "1",
		Title: "Episode 42",
		URL:   "https://feed/42",
		Enclosures: []feed.Enclosure{
			{URL: "https://media/42.mp3", MimeType: "audio/mpeg", Size: 123},
			{URL: "https://media/42-alt.ogg"},
		},
	}}, map[string]bool{})
	m, _ = press(m, "enter")

	m, _ = press(m, "e")
	if len(q.items) != 1 {
		t.Fatalf("queue items = %v, want exactly one", q.items)
	}
	it := q.items[0]
	if it.URL != "https://media/42.mp3" {
		t.Errorf("item URL = %q, want the FIRST enclosure", it.URL)
	}
	if it.Title != "Episode 42" || it.FeedURL != "https://feed/" || it.MimeType != "audio/mpeg" || it.Size != 123 {
		t.Errorf("item = %+v, want title/feed/mimetype/size carried over", it)
	}
	if !contains(m.status, "queued") {
		t.Errorf("status = %q, want a queued confirmation", m.status)
	}
}

func TestEnqueueGuards(t *testing.T) {
	// No enclosures.
	m := newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}, Enqueue: &fakeQueue{}})
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")
	m, _ = press(m, "e")
	if m.status == "" || contains(m.status, "queued") {
		t.Fatalf("status = %q, want a no-enclosure error", m.status)
	}

	// Queue unavailable (open failure at startup).
	m = newTUI(t, Options{Feeds: []urls.Feed{{URL: "b"}}})
	setState(&m, "b", []feed.Article{{
		ID: "1", Enclosures: []feed.Enclosure{{URL: "https://m/x"}},
	}}, map[string]bool{})
	m, _ = press(m, "enter")
	m, _ = press(m, "e")
	if m.status == "" || contains(m.status, "queued") {
		t.Fatalf("status = %q, want a queue-unavailable error", m.status)
	}

	// Add failure surfaces.
	q := &fakeQueue{err: errors.New("disk full")}
	m = newTUI(t, Options{Feeds: []urls.Feed{{URL: "c"}}, Enqueue: q})
	setState(&m, "c", []feed.Article{{
		ID: "1", Enclosures: []feed.Enclosure{{URL: "https://m/y"}},
	}}, map[string]bool{})
	m, _ = press(m, "enter")
	m, _ = press(m, "e")
	if !contains(m.status, "disk full") {
		t.Fatalf("status = %q, want the queue error surfaced", m.status)
	}
}

// ---- macros ---------------------------------------------------------------------

func macroModel(t *testing.T, extra ...string) Model {
	t.Helper()
	cfg := testConfig(t, extra...)
	return newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}, {URL: "b"}}, Store: newFakeStore(), Config: cfg})
}

func TestMacroRunsOpsSequentially(t *testing.T) {
	m := macroModel(t,
		`macro m toggle-article-read ; quit ; down`,
	)
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	m, cmd := press(m, ",", "m")
	if m.macroPending {
		t.Fatal("macro selection should clear macroPending")
	}
	// Op 1 ran on the article list: article 1 is read.
	if !m.states["a"].read["1"] {
		t.Error("toggle-article-read should have run on the article list")
	}
	// Op 2 switched to the feed list; op 3 resolved in the NEW context.
	if m.screen != screenFeedList {
		t.Fatalf("screen = %v, want feed list after the macro's quit", m.screen)
	}
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (down resolved on the feed list)", m.cursor)
	}
	runCmd(cmd) // the save command from op 1
}

func TestMacroOpWithArguments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.txt")
	m := macroModel(t,
		`macro f toggle-flag a`,
		`macro s save `+path,
	)
	setState(&m, "a", []feed.Article{{ID: "1", Title: "Saved", ContentHTML: "<p>body</p>"}}, map[string]bool{})
	m, _ = press(m, "enter")

	// toggle-flag with a literal argument bypasses the prompt.
	m, _ = press(m, ",", "f")
	if got := m.flagOf("a", "1"); got != "a" {
		t.Fatalf("flags = %q, want a (macro argument applied)", got)
	}

	// save with a literal path bypasses the prompt.
	m, cmd := press(m, ",", "s")
	m = pump(m, cmd)
	if m.inputKind != inputNone {
		t.Error("save with an argument must not open the prompt")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("macro save should have written %s: %v", path, err)
	}
}

func TestMacroUnknownKeyAndPrefixGuards(t *testing.T) {
	m := macroModel(t, `macro m help`)
	m, _ = press(m, ",")
	if !m.macroPending {
		t.Fatal(", should arm macro selection")
	}
	m, _ = press(m, "z") // no macro z
	if m.macroPending {
		t.Fatal("an unknown macro key should clear macroPending")
	}
	if !contains(m.status, "not defined") {
		t.Errorf("status = %q, want a not-defined error", m.status)
	}

	// Esc cancels the selection silently.
	m.status = ""
	m, _ = press(m, ",")
	m, _ = press(m, "esc")
	if m.macroPending || m.status != "" {
		t.Fatalf("Esc should cancel macro selection silently (pending=%v status=%q)", m.macroPending, m.status)
	}

	// No macros configured at all.
	m = newTUI(t, Options{Feeds: []urls.Feed{{URL: "a"}}})
	m, _ = press(m, ",")
	if m.macroPending {
		t.Fatal("no macros: , must not arm selection")
	}
	if !contains(m.status, "no macros") {
		t.Errorf("status = %q, want a no-macros hint", m.status)
	}
}

func TestMacroPrefixAliasParses(t *testing.T) {
	// A config written for newsboat says `bind-key , macro-prefix` and
	// `macro p ...`; the alias must normalize onto run-macro semantics.
	m := macroModel(t,
		`bind-key , macro-prefix`,
		`macro p toggle-article-read`,
	)
	setState(&m, "a", []feed.Article{art("1", t1, "")}, map[string]bool{})
	m, _ = press(m, "enter")

	m, _ = press(m, ",", "p")
	if !m.states["a"].read["1"] {
		t.Fatal("aliased macro-prefix should run the macro")
	}
}

// ---- auto-reload ------------------------------------------------------------------

func TestAutoReloadTickTriggersRefresh(t *testing.T) {
	// The fetch command itself is never executed in this test (see the
	// fake-clock comment below); a fetcher is still injected because
	// startRefresh no-ops without one.
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		return feed.Fetched{Feed: f, Articles: []feed.Article{art("n", t1, "")}}, nil
	})
	cfg := testConfig(t, "auto-reload yes", "reload-time 1")
	m := newTUI(t, Options{
		Feeds:   []urls.Feed{{URL: "a"}},
		Fetcher: fetch,
		Store:   newFakeStore(),
		Config:  cfg,
	})

	// Fake-clock style: the statesLoadedMsg is delivered by hand (its
	// command is immediate), and the batch it returns — the startup
	// refresh plus the FIRST tea.Tick — is never executed, because a
	// real tick sleeps for reload-time minutes. The refresh is then
	// completed by delivering its result message, exactly like the
	// Program would.
	tm, cmd := m.Update(statesLoadedMsg{states: map[string]*feedState{}})
	m = tm.(Model)
	if m.refreshing != 1 {
		t.Fatalf("refreshing = %d, want 1 (startup refresh armed)", m.refreshing)
	}
	if cmd == nil {
		t.Fatal("startup should return the refresh batch plus the first tick")
	}
	res := feed.Fetched{Feed: urls.Feed{URL: "a"}, Articles: []feed.Article{art("n1", t1, "")}}
	tm, _ = m.Update(refreshedMsg{feed: urls.Feed{URL: "a"}, res: res})
	m = tm.(Model)
	if m.refreshing != 0 {
		t.Fatalf("refreshing = %d, want 0 after the startup refresh landed", m.refreshing)
	}

	// The tick re-runs the refresh; its command re-arms the next tick
	// (another real sleep — also never executed here).
	tm, cmd = m.Update(autoReloadTickMsg{})
	m = tm.(Model)
	if m.refreshing != 1 {
		t.Fatalf("refreshing = %d, want 1 after the tick", m.refreshing)
	}
	if cmd == nil {
		t.Fatal("the tick handler should re-arm the next tick")
	}
	tm, _ = m.Update(refreshedMsg{feed: urls.Feed{URL: "a"}, res: res})
	m = tm.(Model)
	if m.refreshing != 0 {
		t.Fatalf("refreshing = %d, want 0 after the tick's refresh landed", m.refreshing)
	}
	if len(m.states["a"].articles) != 1 {
		t.Fatalf("articles = %v, want the tick's fetch merged", ids(m.states["a"].articles))
	}
}

func TestAutoReloadOffIgnoresTick(t *testing.T) {
	calls := 0
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		calls++
		return feed.Fetched{Feed: f}, feed.ErrNotModified
	})
	m := newTUI(t, Options{
		Feeds:   []urls.Feed{{URL: "a"}},
		Fetcher: fetch,
		Store:   newFakeStore(),
		Config:  testConfig(t, "auto-reload no"), // newsboat default
	})
	m = pump(m, m.Init())

	tm, cmd := m.Update(autoReloadTickMsg{})
	m = tm.(Model)
	if cmd != nil || m.refreshing != 0 {
		t.Fatalf("a stray tick with auto-reload off must be a no-op (cmd=%v refreshing=%d)", cmd, m.refreshing)
	}
	if calls != 1 {
		t.Fatalf("fetches = %d, want only the startup refresh", calls)
	}
}

func TestParseRuntimeOptionsReloadTimeFloor(t *testing.T) {
	cases := []struct {
		lines    []string
		auto     bool
		interval time.Duration
	}{
		{[]string{"auto-reload yes", "reload-time 5"}, true, 5 * time.Minute},
		{[]string{"auto-reload yes", "reload-time 1"}, true, time.Minute},
		{[]string{"auto-reload yes", "reload-time 0"}, true, 60 * time.Minute}, // invalid falls back to the default
		{[]string{"auto-reload yes", "reload-time abc"}, true, 60 * time.Minute},
		{[]string{"auto-reload yes"}, true, 60 * time.Minute}, // default reload-time 60
		{[]string{"auto-reload no"}, false, 0},
		{nil, false, 0},
	}
	for _, tc := range cases {
		m := newTUI(t, Options{Config: testConfig(t, tc.lines...)})
		if m.autoReload != tc.auto {
			t.Errorf("%v: autoReload = %v, want %v", tc.lines, m.autoReload, tc.auto)
		}
		if m.reloadEvery != tc.interval {
			t.Errorf("%v: reloadEvery = %v, want %v", tc.lines, m.reloadEvery, tc.interval)
		}
	}
}

// ---- notifications ------------------------------------------------------------------

func TestCountNewArticles(t *testing.T) {
	old := []feed.Article{art("1", t1, ""), art("2", t2, "")}
	fresh := []feed.Article{art("2", t2, ""), art("3", t3, ""), art("4", t3, "")}
	if got := countNewArticles(old, fresh); got != 2 {
		t.Errorf("countNewArticles = %d, want 2", got)
	}
	if got := countNewArticles(old, old); got != 0 {
		t.Errorf("countNewArticles = %d, want 0 for no new", got)
	}
	if got := countNewArticles(nil, fresh); got != 3 {
		t.Errorf("countNewArticles = %d, want 3 for an empty cache", got)
	}
	if got := countNewArticles(old, nil); got != 0 {
		t.Errorf("countNewArticles = %d, want 0 for an empty fetch", got)
	}
}

func TestNotificationOnNewArticles(t *testing.T) {
	cases := []struct {
		name       string
		notify     string
		oldIDs     []string
		fetchIDs   []string
		wantStatus string // contains
		notWant    string // must not contain ("" skips)
	}{
		{
			name:       "notify on one new article",
			notify:     "yes",
			oldIDs:     []string{"o1"},
			fetchIDs:   []string{"o1", "n1"},
			wantStatus: "1 new articles",
		},
		{
			name:       "no notification without new articles",
			notify:     "yes",
			oldIDs:     []string{"o1"},
			fetchIDs:   []string{"o1"},
			wantStatus: "articles",
			notWant:    "new articles",
		},
		{
			name:       "notify-screen off keeps the normal status",
			notify:     "no",
			oldIDs:     []string{"o1"},
			fetchIDs:   []string{"o1", "n1"},
			wantStatus: "2 articles",
			notWant:    "new articles",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
				arts := make([]feed.Article, 0, len(tc.fetchIDs))
				for _, id := range tc.fetchIDs {
					arts = append(arts, art(id, t1, ""))
				}
				return feed.Fetched{Feed: f, Articles: arts}, nil
			})
			m := newTUI(t, Options{
				Feeds:   []urls.Feed{{URL: "a", Title: "A"}},
				Fetcher: fetch,
				Store:   newFakeStore(),
				Config:  testConfig(t, "notify-screen "+tc.notify),
			})
			old := make([]feed.Article, 0, len(tc.oldIDs))
			for _, id := range tc.oldIDs {
				old = append(old, art(id, t1, ""))
			}
			setState(&m, "a", old, map[string]bool{})
			m, _ = press(m, "enter")

			m, cmd := press(m, "r")
			m = pump(m, cmd)
			if !contains(m.status, tc.wantStatus) {
				t.Fatalf("status = %q, want it to contain %q", m.status, tc.wantStatus)
			}
			if tc.notWant != "" && contains(m.status, tc.notWant) {
				t.Fatalf("status = %q, must not contain %q", m.status, tc.notWant)
			}
		})
	}
}

func TestNotificationAggregatesAcrossBatch(t *testing.T) {
	// Two feeds, one new article each: the status fires once, with the
	// total, when the batch completes.
	fetch := fetchFunc(func(_ context.Context, f urls.Feed, _, _ string) (feed.Fetched, error) {
		return feed.Fetched{Feed: f, Articles: []feed.Article{art("n-"+f.URL, t1, "")}}, nil
	})
	m := newTUI(t, Options{
		Feeds:   []urls.Feed{{URL: "a", Title: "A"}, {URL: "b", Title: "B"}},
		Fetcher: fetch,
		Store:   newFakeStore(),
		Config:  testConfig(t, "notify-screen yes"),
	})
	setState(&m, "a", []feed.Article{art("o-a", t1, "")}, map[string]bool{})
	setState(&m, "b", []feed.Article{art("o-b", t1, "")}, map[string]bool{})

	m, cmd := press(m, "R")
	m = pump(m, cmd)
	if !contains(m.status, "2 new articles") {
		t.Fatalf("status = %q, want the batch total", m.status)
	}
	if m.batchNew != 0 {
		t.Errorf("batchNew = %d, want reset after the batch", m.batchNew)
	}
}
