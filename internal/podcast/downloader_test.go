package podcast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func mustQueue(t *testing.T) *Queue {
	t.Helper()
	q, err := OpenQueue(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func findItem(t *testing.T, q *Queue, url string) Item {
	t.Helper()
	for _, it := range q.Items() {
		if it.URL == url {
			return it
		}
	}
	t.Fatalf("queue has no item %s", url)
	return Item{}
}

func TestNextEmptyQueue(t *testing.T) {
	t.Parallel()

	q := mustQueue(t)
	d := &Downloader{Dir: t.TempDir()}
	if _, err := d.Next(context.Background(), q); !errors.Is(err, ErrQueueEmpty) {
		t.Fatalf("err = %v, want ErrQueueEmpty", err)
	}
}

func TestNextSuccess(t *testing.T) {
	t.Parallel()

	const body = "podcast audio bytes"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	q := mustQueue(t)
	if err := q.Add(Item{URL: ts.URL + "/ep1.mp3", Title: "Ep1", Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	var lastDone, lastTotal int64
	d := &Downloader{Dir: dir, OnProgress: func(url string, done, total int64) {
		lastDone, lastTotal = done, total
	}}
	item, err := d.Next(context.Background(), q)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	wantPath := filepath.Join(dir, "ep1.mp3")
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("reading %s: %v", wantPath, err)
	}
	if string(got) != body {
		t.Errorf("file content = %q, want %q", got, body)
	}
	if _, err := os.Stat(wantPath + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".part file still exists: %v", err)
	}
	if item.Status != StatusDone || item.FilePath != wantPath {
		t.Errorf("returned item = %#v", item)
	}
	if it := findItem(t, q, ts.URL+"/ep1.mp3"); it.Status != StatusDone || it.FilePath != wantPath {
		t.Errorf("queue item = %#v", it)
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Errorf("OnProgress last = %d/%d, want %d/%d", lastDone, lastTotal, len(body), len(body))
	}
}

func TestNextHTTP404IsFailed(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer ts.Close()

	q := mustQueue(t)
	if err := q.Add(Item{URL: ts.URL + "/missing.mp3"}); err != nil {
		t.Fatal(err)
	}

	d := &Downloader{Dir: t.TempDir()}
	item, err := d.Next(context.Background(), q)
	if err == nil {
		t.Fatal("Next on 404: want error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention the HTTP status: %v", err)
	}
	if item.Status != StatusFailed {
		t.Errorf("returned item status = %v, want failed", item.Status)
	}
	it := findItem(t, q, ts.URL+"/missing.mp3")
	if it.Status != StatusFailed || !strings.Contains(it.Error, "404") {
		t.Errorf("queue item = %#v, want failed with status in Error", it)
	}
}

// TestNextCancelThenResume: canceling ctx mid-download keeps the .part
// file and the Downloading status; the next Next() run sends a Range
// header matching the partial size and completes the file.
func TestNextCancelThenResume(t *testing.T) {
	t.Parallel()

	const full = "0123456789abcdef"
	var partial = make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rng := r.Header.Get("Range"); rng != "" {
			// Second (resumed) request: serve the remainder.
			start := parseRangeStart(t, rng)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(full)-1, len(full)))
			w.Header().Set("Content-Length", strconv.Itoa(len(full)-int(start)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(full[start:]))
			return
		}
		// First request: write half, flush, then hang until canceled.
		w.Header().Set("Content-Length", strconv.Itoa(len(full)))
		_, _ = w.Write([]byte(full[:8]))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-partial
	}))
	defer ts.Close()

	q := mustQueue(t)
	if err := q.Add(Item{URL: ts.URL + "/ep.mp3"}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	d := &Downloader{Dir: dir}
	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)
	itemc := make(chan Item, 1)
	go func() {
		it, err := d.Next(ctx, q)
		itemc <- it
		errc <- err
	}()

	// Wait until half the body hit the .part file, then cancel.
	partPath := filepath.Join(dir, "ep.mp3.part")
	waitFor(t, 5*time.Second, func() bool {
		b, err := os.ReadFile(partPath)
		return err == nil && len(b) == 8
	})
	cancel()

	var item Item
	select {
	case item = <-itemc:
	case <-time.After(5 * time.Second):
		t.Fatal("Next did not return after cancel")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	// Cancel leaves .part (with the partial content) and status Downloading.
	if b, err := os.ReadFile(partPath); err != nil || string(b) != full[:8] {
		t.Errorf(".part = %q, %v; want %q (resume artifact)", b, err, full[:8])
	}
	if item.Status != StatusDownloading {
		t.Errorf("status after cancel = %v, want downloading", item.Status)
	}
	if it := findItem(t, q, ts.URL+"/ep.mp3"); it.Status != StatusDownloading {
		t.Errorf("queue status after cancel = %v, want downloading", it.Status)
	}

	// Next run: server honors Range, download resumes and completes.
	close(partial)
	resumed, err := d.Next(context.Background(), q)
	if err != nil {
		t.Fatalf("resumed Next: %v", err)
	}
	if resumed.Status != StatusDone {
		t.Fatalf("resumed item status = %v, want done", resumed.Status)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ep.mp3"))
	if err != nil || string(got) != full {
		t.Errorf("resumed file = %q, %v; want %q", got, err, full)
	}
	if _, err := os.Stat(partPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".part still exists after resume: %v", err)
	}
}

// TestNextResumeServerIgnoresRange: when a stale .part exists but the
// server answers 200 with the full body, the temp file is restarted
// (not appended to).
func TestNextResumeServerIgnoresRange(t *testing.T) {
	t.Parallel()

	const full = "the full response body"
	var gotRange string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Length", strconv.Itoa(len(full)))
		_, _ = w.Write([]byte(full)) // 200, full body, Range ignored
	}))
	defer ts.Close()

	q := mustQueue(t)
	if err := q.Add(Item{URL: ts.URL + "/ep.mp3"}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	// Stale partial download from a previous run.
	if err := os.WriteFile(filepath.Join(dir, "ep.mp3.part"), []byte("stale partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Downloader{Dir: dir}
	if _, err := d.Next(context.Background(), q); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if gotRange != "bytes=13-" {
		t.Errorf("Range header = %q, want %q", gotRange, "bytes=13-")
	}
	got, err := os.ReadFile(filepath.Join(dir, "ep.mp3"))
	if err != nil || string(got) != full {
		t.Errorf("file = %q, %v; want %q (restarted, not appended)", got, err, full)
	}
}

func TestFilenameCollisionSuffixes(t *testing.T) {
	t.Parallel()

	const body = "same name everywhere"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	q := mustQueue(t)
	// A pre-existing file claims the first name.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ep.mp3"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Two queued items with the same basename but different URLs.
	if err := q.Add(Item{URL: ts.URL + "/a/ep.mp3?token=1"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Add(Item{URL: ts.URL + "/b/ep.mp3?token=2"}); err != nil {
		t.Fatal(err)
	}

	d := &Downloader{Dir: dir}
	first, err := d.Next(context.Background(), q)
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	second, err := d.Next(context.Background(), q)
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}

	if got := filepath.Base(first.FilePath); got != "ep (2).mp3" {
		t.Errorf("first file = %q, want %q (query string stripped, collision suffixed)", got, "ep (2).mp3")
	}
	if got := filepath.Base(second.FilePath); got != "ep (3).mp3" {
		t.Errorf("second file = %q, want %q", got, "ep (3).mp3")
	}
	for _, p := range []string{first.FilePath, second.FilePath} {
		if b, err := os.ReadFile(p); err != nil || string(b) != body {
			t.Errorf("%s content = %q, %v", p, b, err)
		}
	}
	if _, err := d.Next(context.Background(), q); !errors.Is(err, ErrQueueEmpty) {
		t.Errorf("drained queue: err = %v, want ErrQueueEmpty", err)
	}
}

func TestFilenameFromURL(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://example.com/ep/episode%2042.mp3": "episode 42.mp3",
		"https://example.com/ep.mp3?query=1&x=2":  "ep.mp3",
		"https://example.com/":                    "download",
		"https://example.com":                     "download",
		"https://example.com/a<b>c.mp3":           "a_b_c.mp3",
		"not a url at all":                        "not a url at all", // parses as a relative path
	}
	for raw, want := range cases {
		if got := filenameFromURL(raw); got != want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestNextPicksDownloadingItem: an item left in Downloading (interrupted
// run) is picked up by Next, not skipped forever.
func TestNextPicksDownloadingItem(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer ts.Close()

	q := mustQueue(t)
	if err := q.Add(Item{URL: ts.URL + "/a.mp3"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Add(Item{URL: ts.URL + "/b.mp3"}); err != nil {
		t.Fatal(err)
	}
	// Simulate an interrupted run on the first item.
	if err := q.MarkDownloading(ts.URL + "/a.mp3"); err != nil {
		t.Fatal(err)
	}

	d := &Downloader{Dir: t.TempDir()}
	item, err := d.Next(context.Background(), q)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if item.URL != ts.URL+"/a.mp3" {
		t.Errorf("Next picked %q, want the interrupted item", item.URL)
	}
}

func parseRangeStart(t *testing.T, rng string) int64 {
	t.Helper()
	var start int64
	if _, err := fmt.Sscanf(rng, "bytes=%d-", &start); err != nil {
		t.Fatalf("bad Range header %q: %v", rng, err)
	}
	return start
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
