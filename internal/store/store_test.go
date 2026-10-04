package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

func expectedFile(t *testing.T, dir, feedURL string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(feedURL))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
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

func TestFilenameStability(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

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

	fileA := expectedFile(t, dir, urlA)
	fileB := expectedFile(t, dir, urlB)
	for _, path := range []string{fileA, fileB} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("dir has %d files, want 2 (second Save of same URL must overwrite)", len(entries))
	}
}

func TestSaveIndentedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Save("https://example.com/x.xml", stateFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(expectedFile(t, dir, "https://example.com/x.xml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var any map[string]json.RawMessage
	if err := json.Unmarshal(b, &any); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if !json.Valid(b) {
		t.Error("not valid JSON")
	}
	// 2-space indent: nested keys are on lines starting with two spaces.
	if !strings.Contains(string(b), "\n  \"") {
		t.Error("file does not look 2-space indented")
	}
}

func TestSaveAtomicUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not affect writes on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not block writes")
	}

	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	orig := stateFixture()
	if err := s.Save("https://example.com/feed.xml", orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	defer func() {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatalf("restore dir mode: %v", err)
		}
	}()

	if err := s.Save("https://example.com/feed.xml", State{ETag: `"partial"`}); err == nil {
		t.Fatal("Save into read-only dir unexpectedly succeeded")
	}

	// Original state intact, no temp leftovers.
	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load after failed Save: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("state corrupted by failed Save:\n got: %#v\nwant: %#v", got, orig)
	}
	assertNoTempFiles(t, dir)
}

func TestSaveAtomicRenameFailure(t *testing.T) {
	// Target path occupied by a directory: temp file is written fine, the
	// rename fails. Even as root this exercises the cleanup branch.
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	feedURL := "https://example.com/feed.xml"
	if err := os.Mkdir(expectedFile(t, dir, feedURL), 0o700); err != nil {
		t.Fatalf("Mkdir target: %v", err)
	}

	if err := s.Save(feedURL, stateFixture()); err == nil {
		t.Fatal("Save onto directory target unexpectedly succeeded")
	}
	assertNoTempFiles(t, dir)
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
