// Package filter implements newsboat's filter expression language as a pure
// parse-and-evaluate library. It is meant for query feeds (urls-file entries
// of the form "query:Name:EXPRESSION"; see ParseQueryFeedURL) and for
// filtering article lists. The package knows nothing about the TUI or
// storage layers: callers adapt their types to Subject (see SubjectFromArticle
// in adapter.go).
//
// # Grammar
//
//	expr       := andexpr ("or" andexpr)*
//	andexpr    := notexpr ("and" notexpr)*
//	notexpr    := "not" notexpr | primary
//	primary    := "(" expr ")" | comparison
//	comparison := attr op value
//	attr       := one of the attribute names listed below
//	op         := "=" | "!=" | "#" | "!#"
//	value      := double-quoted string
//
// "and" binds tighter than "or", "not" binds tightest; parentheses override.
// Keywords (and, or, not) and attribute names are lowercase and
// case-sensitive.
//
// # Operators
//
//	=   exact equality (whole value)
//	!=  exact inequality
//	#   substring containment
//	!#  substring non-containment
//
// # Attributes
//
//	title, link, author, guid, content
//	    Subject fields. link is the article URL; content matches against the
//	    raw HTML of ContentHTML (markup is not stripped — a plain substring
//	    over the HTML source).
//	unread
//	    Compares against "yes" (unread) or "no" (read).
//	date, feeddate
//	    Matched against both the date-only rendering ("2006-01-02") and the
//	    full RFC3339 rendering of the time value; either may satisfy the
//	    operator. Only string equality/containment is supported —
//	    newsboat's "between" date ranges are not implemented. A zero time
//	    behaves as an empty string.
//	tags
//	    A list: "#" and "!#" match if any single element contains the value;
//	    "=" and "!=" compare against the elements joined with single spaces.
//	    An empty list therefore satisfies "=" "" and never satisfies "#".
//	feedtitle, feedlink
//	    Feed-level Subject fields.
//
// # Matching semantics
//
// All string matching is case-insensitive on both sides: values are
// lowercased at compile time, attribute strings at evaluation time. Unlike
// newsboat, this also applies to flags: "A" and "a" are the same flag here.
//
// Values are double-quoted strings without escape processing: a value ends
// at the first '"' and cannot contain a double quote ('\' is an ordinary
// character).
//
// Compile returns *SyntaxError for malformed input; Pos is the 1-based byte
// offset of the offending token (line is always 1). A compiled Filter is
// immutable and safe for concurrent Eval.
package filter

import (
	"fmt"
	"strings"
	"time"
)

// dateOnlyLayout is the short date rendering matched by date and feeddate.
const dateOnlyLayout = "2006-01-02"

// Filter is a compiled filter expression. Compile once, Eval many: a Filter
// is immutable and safe for concurrent use. The zero Filter (and a nil one)
// matches nothing.
type Filter struct {
	root node
}

// Compile parses a filter expression. It returns a *SyntaxError with 1-based
// position info for malformed input.
func Compile(src string) (*Filter, error) {
	root, err := parse(src)
	if err != nil {
		return nil, err
	}
	return &Filter{root: root}, nil
}

// Eval reports whether the article described by s matches the compiled
// expression. It is safe for concurrent use.
func (f *Filter) Eval(s Subject) bool {
	if f == nil || f.root == nil {
		return false
	}
	return f.root.eval(s)
}

// Subject decouples evaluation from storage types. Adapters for concrete
// storage types live with the caller (see SubjectFromArticle).
type Subject struct {
	Title, Link, Author, GUID, ContentHTML string
	Unread                                 bool
	Published                              time.Time
	Tags                                   []string
	Flags                                  string // newsboat flags are a string of flag chars, e.g. "aZ"
	FeedTitle, FeedLink                    string
	FeedDate                               time.Time
}

// SyntaxError describes a malformed filter expression. Pos is the 1-based
// byte offset of the offending token within the source; the line is always 1
// (expressions are single-line). Token holds the offending token's text, or
// "end of input" where the error is caused by the expression ending.
type SyntaxError struct {
	Msg   string
	Token string
	Pos   int
}

func (e *SyntaxError) Error() string {
	if e.Token == "" {
		return fmt.Sprintf("filter: line 1, pos %d: %s", e.Pos, e.Msg)
	}
	return fmt.Sprintf("filter: line 1, pos %d: %s (near %q)", e.Pos, e.Msg, e.Token)
}

// ParseQueryFeedURL splits a urls-file entry of the form
// "query:NAME:EXPRESSION" and returns the feed name and the filter
// expression. It reports ok=false for entries that are not query feeds, or
// where the expression is missing or empty. NAME may itself be empty
// ("query::expr"). The "query:" prefix is case-sensitive; the expression may
// contain colons.
func ParseQueryFeedURL(url string) (name, expr string, ok bool) {
	const prefix = "query:"
	if !strings.HasPrefix(url, prefix) {
		return "", "", false
	}
	rest := url[len(prefix):]
	i := strings.Index(rest, ":")
	if i < 0 {
		return "", "", false
	}
	name, expr = rest[:i], rest[i+1:]
	if expr == "" {
		return "", "", false
	}
	return name, expr, true
}
