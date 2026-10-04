// Package search implements newsboat-style full-text search over cached
// articles.
//
// Base semantics follow newsboat: matching is case-insensitive substring
// matching over the article title and content HTML, and space-separated
// terms are combined with AND — every term must appear somewhere in the
// article (in the title or in the content, independently per term).
//
// charss extensions on top of plain terms (these are NOT newsboat
// syntax):
//
//	-term          exclusion: the article must NOT contain this term
//	"quoted text"  exact phrase: the article must contain this substring
//	/re/           Go regular expression, compiled with the (?i)
//	               case-insensitive flag and matched against the title
//	               and the content independently
//
// Escapes are NOT supported: a backslash is an ordinary character
// everywhere, a double quote always ends a quoted phrase, and a slash
// always ends a regex. To put a slash inside a plain term, quote the
// term ("a/b"); to match a literal quote, use a regex (/"/).
//
// The empty query matches nothing.
package search

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A Query is a parsed search query. Build it with Parse; hand-constructed
// Queries are allowed too, but the zero Query (and any Query whose
// criteria are all empty) matches nothing.
type Query struct {
	Terms   []string         // plain terms (all must match)
	Exclude []string         // -term (none may match)
	Phrases []string         // exact phrases (all must match)
	Regexps []*regexp.Regexp // /regex/ (all must match)
}

// A SyntaxError is a malformed search query. Pos is the byte offset into
// the original input where the offending token starts (for unterminated
// quotes and regexes: the position of the opening delimiter).
type SyntaxError struct {
	Msg string
	Pos int
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("search: %s at position %d", e.Msg, e.Pos)
}

// Parse builds a Query from raw user input.
//
// Grammar (whitespace separates tokens; a plain term ends at whitespace,
// a double quote, or a slash; a leading '-' makes the rest of the token
// an exclusion):
//
//	query  := token (ws token)*
//	token  := phrase | regex | exclusion | term
//	phrase := '"' non-quote-chars '"'
//	regex  := '/' non-slash-chars '/'
//	excl   := '-' term-chars
//	term   := chars other than ws, '"', '/'
//
// Syntax errors (unterminated quote, unterminated /.../, empty quote or
// regex, invalid regex, '-' followed by a quote or regex) return a
// *SyntaxError.
func Parse(input string) (*Query, error) {
	q := &Query{}
	i := 0
	for i < len(input) {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		switch input[i] {
		case '"':
			end := strings.IndexByte(input[i+1:], '"')
			if end < 0 {
				return nil, &SyntaxError{Msg: "unterminated quoted phrase", Pos: i}
			}
			if end == 0 {
				return nil, &SyntaxError{Msg: "empty quoted phrase", Pos: i}
			}
			q.Phrases = append(q.Phrases, input[i+1:i+1+end])
			i += 1 + end + 1
		case '/':
			end := strings.IndexByte(input[i+1:], '/')
			if end < 0 {
				return nil, &SyntaxError{Msg: "unterminated regex", Pos: i}
			}
			if end == 0 {
				return nil, &SyntaxError{Msg: "empty regex", Pos: i}
			}
			re, err := regexp.Compile("(?i)" + input[i+1:i+1+end])
			if err != nil {
				return nil, &SyntaxError{Msg: "invalid regex: " + err.Error(), Pos: i}
			}
			q.Regexps = append(q.Regexps, re)
			i += 1 + end + 1
		case '-':
			// An exclusion is '-' followed by a plain term. A lone '-'
			// (end of input or followed by whitespace) is a plain term.
			if i+1 >= len(input) {
				q.Terms = append(q.Terms, "-")
				i++
				continue
			}
			switch input[i+1] {
			case '"':
				return nil, &SyntaxError{Msg: "exclusion must be a plain term, not a quoted phrase", Pos: i}
			case '/':
				return nil, &SyntaxError{Msg: "exclusion must be a plain term, not a regex", Pos: i}
			}
			term, next := scanTerm(input, i+1)
			if term == "" {
				q.Terms = append(q.Terms, "-")
				i++
				continue
			}
			q.Exclude = append(q.Exclude, term)
			i = next
		default:
			term, next := scanTerm(input, i)
			q.Terms = append(q.Terms, term)
			i = next
		}
	}
	return q, nil
}

// scanTerm scans a plain term starting at input[i], which must not be
// whitespace, and returns the term and the offset just past it. A term
// ends at whitespace, a double quote, or a slash.
func scanTerm(input string, i int) (string, int) {
	start := i
	for i < len(input) {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) || r == '"' || r == '/' {
			break
		}
		i += size
	}
	return input[start:i], i
}
