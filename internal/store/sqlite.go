// SQLite plumbing: driver setup, DSN pragmas, schema, row codecs.

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver; pure Go, no cgo

	"github.com/71g3pf4c3/charss/internal/feed"
)

const (
	dbFileName = "charss.db"

	// DSN pragmas, applied to every pooled connection (busy_timeout and
	// foreign_keys are per-connection; journal_mode is persistent but
	// harmless to re-apply).
	busyTimeoutMS = 5000

	// Concurrency: WAL + busy_timeout absorbs most contention; these are
	// the belt-and-suspenders retry bounds for SQLITE_BUSY.
	maxBusyAttempts = 3
	busyRetryDelay  = 25 * time.Millisecond
)

const articleSelect = `
SELECT id, guid, title, link, author, content_html, published, read, enclosures, flags
FROM articles
WHERE feed_url = ?
ORDER BY published DESC, rowid ASC
` // NULL published (zero time) sorts last; rowid preserves saved order on ties.

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		path, busyTimeoutMS)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init schema in %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: chmod %s: %w", path, err)
	}
	// Best effort: wal/shm sidecars from a previous run may carry looser
	// bits. New sidecars are created with the database file's mode.
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Chmod(path+suffix, 0o600)
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	// NB: feeds.url holds feedKey(feedURL) — hex(sha256(url)) — not the
	// plaintext URL. See the package comment.
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS feeds (
			url           TEXT PRIMARY KEY,
			etag          TEXT,
			last_modified TEXT,
			last_fetched  TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS articles (
			id           TEXT NOT NULL,
			feed_url     TEXT NOT NULL,
			guid         TEXT,
			title        TEXT,
			link         TEXT,
			author       TEXT,
			content_html TEXT,
			published    TIMESTAMP,
			read         INTEGER NOT NULL DEFAULT 0,
			enclosures   TEXT, -- JSON-encoded []feed.Enclosure
			flags        TEXT, -- newsboat-style flag chars, e.g. "aZ"
			PRIMARY KEY (feed_url, id),
			FOREIGN KEY (feed_url) REFERENCES feeds(url) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		)`,
		`INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '2')`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("%q: %w", firstLine(q), err)
		}
	}
	// Databases created before flags existed (schema_version 1) lack the
	// column; CREATE TABLE IF NOT EXISTS does not add it. The ALTER is a
	// no-op failure on fresh databases ("duplicate column name").
	if _, err := db.Exec(`ALTER TABLE articles ADD COLUMN flags TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("add flags column: %w", err)
		}
	}
	return nil
}

// saveState writes a full feed state inside a transaction: upsert feed
// meta, then replace the feed's article rows. The caller owns the
// transaction (commit/rollback), so Save and the JSON import share this.
func saveState(tx *sql.Tx, key string, st State) error {
	if _, err := tx.Exec(`
		INSERT INTO feeds (url, etag, last_modified, last_fetched)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (url) DO UPDATE SET
			etag = excluded.etag,
			last_modified = excluded.last_modified,
			last_fetched = excluded.last_fetched`,
		key, st.ETag, st.LastModified, formatTime(st.LastFetched)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM articles WHERE feed_url = ?`, key); err != nil {
		return err
	}
	for _, a := range st.Articles {
		enclosures, err := encodeEnclosures(a.Enclosures)
		if err != nil {
			return fmt.Errorf("article %s: %w", a.ID, err)
		}
		read := 0
		if st.Read[a.ID] {
			read = 1
		}
		var flags any
		if f := st.Flags[a.ID]; f != "" {
			flags = f
		}
		if _, err := tx.Exec(`
			INSERT INTO articles
				(id, feed_url, guid, title, link, author, content_html, published, read, enclosures, flags)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, key, a.GUID, a.Title, a.URL, a.Author, a.ContentHTML,
			formatTime(a.Published), read, enclosures, flags); err != nil {
			return err
		}
	}
	return nil
}

// formatTime returns the storage form of t: NULL for the zero time (so it
// round-trips to the exact zero value), RFC3339Nano text otherwise.
func formatTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}

func parseTime(col sql.NullString) (time.Time, error) {
	if !col.Valid || col.String == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, col.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", col.String, err)
	}
	return t, nil
}

func encodeEnclosures(enc []feed.Enclosure) (any, error) {
	if len(enc) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(enc)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func decodeEnclosures(col sql.NullString) ([]feed.Enclosure, error) {
	if !col.Valid || col.String == "" {
		return nil, nil
	}
	var enc []feed.Enclosure
	if err := json.Unmarshal([]byte(col.String), &enc); err != nil {
		return nil, fmt.Errorf("decode enclosures %q: %w", col.String, err)
	}
	return enc, nil
}

// isBusy reports whether err is SQLITE_BUSY ("database is locked"). The
// modernc driver surfaces it as e.g. "database is locked (5) (SQLITE_BUSY)".
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
