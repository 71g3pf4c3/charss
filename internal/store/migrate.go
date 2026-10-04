// One-time migration from the legacy JSON-per-feed cache.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/71g3pf4c3/charss/internal/feed"
)

const (
	metaMigrated    = "migrated"
	legacyDirName   = "feeds"
	legacyBackupFmt = "feeds.migrated-backup" // legacyDirName + ".migrated-backup"
)

// migrateFromJSON imports <dir>/feeds/*.json into the database once, then
// renames the directory to feeds.migrated-backup. Idempotent: once the
// meta row "migrated" is set (in the same transaction as the import),
// later opens are a no-op even if a feeds directory reappears.
//
// On import failure the JSON cache is left untouched and the partial
// database file is removed, so the next start retries from a clean slate
// (Open fails hard in the meantime).
func (s *Store) migrateFromJSON(dbPath string) error {
	migrated, err := metaGet(s.db, metaMigrated)
	if err != nil {
		return fmt.Errorf("store: read migration marker: %w", err)
	}
	if migrated != "" {
		return nil
	}

	legacyDir := filepath.Join(s.dir, legacyDirName)
	files, err := filepath.Glob(filepath.Join(legacyDir, "*.json"))
	if err != nil {
		return fmt.Errorf("store: scan %s: %w", legacyDir, err)
	}
	if len(files) == 0 {
		return nil // no legacy cache (or already renamed away) — nothing to do
	}

	if err := s.importLegacy(files); err != nil {
		// Leave the JSON cache intact, wipe the partial database.
		s.db.Close()
		removeDBFiles(dbPath)
		return fmt.Errorf("store: migrate JSON cache in %s: %w", legacyDir, err)
	}

	backup := filepath.Join(s.dir, legacyBackupFmt)
	if err := os.Rename(legacyDir, backup); err != nil {
		return fmt.Errorf("store: rename %s -> %s after migration: %w", legacyDir, backup, err)
	}
	return nil
}

// importLegacy reads every legacy JSON file and writes it as feed state in
// one transaction, setting the migration marker in the same transaction.
// A legacy filename (minus .json) is the feed key, matching feedKey.
func (s *Store) importLegacy(files []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a successful commit

	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var st State
		if err := json.Unmarshal(b, &st); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		// Legacy files could contain duplicate article IDs (older merge
		// versions were not strict); the articles PK is, so keep the
		// first (newest-first files) occurrence.
		st.Articles = dedupeArticles(st.Articles)
		key := strings.TrimSuffix(filepath.Base(path), ".json")
		if err := saveState(tx, key, st); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	if err := metaSet(tx, metaMigrated, "1"); err != nil {
		return err
	}
	return tx.Commit()
}

func dedupeArticles(articles []feed.Article) []feed.Article {
	if len(articles) < 2 {
		return articles
	}
	seen := make(map[string]struct{}, len(articles))
	out := make([]feed.Article, 0, len(articles))
	for _, a := range articles {
		if _, dup := seen[a.ID]; dup {
			continue
		}
		seen[a.ID] = struct{}{}
		out = append(out, a)
	}
	return out
}

func metaGet(db *sql.DB, key string) (string, error) {
	var v sql.NullString
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v.String, nil
}

func metaSet(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`
		INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// removeDBFiles deletes the database and its WAL/SHM sidecars, ignoring
// absent files.
func removeDBFiles(dbPath string) {
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(p)
	}
}
