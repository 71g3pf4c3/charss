// Package store persists per-feed state (articles, read flags, conditional
// request validators) as one JSON file per feed under a cache directory.
//
// The default location is $XDG_CACHE_HOME/charss/feeds/ (falling back to
// ~/.cache/charss/feeds/), used when Open is called with an empty dir.
// Files are named by the sha256 of the feed URL and written atomically
// (temp file + rename), so readers never observe a partial file.
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// State is the persisted per-feed state.
type State struct {
	ETag         string          `json:"etag,omitempty"`
	LastModified string          `json:"last_modified,omitempty"`
	LastFetched  time.Time       `json:"last_fetched"`
	Articles     []feed.Article  `json:"articles"`
	Read         map[string]bool `json:"read"` // article ID -> read
}

// Store is a file-per-feed JSON store rooted at a fixed directory.
type Store struct {
	dir string
}

// Open creates (if needed) and returns a store rooted at dir. An empty dir
// selects the default cache location, $XDG_CACHE_HOME/charss/feeds/
// (or ~/.cache/charss/feeds/).
func Open(dir string) (*Store, error) {
	if dir == "" {
		var err error
		dir, err = defaultDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

func defaultDir() (string, error) {
	base, err := os.UserCacheDir() // $XDG_CACHE_HOME or ~/.cache
	if err != nil {
		return "", fmt.Errorf("store: locate cache dir: %w", err)
	}
	return filepath.Join(base, "charss", "feeds"), nil
}

// Load returns the stored state for the feed. The bool is false when no
// state has been saved yet.
func (s *Store) Load(feedURL string) (State, bool, error) {
	b, err := os.ReadFile(s.path(feedURL))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("store: read state for %s: %w", feedURL, err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, false, fmt.Errorf("store: decode state for %s: %w", feedURL, err)
	}
	return st, true, nil
}

// Save atomically writes the state for the feed: it marshals to indented
// JSON, writes a temp file in the store directory and renames it over the
// target. A failed save never leaves a partial or temp file behind.
func (s *Store) Save(feedURL string, st State) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep content_html readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(st); err != nil {
		return fmt.Errorf("store: encode state for %s: %w", feedURL, err)
	}
	data := buf.Bytes() // Encode already appends '\n'

	path := s.path(feedURL)
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("store: create temp file in %s: %w", s.dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("store: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("store: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("store: rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}

// path returns the per-feed file path: sha256(feedURL).json.
func (s *Store) path(feedURL string) string {
	sum := sha256.Sum256([]byte(feedURL))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}
