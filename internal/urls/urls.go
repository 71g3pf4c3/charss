// Package urls parses the newsboat-compatible urls file.
//
// Format (subset of newsboat's, see man newsboat(1)):
//
//	https://example.com/feed.xml "Feed Title" tag1 tag2
//	"https://example.com/quoted url.xml" "Quoted"
//	# comment line — skipped
//
// The first field is the feed URL (optionally double-quoted if it contains
// spaces). An optional double-quoted title follows. Remaining fields are tags.
// newsboat >= 2.34 also allows quoted per-feed config pairs ("key: value")
// after the tags; those are preserved in Feed.Extra.
package urls

import (
	"fmt"
	"strings"
)

// Feed is one entry of the urls file.
type Feed struct {
	URL   string
	Title string   // optional; empty if not set
	Tags  []string // optional
	Extra []string // raw per-feed config pairs ("key: value"), newsboat >= 2.34
	Line  int      // 1-based source line number, for error reporting
}

// Parse parses urls-file content. Comment lines (first non-space char '#')
// and blank lines are skipped.
func Parse(content string) ([]Feed, error) {
	var feeds []Feed
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields, err := splitFields(line)
		if err != nil {
			return nil, fmt.Errorf("urls line %d: %w", i+1, err)
		}
		if len(fields) == 0 {
			continue
		}
		f := Feed{URL: unquote(fields[0]), Line: i + 1}
		rest := fields[1:]
		if len(rest) > 0 && isQuoted(rest[0]) {
			f.Title = unquote(rest[0])
			rest = rest[1:]
		}
		for _, field := range rest {
			if isQuoted(field) {
				f.Extra = append(f.Extra, unquote(field))
				continue
			}
			f.Tags = append(f.Tags, field)
		}
		if f.URL == "" {
			return nil, fmt.Errorf("urls line %d: empty feed URL", i+1)
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

// splitFields splits a line into space-separated fields, honoring
// double-quoted segments (quotes are kept on the field so the caller can
// distinguish a quoted title from a tag).
func splitFields(line string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			fields = append(fields, cur.String())
			cur.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return fields, nil
}

func isQuoted(field string) bool {
	return len(field) >= 2 && strings.HasPrefix(field, `"`) && strings.HasSuffix(field, `"`)
}

func unquote(s string) string {
	if isQuoted(s) {
		return s[1 : len(s)-1]
	}
	return s
}
