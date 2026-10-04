package urls

import "strings"

// Format renders a Feed as a single urls-file line:
//
//	URL "Title" tag1 tag2 "key: value"
//
// The title and per-feed config pairs are double-quoted; a URL is quoted
// only if it contains whitespace. Format roundtrips with Parse for the
// URL, Title, Tags and Extra fields (Line is source metadata and is not
// preserved). Like newsboat's format, values containing double quotes do
// not roundtrip — the urls format has no escape mechanism.
func Format(f Feed) string {
	var b strings.Builder

	url := f.URL
	if strings.ContainsAny(url, " \t") {
		url = quote(url)
	}
	b.WriteString(url)

	if f.Title != "" {
		b.WriteByte(' ')
		b.WriteString(quote(f.Title))
	}
	for _, tag := range f.Tags {
		b.WriteByte(' ')
		b.WriteString(tag)
	}
	for _, extra := range f.Extra {
		b.WriteByte(' ')
		b.WriteString(quote(extra))
	}
	return b.String()
}

// quote wraps s in double quotes the way the urls file represents
// titles and config pairs (see splitFields/unquote in urls.go).
func quote(s string) string {
	return `"` + s + `"`
}
