package store

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

func stateFixture() State {
	return State{
		ETag:         `"etag-1"`,
		LastModified: "Mon, 02 Jan 2006 15:04:05 GMT",
		LastFetched:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Articles: []feed.Article{
			{
				ID:          "id-1",
				GUID:        "guid-1",
				Title:       "First",
				URL:         "https://example.com/1",
				ContentHTML: "<p>rich</p>",
				Author:      "Alice",
				Published:   time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
			},
		},
		Read: map[string]bool{"id-1": true},
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state State
	}{
		{"empty state", State{}},
		{"validators only", State{ETag: `"e"`, LastModified: "Mon, 02 Jan 2006 15:04:05 GMT", LastFetched: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)}},
		{"full state", stateFixture()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer s.Close()
			if err := s.Save("https://example.com/feed.xml", tc.state); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, ok, err := s.Load("https://example.com/feed.xml")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !ok {
				t.Fatal("Load reported no state after Save")
			}
			if !reflect.DeepEqual(got, tc.state) {
				t.Errorf("roundtrip mismatch:\n got: %#v\nwant: %#v", got, tc.state)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	st, ok, err := s.Load("https://never-saved.example.com/feed")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ok {
		t.Error("ok = true for missing state")
	}
	if !st.LastFetched.IsZero() || st.Articles != nil || st.Read != nil {
		t.Errorf("missing state should be zero, got %#v", st)
	}
}

func TestOpenCreatesDBFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	dbPath := filepath.Join(dir, dbFileName)
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("expected database file %s: %v", dbPath, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("database mode is %o, want 0600", perm)
	}
}

func TestOpenPragmasAndSchema(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	var busy int
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busy != busyTimeoutMS {
		t.Errorf("busy_timeout = %d, want %d", busy, busyTimeoutMS)
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
	if v, err := metaGet(s.db, "schema_version"); err != nil || v != "2" {
		t.Errorf("schema_version = %q, err = %v; want \"2\", nil", v, err)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := s.db.Exec(`INSERT INTO articles (id, feed_url) VALUES ('x', 'no-such-feed')`); err == nil {
		t.Error("orphan article insert unexpectedly succeeded; foreign_keys not enforced")
	}
}

// Re-saving a feed replaces its rows instead of duplicating them, and
// distinct feeds coexist (the SQLite analog of the old one-file-per-feed
// overwrite behavior).
func TestSaveUpsertsNoDuplicates(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	urlA := "https://example.com/a.xml"
	urlB := "https://example.com/b.xml"
	st := stateFixture()
	for range 2 {
		if err := s.Save(urlA, st); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if err := s.Save(urlB, st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var feeds, articles int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM feeds`).Scan(&feeds); err != nil {
		t.Fatalf("count feeds: %v", err)
	}
	if feeds != 2 {
		t.Errorf("feeds has %d rows, want 2", feeds)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&articles); err != nil {
		t.Fatalf("count articles: %v", err)
	}
	if articles != 2 {
		t.Errorf("articles has %d rows, want 2 (second Save of same URL must overwrite)", articles)
	}
}

// A Save that fails mid-transaction (here: duplicate article IDs violate
// the PK) must leave previously stored state intact.
func TestSaveFailureIsAtomic(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	feedURL := "https://example.com/feed.xml"
	orig := stateFixture()
	if err := s.Save(feedURL, orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dup := stateFixture()
	dup.Articles = append([]feed.Article{dup.Articles[0]}, dup.Articles[0])
	if err := s.Save(feedURL, dup); err == nil {
		t.Fatal("Save with duplicate article IDs unexpectedly succeeded")
	}

	got, ok, err := s.Load(feedURL)
	if err != nil || !ok {
		t.Fatalf("Load after failed Save: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("state corrupted by failed Save:\n got: %#v\nwant: %#v", got, orig)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles WHERE feed_url = ?`, feedKey(feedURL)).Scan(&n); err != nil {
		t.Fatalf("count articles: %v", err)
	}
	if n != 1 {
		t.Errorf("articles rows = %d after failed Save, want 1", n)
	}
}

func TestLoadOrdersNewestFirst(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	tie := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	st := State{
		Articles: []feed.Article{
			{ID: "old", Published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "tie-a", Published: tie},
			{ID: "tie-b", Published: tie},
			{ID: "zero"},
			{ID: "mid", Published: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	wantOrder := []string{"tie-a", "tie-b", "mid", "old", "zero"} // ties keep saved order
	if len(got.Articles) != len(wantOrder) {
		t.Fatalf("got %d articles, want %d", len(got.Articles), len(wantOrder))
	}
	for i, id := range wantOrder {
		if got.Articles[i].ID != id {
			t.Errorf("articles[%d] = %s, want %s", i, got.Articles[i].ID, id)
		}
	}
}

func TestReadFlagsRoundtrip(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	st := State{
		Articles: []feed.Article{
			{ID: "a", Title: "read one"},
			{ID: "b", Title: "unread"},
			{ID: "c", Title: "read two"},
		},
		Read: map[string]bool{"a": true, "c": true},
	}
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	want := map[string]bool{"a": true, "c": true}
	if !reflect.DeepEqual(got.Read, want) {
		t.Errorf("read map mismatch:\n got: %#v\nwant: %#v", got.Read, want)
	}

	// Flipping a flag and re-saving persists it.
	got.Read["b"] = true
	if err := s.Save("https://example.com/feed.xml", got); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got2, _, err := s.Load("https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want2 := map[string]bool{"a": true, "b": true, "c": true}
	if !reflect.DeepEqual(got2.Read, want2) {
		t.Errorf("read map mismatch after flag flip:\n got: %#v\nwant: %#v", got2.Read, want2)
	}
}

// Busy-retry smoke: two goroutines hammering the same database with saves
// of different feeds must both succeed (WAL + busy_timeout + retries).
func TestConcurrentSaves(t *testing.T) {
	t.Parallel()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	urlA := "https://example.com/a.xml"
	urlB := "https://example.com/b.xml"

	var wg sync.WaitGroup
	for _, url := range []string{urlA, urlB} {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			for i := range 25 {
				st := stateFixture()
				st.ETag = `"busy-` + url + `"`
				st.Articles[0].GUID = url
				st.Articles[0].Published = time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)
				if err := s.Save(url, st); err != nil {
					t.Errorf("Save %s: %v", url, err)
					return
				}
			}
		}(url)
	}
	wg.Wait()

	for _, url := range []string{urlA, urlB} {
		got, ok, err := s.Load(url)
		if err != nil || !ok {
			t.Fatalf("Load %s: ok=%v err=%v", url, ok, err)
		}
		if len(got.Articles) != 1 || got.Articles[0].GUID != url {
			t.Errorf("Load %s: got %#v", url, got.Articles)
		}
		if got.Articles[0].Published.Second() != 24 {
			t.Errorf("Load %s: last save did not win, published=%v", url, got.Articles[0].Published)
		}
	}
}

func TestFlagsRoundtrip(t *testing.T) {
	t.Parallel()

	st := stateFixture()
	st.Articles = append(st.Articles, feed.Article{ID: "id-2", Title: "Second"})
	st.Flags = map[string]string{"id-1": "a", "id-2": "Za"}

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got.Flags, st.Flags) {
		t.Errorf("flags roundtrip mismatch:\n got: %#v\nwant: %#v", got.Flags, st.Flags)
	}

	// Clearing flags round-trips too: an empty entry is dropped, and a
	// state without flags loads with a nil map.
	st.Flags = map[string]string{"id-1": "a"}
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, err = s.Load("https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{"id-1": "a"}
	if !reflect.DeepEqual(got.Flags, want) {
		t.Errorf("flags = %#v, want %#v", got.Flags, want)
	}

	st.Flags = nil
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, err = s.Load("https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Flags != nil {
		t.Errorf("flags = %#v, want nil when none are set", got.Flags)
	}
}

// A database written before flags existed (schema_version 1) must gain
// the column on the next open, transparently.
func TestFlagsColumnMigrationFromSchemaV1(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Regress the database to the pre-flags schema.
	if _, err := s.db.Exec(`ALTER TABLE articles DROP COLUMN flags`); err != nil {
		s.Close()
		t.Fatalf("regress schema: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE meta SET value = '1' WHERE key = 'schema_version'`); err != nil {
		s.Close()
		t.Fatalf("regress version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen: initSchema re-adds the column; saving flags then works.
	s, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen after migration: %v", err)
	}
	defer s.Close()

	st := stateFixture()
	st.Flags = map[string]string{"id-1": "Q"}
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatalf("Save after migration: %v", err)
	}
	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if got.Flags["id-1"] != "Q" {
		t.Errorf("flags after migration = %#v, want id-1: Q", got.Flags)
	}
}
