package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// writeLegacyJSON writes st as a legacy per-feed JSON cache file: a file
// named hex(sha256(feedURL)).json inside <dir>/feeds/, containing a State
// exactly as the old store wrote it.
func writeLegacyJSON(t *testing.T, dir, feedURL string, st State) {
	t.Helper()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	legacy := filepath.Join(dir, legacyDirName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	path := filepath.Join(legacy, feedKey(feedURL)+".json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
}

func legacyFixture() State {
	return State{
		ETag:         `"legacy-etag"`,
		LastModified: "Mon, 02 Jan 2006 15:04:05 GMT",
		LastFetched:  time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
		Articles: []feed.Article{
			{
				ID:        "legacy-2",
				Title:     "Legacy two",
				Published: time.Date(2025, 6, 2, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:          "legacy-1",
				GUID:        "guid-legacy-1",
				Title:       "Legacy one",
				URL:         "https://example.com/legacy/1",
				ContentHTML: "<p>old &amp; gold</p>",
				Author:      "Bob",
				Published:   time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC),
				Enclosures: []feed.Enclosure{
					{URL: "https://example.com/legacy/1.mp3", MimeType: "audio/mpeg", Size: 42},
				},
			},
		},
		Read: map[string]bool{"legacy-1": true},
	}
}

func TestMigrationImportsJSONCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	urlA := "https://example.com/legacy-a.xml"
	urlB := "https://example.com/legacy-b.xml"
	stA := legacyFixture()
	stB := State{ETag: `"b-etag"`, LastFetched: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)}
	writeLegacyJSON(t, dir, urlA, stA)
	writeLegacyJSON(t, dir, urlB, stB)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	gotA, ok, err := s.Load(urlA)
	if err != nil || !ok {
		t.Fatalf("Load legacy A: ok=%v err=%v", ok, err)
	}
	if !jsonEqualStates(t, gotA, stA) {
		t.Errorf("legacy A mismatch:\n got: %#v\nwant: %#v", gotA, stA)
	}
	gotB, ok, err := s.Load(urlB)
	if err != nil || !ok {
		t.Fatalf("Load legacy B: ok=%v err=%v", ok, err)
	}
	if !jsonEqualStates(t, gotB, stB) {
		t.Errorf("legacy B mismatch:\n got: %#v\nwant: %#v", gotB, stB)
	}

	// JSON data must survive: renamed, not deleted.
	backup := filepath.Join(dir, legacyBackupFmt)
	if entries, err := os.ReadDir(backup); err != nil {
		t.Fatalf("backup dir %s missing: %v", backup, err)
	} else if len(entries) != 2 {
		t.Errorf("backup dir has %d files, want 2", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, legacyDirName)); !os.IsNotExist(err) {
		t.Errorf("legacy dir still present (err=%v), want it renamed away", err)
	}
	if v, err := metaGet(s.db, metaMigrated); err != nil || v != "1" {
		t.Errorf("migration marker = %q, err = %v; want \"1\", nil", v, err)
	}

	// Re-open is a no-op even if a feeds dir reappears (e.g. restored by
	// the user): the marker short-circuits, nothing is imported.
	writeLegacyJSON(t, dir, "https://example.com/stray.xml", legacyFixture())
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer s2.Close()
	if _, ok, err := s2.Load("https://example.com/stray.xml"); err != nil || ok {
		t.Errorf("stray feed imported on re-open: ok=%v err=%v", ok, err)
	}
	if gotA2, _, err := s2.Load(urlA); err != nil || !jsonEqualStates(t, gotA2, stA) {
		t.Errorf("legacy A corrupted by re-open:\n got: %#v\n err: %v", gotA2, err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyDirName)); err != nil {
		t.Errorf("re-created feeds dir must be left alone, stat err=%v", err)
	}
}

func TestMigrationFailureLeavesJSONIntact(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	urlA := "https://example.com/good.xml"
	urlB := "https://example.com/broken.xml"
	stA := legacyFixture()
	writeLegacyJSON(t, dir, urlA, stA)

	// A malformed file in the middle of the set must abort the migration.
	brokenPath := filepath.Join(dir, legacyDirName, feedKey(urlB)+".json")
	if err := os.WriteFile(brokenPath, []byte(`{"etag": "oops`), 0o600); err != nil {
		t.Fatalf("write broken file: %v", err)
	}

	if _, err := Open(dir); err == nil {
		t.Fatal("Open unexpectedly succeeded with a malformed legacy file")
	}

	// JSON cache untouched: dir still there, both files present.
	for _, f := range []string{feedKey(urlA) + ".json", feedKey(urlB) + ".json"} {
		if _, err := os.Stat(filepath.Join(dir, legacyDirName, f)); err != nil {
			t.Errorf("legacy file %s missing after failed migration: %v", f, err)
		}
	}
	// Partial database removed: next start retries clean.
	for _, f := range []string{dbFileName, dbFileName + "-wal", dbFileName + "-shm"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("partial db file %s left behind (err=%v)", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, legacyBackupFmt)); !os.IsNotExist(err) {
		t.Error("backup dir created despite failed migration")
	}

	// Fix the broken file and retry: migration now succeeds for both.
	stB := State{ETag: `"fixed"`}
	b, err := json.Marshal(stB)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(brokenPath, b, 0o600); err != nil {
		t.Fatalf("rewrite broken file: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after fixing: %v", err)
	}
	defer s.Close()
	if got, ok, err := s.Load(urlA); err != nil || !ok || !jsonEqualStates(t, got, stA) {
		t.Errorf("good feed lost after retry: ok=%v err=%v", ok, err)
	}
	if got, ok, err := s.Load(urlB); err != nil || !ok || !jsonEqualStates(t, got, stB) {
		t.Errorf("fixed feed missing after retry: ok=%v err=%v", ok, err)
	}
}

func TestMigrationNoopWithoutJSONFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Empty feeds dir: no migration, no marker, no rename.
	if err := os.MkdirAll(filepath.Join(dir, legacyDirName), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if v, err := metaGet(s.db, metaMigrated); err != nil || v != "" {
		t.Errorf("migration marker = %q, err = %v; want unset", v, err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyDirName)); err != nil {
		t.Errorf("empty feeds dir must not be renamed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyBackupFmt)); !os.IsNotExist(err) {
		t.Error("backup dir created without migration")
	}

	// And a plain save works next to it.
	if err := s.Save("https://example.com/fresh.xml", legacyFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok, err := s.Load("https://example.com/fresh.xml"); err != nil || !ok {
		t.Errorf("Load after save: ok=%v err=%v", ok, err)
	}
}

// jsonEqualStates compares two States field by field via JSON encoding,
// giving readable diffs for time.Time location nuances.
func jsonEqualStates(t *testing.T, a, b State) bool {
	t.Helper()
	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	return string(ja) == string(jb)
}

// TestMigrateRealCache runs the migration against a copy of this
// machine's real legacy cache (DoD smoke). Opt-in via
// CHARSS_TEST_REAL_CACHE=<dir with *.json> (default ~/.cache/charss/feeds);
// skipped when unset or missing.
func TestMigrateRealCache(t *testing.T) {
	src := os.Getenv("CHARSS_TEST_REAL_CACHE")
	if src == "" {
		src = filepath.Join(homeCache(t), "charss", "feeds")
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Skipf("no real cache at %s: %v", src, err)
	}

	dir := t.TempDir()
	legacy := filepath.Join(dir, legacyDirName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("copy %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(legacy, e.Name()), b, 0o600); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
		n++
	}
	if n == 0 {
		t.Skipf("no .json files in %s", src)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open with real cache: %v", err)
	}
	defer s.Close()

	var feeds, articles int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM feeds`).Scan(&feeds); err != nil {
		t.Fatalf("count feeds: %v", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&articles); err != nil {
		t.Fatalf("count articles: %v", err)
	}
	if feeds != n {
		t.Errorf("migrated %d feeds, want %d", feeds, n)
	}
	t.Logf("real cache: %d feeds, %d articles migrated", feeds, articles)

	if _, err := os.Stat(filepath.Join(dir, legacyBackupFmt)); err != nil {
		t.Errorf("backup dir missing: %v", err)
	}
}

func homeCache(t *testing.T) string {
	t.Helper()
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("cannot locate cache dir: %v", err)
	}
	return base
}
