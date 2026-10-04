package tui

import (
	"html"
	"strings"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// saveArticleText renders an article as plain text for the save-article
// operation: title, author, date, source URL, then the content with HTML
// stripped. This is lossy text extraction, not rendering — chawan does
// all real rendering.
func saveArticleText(a feed.Article, feedTitle string) string {
	var b strings.Builder
	if a.Title != "" {
		b.WriteString("Title: " + a.Title + "\n")
	}
	if feedTitle != "" {
		b.WriteString("Feed: " + feedTitle + "\n")
	}
	if a.Author != "" {
		b.WriteString("Author: " + a.Author + "\n")
	}
	if !a.Published.IsZero() {
		b.WriteString("Date: " + a.Published.Format("2006-01-02 15:04") + "\n")
	}
	if a.URL != "" {
		b.WriteString("Link: " + a.URL + "\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	text := stripHTML(a.ContentHTML)
	if strings.TrimSpace(text) == "" {
		text = "(no content)"
	}
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// blockTags are tags that structurally imply a line break in the text
// output; everything else is dropped without a trace.
var blockTags = map[string]bool{
	"p": true, "br": true, "li": true, "div": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "dl": true, "dt": true, "dd": true,
	"table": true, "tr": true, "pre": true, "hr": true, "section": true,
	"article": true, "header": true, "footer": true,
}

// stripHTML removes HTML markup and unescapes entities, turning
// structural tags into newlines. It is a deliberately minimal tokenizer
// (tags, comments, entities), not a parser: attribute quoting is
// respected only far enough to find the end of a tag, script/style
// bodies are dropped entirely. Feeds carry real-world soup; anything
// this mis-parses degrades to visible text, never to a panic.
func stripHTML(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		switch c := s[i]; c {
		case '<':
			tagEnd := scanTagEnd(s, i)
			if tagEnd < 0 { // unterminated tag: treat the rest as text
				out.WriteString(html.UnescapeString(s[i+1:]))
				i = len(s)
				continue
			}
			name, closing := tagName(s[i+1 : tagEnd])
			switch {
			case !closing && (name == "script" || name == "style"):
				i = skipToClose(s, tagEnd+1, name)
			case !closing && blockTags[name]:
				// Only opening tags break lines: emitting on both ends
				// would double every paragraph break.
				out.WriteByte('\n')
				i = tagEnd + 1
			default:
				i = tagEnd + 1
			}

		case '&':
			j := strings.IndexByte(s[i:], ';')
			if j > 0 && j <= 10 && entityOK(s[i:i+j+1]) {
				out.WriteString(html.UnescapeString(s[i : i+j+1]))
				i += j + 1
			} else {
				out.WriteByte('&')
				i++
			}

		default:
			out.WriteByte(c)
			i++
		}
	}
	return collapseBlankLines(out.String())
}

// scanTagEnd returns the index of the '>' closing the tag that starts at
// i ('<'), honoring quoted attribute values; -1 when unterminated.
func scanTagEnd(s string, i int) int {
	var quote byte
	for j := i + 1; j < len(s); j++ {
		switch c := s[j]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j
		}
	}
	return -1
}

// tagName extracts the lowercased element name from a tag body (the text
// between '<' and '>'), reporting whether the tag is a closing one.
func tagName(body string) (string, bool) {
	closing := strings.HasPrefix(body, "/")
	if closing {
		body = body[1:]
	}
	end := 0
	for end < len(body) && isNameByte(body[end]) {
		end++
	}
	return strings.ToLower(body[:end]), closing
}

// skipToClose advances past the body of a raw-text element, returning the
// index just after its closing tag (or len(s)).
func skipToClose(s string, from int, elem string) int {
	lower := strings.ToLower(s)
	closeTag := "</" + elem
	i := strings.Index(lower[from:], closeTag)
	if i < 0 {
		return len(s)
	}
	i += from
	if end := strings.IndexByte(s[i:], '>'); end >= 0 {
		return i + end + 1
	}
	return len(s)
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// entityOK reports whether s looks like an entity reference (starts with
// '&', ends with ';', no intervening whitespace) so UnescapeString gets
// bounded input instead of the whole remainder.
func entityOK(s string) bool {
	if len(s) < 3 || s[0] != '&' || s[len(s)-1] != ';' {
		return false
	}
	return !strings.ContainsAny(s[1:len(s)-1], " \t\n\r<")
}

// collapseBlankLines trims trailing spaces per line and collapses runs of
// 3+ newlines down to one blank line, trimming the outer edges.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}
