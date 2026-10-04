package podcast

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOpenQueueMissingDirAndFile(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "charss")
	q, err := OpenQueue(dir)
	if err != nil {
		t.Fatalf("OpenQueue on missing dir: %v", err)
	}
	// Dir was created.
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("OpenQueue did not create %s: %v", dir, err)
	}
	// Missing queue file is an empty queue, not an error.
	if items := q.Items(); len(items) != 0 {
		t.Fatalf("Items() = %d, want 0", len(items))
	}
}

func TestOpenQueueDefaultDir(t *testing.T) {
	// Empty dir selects $XDG_CACHE_HOME/charss. Not parallel: Setenv.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	q, err := OpenQueue("")
	if err != nil {
		t.Fatalf("OpenQueue default dir: %v", err)
	}
	if got, want := filepath.Base(q.Path()), "queue.json"; got != want {
		t.Errorf("queue file base = %q, want %q", got, want)
	}
}

func TestOpenQueueMalformedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenQueue(dir); err == nil {
		t.Fatal("OpenQueue on malformed queue.json: want error, got nil")
	}
}

func TestQueueAddDedupe(t *testing.T) {
	t.Parallel()

	q, err := OpenQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := Item{URL: "https://example.com/a.mp3", Title: "A", Size: 10}
	if err := q.Add(first); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Same URL, different title: still a duplicate, no error, no second row.
	if err := q.Add(Item{URL: "https://example.com/a.mp3", Title: "A again"}); err != nil {
		t.Fatalf("Add duplicate: %v", err)
	}
	if err := q.Add(Item{URL: "https://example.com/b.mp3", Title: "B"}); err != nil {
		t.Fatalf("Add second: %v", err)
	}

	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("Items() len = %d, want 2", len(items))
	}
	if items[0].URL != first.URL || items[1].URL != "https://example.com/b.mp3" {
		t.Errorf("order not stable: %v", items)
	}
	if items[0].Title != "A" {
		t.Errorf("duplicate Add overwrote the original item: %#v", items[0])
	}
	if items[0].AddedAt.IsZero() || items[0].EnqueuedAt.IsZero() {
		t.Errorf("Add must stamp AddedAt/EnqueuedAt: %#v", items[0])
	}
}

func TestQueueItemsSnapshotIsACopy(t *testing.T) {
	t.Parallel()

	q, err := OpenQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Add(Item{URL: "https://example.com/a.mp3"}); err != nil {
		t.Fatal(err)
	}
	items := q.Items()
	items[0].URL = "mutated"
	if got := q.Items()[0].URL; got != "https://example.com/a.mp3" {
		t.Errorf("Items() leaked internal state: %q", got)
	}
}

func TestQueueRemove(t *testing.T) {
	t.Parallel()

	q, err := OpenQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Add(Item{URL: "https://example.com/a.mp3"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Remove("https://example.com/a.mp3"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(q.Items()) != 0 {
		t.Errorf("item still present after Remove")
	}
	if err := q.Remove("https://example.com/missing.mp3"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Remove unknown URL: err = %v, want ErrNotFound", err)
	}
}

func TestQueueMarkTransitions(t *testing.T) {
	t.Parallel()

	q, err := OpenQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Add(Item{URL: "https://example.com/a.mp3", Size: -1}); err != nil {
		t.Fatal(err)
	}

	if err := q.MarkDownloading("https://example.com/a.mp3"); err != nil {
		t.Fatalf("MarkDownloading: %v", err)
	}
	if got := q.Items()[0].Status; got != StatusDownloading {
		t.Errorf("status after MarkDownloading = %v, want downloading", got)
	}
	if !q.Items()[0].EnqueuedAt.After(time.Time{}) {
		t.Error("MarkDownloading should refresh EnqueuedAt")
	}

	if err := q.MarkFailed("https://example.com/a.mp3", "HTTP 404 Not Found"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	it := q.Items()[0]
	if it.Status != StatusFailed || it.Error != "HTTP 404 Not Found" {
		t.Errorf("after MarkFailed: %#v", it)
	}

	if err := q.MarkDownloaded("https://example.com/a.mp3", "/tmp/a.mp3"); err != nil {
		t.Fatalf("MarkDownloaded: %v", err)
	}
	it = q.Items()[0]
	if it.Status != StatusDone || it.FilePath != "/tmp/a.mp3" || it.Error != "" {
		t.Errorf("after MarkDownloaded: %#v", it)
	}

	if err := q.MarkDownloaded("https://example.com/missing.mp3", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkDownloaded unknown URL: err = %v, want ErrNotFound", err)
	}
}

func TestQueueRoundtripAndJSONValidity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	q, err := OpenQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{
			URL: "https://example.com/a.mp3", Title: "A", FeedURL: "https://example.com/feed.xml",
			MimeType: "audio/mpeg", Size: 123,
		},
		{URL: "https://example.com/b.ogg", MimeType: "audio/ogg", Size: -1},
	}
	for _, it := range want {
		if err := q.Add(it); err != nil {
			t.Fatalf("Add %s: %v", it.URL, err)
		}
	}

	// The on-disk file is valid JSON.
	b, err := os.ReadFile(filepath.Join(dir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("queue.json is not valid JSON:\n%s", b)
	}

	// A fresh handle on the same dir sees the same items (both reads
	// come from disk, i.e. after the JSON roundtrip).
	q2, err := OpenQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, after := q.Items(), q2.Items()
	if len(after) != len(want) {
		t.Fatalf("Items() len = %d, want %d", len(after), len(want))
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("roundtrip mismatch:\n got: %#v\nwant: %#v", after, before)
	}
	if after[0].AddedAt.IsZero() || after[0].EnqueuedAt.IsZero() {
		t.Error("AddedAt/EnqueuedAt did not survive the roundtrip")
	}
	if after[1].Size != -1 {
		t.Errorf("Size = %d, want -1 to survive the roundtrip", after[1].Size)
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()

	cases := map[Status]string{
		StatusQueued:      "queued",
		StatusDownloading: "downloading",
		StatusDone:        "done",
		StatusFailed:      "failed",
		StatusCancelled:   "cancelled",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}
