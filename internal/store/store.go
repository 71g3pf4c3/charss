// Package store persists per-feed state (articles, read flags, conditional
// request validators) in a single SQLite database under a cache directory.
//
// The default location is $XDG_CACHE_HOME/charss/ (falling back to
// ~/.cache/charss/), used when Open is called with an empty dir; the
// database file is <dir>/charss.db, mode 0600.
//
// Feeds are keyed by hex(sha256(feedURL)). The legacy JSON-per-feed cache
// named its files by exactly that hash and the files themselves contain no
// feed URL, so the hash — not the plaintext URL — is the only persistent
// feed identity available to a migration. All lookups go through feedKey,
// so callers keep using feed URLs.
//
// On open, a legacy JSON cache found at <dir>/feeds/*.json is imported in
// one transaction and the directory is renamed to feeds.migrated-backup
// (user data is never deleted). A failed import leaves the JSON files
// untouched and removes the partial database, so the next start retries
// from a clean slate.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// State is the persisted per-feed state.
type State struct {
	ETag         string            `json:"etag,omitempty"`
	LastModified string            `json:"last_modified,omitempty"`
	LastFetched  time.Time         `json:"last_fetched"`
	Articles     []feed.Article    `json:"articles"`
	Read         map[string]bool   `json:"read"`            // article ID -> read
	Flags        map[string]string `json:"flags,omitempty"` // article ID -> flag chars, e.g. "aZ" ("" = no flags)
}

// Store is a SQLite-backed store rooted at a fixed directory.
type Store struct {
	db  *sql.DB
	dir string
}

// Open creates (if needed) and returns a store rooted at dir. An empty dir
// selects the default cache location, $XDG_CACHE_HOME/charss/
// (or ~/.cache/charss/). If a legacy JSON cache exists at <dir>/feeds/,
// it is imported once (see the package comment).
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
	dbPath := filepath.Join(dir, dbFileName)
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dir: dir}
	if err := s.migrateFromJSON(dbPath); err != nil {
		db.Close() // no-op if migrateFromJSON already closed on hard failure
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle. It is safe to call
// multiple times. The zero-value-free API used by the TUI (Open/Load/Save)
// does not require closing; the OS reclaims everything at exit.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func defaultDir() (string, error) {
	base, err := os.UserCacheDir() // $XDG_CACHE_HOME or ~/.cache
	if err != nil {
		return "", fmt.Errorf("store: locate cache dir: %w", err)
	}
	return filepath.Join(base, "charss"), nil
}

// Load returns the stored state for the feed. The bool is false when no
// state has been saved yet. Articles come back newest first by published
// (zero published last, ties in saved order).
func (s *Store) Load(feedURL string) (State, bool, error) {
	key := feedKey(feedURL)

	var etag, lastModified, lastFetched sql.NullString
	err := s.db.QueryRow(
		`SELECT etag, last_modified, last_fetched FROM feeds WHERE url = ?`, key,
	).Scan(&etag, &lastModified, &lastFetched)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("store: load state for %s: %w", feedURL, err)
	}

	st := State{ETag: etag.String, LastModified: lastModified.String}
	if st.LastFetched, err = parseTime(lastFetched); err != nil {
		return State{}, false, fmt.Errorf("store: load state for %s: %w", feedURL, err)
	}

	rows, err := s.db.Query(articleSelect, key)
	if err != nil {
		return State{}, false, fmt.Errorf("store: load articles for %s: %w", feedURL, err)
	}
	defer rows.Close()

	articles := make([]feed.Article, 0, 16)
	read := make(map[string]bool)
	var flags map[string]string
	for rows.Next() {
		var (
			a                  feed.Article
			published, enclCol sql.NullString
			flagsCol           sql.NullString
			isRead             bool
		)
		if err := rows.Scan(
			&a.ID, &a.GUID, &a.Title, &a.URL, &a.Author, &a.ContentHTML,
			&published, &isRead, &enclCol, &flagsCol,
		); err != nil {
			return State{}, false, fmt.Errorf("store: load articles for %s: %w", feedURL, err)
		}
		if a.Published, err = parseTime(published); err != nil {
			return State{}, false, fmt.Errorf("store: load articles for %s: %w", feedURL, err)
		}
		if a.Enclosures, err = decodeEnclosures(enclCol); err != nil {
			return State{}, false, fmt.Errorf("store: load articles for %s: %w", feedURL, err)
		}
		articles = append(articles, a)
		if isRead {
			read[a.ID] = true
		}
		if flagsCol.Valid && flagsCol.String != "" {
			if flags == nil {
				flags = make(map[string]string)
			}
			flags[a.ID] = flagsCol.String
		}
	}
	if err := rows.Err(); err != nil {
		return State{}, false, fmt.Errorf("store: load articles for %s: %w", feedURL, err)
	}

	if len(articles) > 0 {
		st.Articles = articles
	}
	if len(read) > 0 {
		st.Read = read
	}
	if len(flags) > 0 {
		st.Flags = flags
	}
	return st, true, nil
}

// Save atomically replaces the stored state for the feed: article rows are
// rewritten in one transaction, feed meta is upserted. Concurrent saves of
// different feeds are safe (WAL + busy_timeout, plus bounded retries on
// SQLITE_BUSY).
func (s *Store) Save(feedURL string, st State) error {
	key := feedKey(feedURL)
	var err error
	for attempt := 1; ; attempt++ {
		err = s.saveOne(key, st)
		if err == nil || !isBusy(err) || attempt >= maxBusyAttempts {
			break
		}
		time.Sleep(busyRetryDelay)
	}
	if err != nil {
		return fmt.Errorf("store: save state for %s: %w", feedURL, err)
	}
	return nil
}

func (s *Store) saveOne(key string, st State) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := saveState(tx, key, st); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// feedKey is the persistent key for a feed URL: hex(sha256(url)). Legacy
// JSON cache files were named by the same value.
func feedKey(feedURL string) string {
	sum := sha256.Sum256([]byte(feedURL))
	return hex.EncodeToString(sum[:])
}
