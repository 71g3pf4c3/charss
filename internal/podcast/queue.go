// Package podcast implements the persistent podcast download queue and a
// sequential downloader for enclosure URLs (newsboat's queue/podbeuter
// engine, minus the UI).
//
// The queue is a JSON file at $XDG_CACHE_HOME/charss/queue.json (opened
// with an explicit dir in tests/tools). Every mutation reloads the file,
// applies the change and rewrites it atomically (temp file + rename):
// simple and safe against stale in-memory state across processes, at the
// cost of one rewrite per transition.
package podcast

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status is the lifecycle state of a queue item.
type Status int

const (
	StatusQueued Status = iota // zero value: a fresh Item is queued
	StatusDownloading
	StatusDone
	StatusFailed
	StatusCancelled
)

func (s Status) String() string {
	switch s {
	case StatusQueued:
		return "queued"
	case StatusDownloading:
		return "downloading"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("status(%d)", int(s))
	}
}

// Item is one queued enclosure download. URL is the primary key.
type Item struct {
	URL        string    `json:"url"` // enclosure URL (primary key)
	Title      string    `json:"title,omitempty"`
	FeedURL    string    `json:"feed_url,omitempty"`
	MimeType   string    `json:"mime_type,omitempty"`
	Size       int64     `json:"size"` // expected size, -1 unknown
	Status     Status    `json:"status"`
	FilePath   string    `json:"file_path,omitempty"` // absolute path once downloaded
	Error      string    `json:"error,omitempty"`     // last error, if Failed
	AddedAt    time.Time `json:"added_at,omitzero"`
	EnqueuedAt time.Time `json:"enqueued_at,omitzero"`
}

// Queue errors.
var (
	// ErrNotFound is returned when no queue item has the given URL.
	ErrNotFound = errors.New("podcast: queue item not found")
	// ErrQueueEmpty is returned by Downloader.Next when no item is
	// Queued (or resumable Downloading).
	ErrQueueEmpty = errors.New("podcast: download queue is empty")
)

// Queue is the persistent download queue backed by <dir>/queue.json.
// All methods reload from disk before mutating, so a Queue value is just
// a handle to the file and stays correct across processes.
type Queue struct {
	path string
	mu   sync.Mutex // serializes mutations within this process
}

// OpenQueue opens (creating the directory if needed) the queue stored at
// <dir>/queue.json. An empty dir selects the default location,
// $XDG_CACHE_HOME/charss (os.UserCacheDir fallback ~/.cache/charss).
// A missing queue file is an empty queue, not an error; a malformed one is.
func OpenQueue(dir string) (*Queue, error) {
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("podcast: locate cache dir: %w", err)
		}
		dir = filepath.Join(base, "charss")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("podcast: create %s: %w", dir, err)
	}
	q := &Queue{path: filepath.Join(dir, "queue.json")}
	if _, err := q.load(); err != nil {
		return nil, err
	}
	return q, nil
}

// Path returns the queue file location (mainly for diagnostics).
func (q *Queue) Path() string { return q.path }

// Add appends it to the queue. Items are deduplicated by URL: adding a
// URL that is already queued (in any status) is a no-op. AddedAt /
// EnqueuedAt are stamped when zero.
func (q *Queue) Add(it Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.load()
	if err != nil {
		return err
	}
	for _, x := range items {
		if x.URL == it.URL {
			return nil // already queued: dedupe, not an error
		}
	}
	now := time.Now().UTC()
	if it.AddedAt.IsZero() {
		it.AddedAt = now
	}
	if it.EnqueuedAt.IsZero() {
		it.EnqueuedAt = now
	}
	items = append(items, it)
	return q.save(items)
}

// Items returns a snapshot of the queue in stable (insertion) order.
// On a read error it returns nil; OpenQueue has already rejected
// malformed files, so this only happens on concurrent corruption.
func (q *Queue) Items() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.load()
	if err != nil {
		return nil
	}
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

// Remove deletes the item with the given URL, or returns ErrNotFound.
func (q *Queue) Remove(url string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.load()
	if err != nil {
		return err
	}
	for i, x := range items {
		if x.URL == url {
			items = append(items[:i], items[i+1:]...)
			return q.save(items)
		}
	}
	return ErrNotFound
}

// MarkDownloading moves the item into the Downloading state.
func (q *Queue) MarkDownloading(url string) error {
	return q.update(url, func(it *Item) {
		it.Status = StatusDownloading
		it.Error = ""
		it.EnqueuedAt = time.Now().UTC()
	})
}

// MarkDownloaded marks the item Done with its final file path.
func (q *Queue) MarkDownloaded(url, path string) error {
	return q.update(url, func(it *Item) {
		it.Status = StatusDone
		it.FilePath = path
		it.Error = ""
	})
}

// MarkFailed marks the item Failed with the last error message.
func (q *Queue) MarkFailed(url, errMsg string) error {
	return q.update(url, func(it *Item) {
		it.Status = StatusFailed
		it.Error = errMsg
	})
}

func (q *Queue) update(url string, fn func(*Item)) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.load()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].URL == url {
			fn(&items[i])
			return q.save(items)
		}
	}
	return ErrNotFound
}

// load reads the queue file; a missing file is an empty queue.
func (q *Queue) load() ([]Item, error) {
	b, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("podcast: read %s: %w", q.path, err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	var items []Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("podcast: parse %s: %w", q.path, err)
	}
	return items, nil
}

// save atomically rewrites the queue file (temp + rename), so readers
// never observe partial JSON.
func (q *Queue) save(items []Item) error {
	if items == nil {
		items = []Item{} // marshal as [] rather than null
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(items); err != nil {
		return fmt.Errorf("podcast: encode queue: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(q.path), ".queue-*")
	if err != nil {
		return fmt.Errorf("podcast: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("podcast: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("podcast: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, q.path); err != nil {
		return fmt.Errorf("podcast: rename %s -> %s: %w", tmpName, q.path, err)
	}
	return nil
}
