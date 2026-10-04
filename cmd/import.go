package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/71g3pf4c3/charss/internal/config"
	"github.com/71g3pf4c3/charss/internal/opml"
	"github.com/71g3pf4c3/charss/internal/urls"
)

var importReplace bool

// importCmd merges an OPML subscription list into the urls file
// (newsboat's `import` equivalent). --replace rewrites the file from the
// OPML instead of merging.
var importCmd = &cobra.Command{
	Use:   "import <file.opml>",
	Short: "Import feeds from an OPML file into the urls file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFlag, urlsFlag)
		if err != nil {
			return err
		}
		return runImport(cfg, args[0])
	},
}

// exportCmd writes the urls file out as OPML (newsboat's `export`
// equivalent). The target is an explicit argument and is overwritten.
var exportCmd = &cobra.Command{
	Use:   "export <file.opml>",
	Short: "Export feeds from the urls file to an OPML file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFlag, urlsFlag)
		if err != nil {
			return err
		}
		return runExport(cfg, args[0])
	},
}

func init() {
	importCmd.Flags().BoolVar(&importReplace, "replace", false,
		"rewrite the urls file from the OPML instead of merging")
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(exportCmd)
}

func runImport(cfg *config.Config, opmlPath string) error {
	data, err := os.ReadFile(opmlPath)
	if err != nil {
		return fmt.Errorf("reading OPML file: %w", err)
	}
	imported, err := opml.Parse(data)
	if err != nil {
		return err
	}

	// Same pattern as the TUI loader: a missing urls file starts empty,
	// a malformed one is a hard error.
	raw, err := os.ReadFile(cfg.URLsFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading urls file: %w", err)
	}
	existing, err := urls.Parse(string(raw))
	if err != nil {
		return err
	}

	if importReplace {
		out := new(strings.Builder)
		for _, f := range imported {
			out.WriteString(urls.Format(f))
			out.WriteByte('\n')
		}
		if err := writeFileAtomic(cfg.URLsFile, []byte(out.String())); err != nil {
			return err
		}
		fmt.Printf("wrote %d feeds to %s (replaced %d existing)\n",
			len(imported), cfg.URLsFile, len(existing))
		return nil
	}

	added, skipped := MergeFeeds(existing, imported)
	if len(added) == 0 {
		fmt.Printf("imported 0, skipped %d (already present)\n", skipped)
		return nil
	}

	// Append to the existing content so comments and formatting in the
	// urls file are preserved.
	out := new(strings.Builder)
	if len(raw) > 0 {
		out.Write(raw)
		if !strings.HasSuffix(string(raw), "\n") {
			out.WriteByte('\n')
		}
	}
	for _, f := range added {
		out.WriteString(urls.Format(f))
		out.WriteByte('\n')
	}
	if err := writeFileAtomic(cfg.URLsFile, []byte(out.String())); err != nil {
		return err
	}
	fmt.Printf("imported %d, skipped %d (already present)\n", len(added), skipped)
	return nil
}

// MergeFeeds returns the subset of imported feeds whose URL is not already
// present in existing, plus how many were skipped as duplicates (present
// in existing, or repeated within imported).
func MergeFeeds(existing, imported []urls.Feed) (added []urls.Feed, skipped int) {
	seen := make(map[string]bool, len(existing)+len(imported))
	for _, f := range existing {
		seen[f.URL] = true
	}
	for _, f := range imported {
		if seen[f.URL] {
			skipped++
			continue
		}
		seen[f.URL] = true
		added = append(added, f)
	}
	return added, skipped
}

func runExport(cfg *config.Config, opmlPath string) error {
	if samePath(opmlPath, cfg.URLsFile) {
		return fmt.Errorf("refusing to export onto the urls file itself (%s)", cfg.URLsFile)
	}

	// Same pattern as the TUI loader.
	feeds, _, err := loadFeeds(cfg.URLsFile)
	if err != nil {
		return err
	}

	data, err := opml.Export(feeds)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(opmlPath, data); err != nil {
		return err
	}
	fmt.Printf("exported %d feeds to %s\n", len(feeds), opmlPath)
	return nil
}

// writeFileAtomic writes data to path via a temp file in the same
// directory followed by rename. It preserves the mode of an existing file.
func writeFileAtomic(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".charss-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming into place: %w", err)
	}
	return nil
}

// samePath reports whether a and b refer to the same file: equal absolute
// paths, or the same inode via os.SameFile (symlinks/hardlinks).
func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && absA == absB {
		return true
	}
	fiA, errA := os.Stat(a)
	fiB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fiA, fiB)
}
