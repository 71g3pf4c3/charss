package filter

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func baseSubject() Subject {
	return Subject{
		Title:       "Hello World",
		Link:        "https://example.com/a",
		Author:      "Jane Doe",
		GUID:        "guid-1",
		ContentHTML: "<p>some <b>content</b> here</p>",
		Unread:      true,
		Published:   time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Tags:        []string{"tech", "news"},
		Flags:       "aZ",
		FeedTitle:   "Example Feed",
		FeedLink:    "https://example.com/feed.xml",
		FeedDate:    time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestEval(t *testing.T) {
	tests := []struct {
		name string
		expr string
		mut  func(s *Subject) // nil: use the base subject as-is
		want bool
	}{
		// --- operators on a plain string attribute ---
		{"eq exact", `title = "Hello World"`, nil, true},
		{"eq case-insensitive both sides", `title = "hello world"`, nil, true},
		{"eq no partial match", `title = "hello"`, nil, false},
		{"ne", `title != "hello"`, nil, true},
		{"ne negates eq", `title != "hello world"`, nil, false},
		{"contains substring", `title # "lo wo"`, nil, true},
		{"contains case-insensitive value", `title # "HELLO"`, nil, true},
		{"not contains", `title !# "lo wo"`, nil, false},
		{"not contains absent", `title !# "zzz"`, nil, true},
		{"contains empty value", `title # ""`, nil, true},

		// --- other single-valued attributes ---
		{"link eq", `link = "https://example.com/a"`, nil, true},
		{"link contains", `link # "example.com"`, nil, true},
		{"author contains", `author # "doe"`, nil, true},
		{"guid eq case-insensitive", `guid = "GUID-1"`, nil, true},
		{"guid ne empty", `guid != ""`, nil, true},
		{"content substring over raw HTML", `content # "<b>content</b>"`, nil, true},
		{"content exact over raw HTML", `content = "<p>some <b>content</b> here</p>"`, nil, true},
		{"feedtitle eq", `feedtitle = "example feed"`, nil, true},
		{"feedlink contains", `feedlink # "feed.xml"`, nil, true},

		// --- unread yes/no mapping ---
		{"unread yes", `unread = "yes"`, nil, true},
		{"unread no", `unread = "no"`, nil, false},
		{"unread ne", `unread != "no"`, nil, true},
		{"unread contains", `unread # "es"`, nil, true},
		{"unread not contains", `unread !# "es"`, nil, false},
		{"unread yes on read article", `unread = "yes"`, func(s *Subject) { s.Unread = false }, false},
		{"unread no on read article", `unread = "no"`, func(s *Subject) { s.Unread = false }, true},

		// --- dates: both renderings match ---
		{"date date-only form", `date = "2024-01-01"`, nil, true},
		{"date RFC3339 form", `date = "2024-01-01T10:00:00Z"`, nil, true},
		{"date wrong day", `date = "2024-01-02"`, nil, false},
		{"date ne", `date != "2024-01-01"`, nil, false},
		{"date contains year", `date # "2024"`, nil, true},
		{"date contains time (RFC3339 form only)", `date # "10:00"`, nil, true},
		{"date not contains", `date !# "2024"`, nil, false},
		{"feeddate date-only form", `feeddate = "2024-01-02"`, nil, true},
		{"feeddate RFC3339 form", `feeddate = "2024-01-02T03:04:05Z"`, nil, true},
		{"feeddate not article date", `feeddate = "2024-01-01"`, nil, false},
		{"date zero time as empty string", `date = ""`, func(s *Subject) { s.Published = time.Time{} }, true},
		{"date zero time contains nothing", `date # "2024"`, func(s *Subject) { s.Published = time.Time{} }, false},
		{"date non-UTC offset form", `date # "23:00"`, func(s *Subject) {
			s.Published = time.Date(2024, 1, 1, 23, 0, 0, 0, time.FixedZone("", 5*3600))
		}, true},

		// --- tags: element match vs whole value ---
		{"tags element contains", `tags # "tech"`, nil, true},
		{"tags element contains case-insensitive", `tags # "NEWS"`, nil, true},
		{"tags element partial", `tags # "eco"`, nil, false},
		{"tags whole value space-joined", `tags = "tech news"`, nil, true},
		{"tags whole value not single element", `tags = "tech"`, nil, false},
		{"tags whole value comma form does not match", `tags = "tech, news"`, nil, false},
		{"tags ne", `tags != "tech news"`, nil, false},
		{"tags not contains", `tags !# "econom"`, nil, true},
		{"tags empty list never contains", `tags # "tech"`, func(s *Subject) { s.Tags = nil }, false},
		{"tags empty list equals empty value", `tags = ""`, func(s *Subject) { s.Tags = nil }, true},

		// --- flags ---
		{"flags contains", `flags # "a"`, nil, true},
		{"flags contains case-insensitive", `flags # "A"`, nil, true},
		{"flags eq whole value", `flags = "az"`, nil, true},
		{"flags eq not single char", `flags = "a"`, nil, false},
		{"flags not contains", `flags !# "q"`, nil, true},

		// --- precedence: and binds tighter than or ---
		// base: unread=yes, title="Hello World", author="Jane Doe"
		// true or (true and false) = true; left-assoc would also be true here.
		{"or-and left operand decides", `unread = "yes" or title = "Hello" and author = "Nobody"`, nil, true},
		// false or (true and true) = true.
		{"or-and right chain true", `unread = "no" or title = "Hello World" and author = "Jane Doe"`, nil, true},
		// false or (true and false) = false.
		{"or-and right chain false", `unread = "no" or title = "Hello World" and author = "Nobody"`, nil, false},
		// true or (false and false) = true — left-assoc would give
		// (true or false) and false = false, so this proves precedence.
		{"or-and proves precedence", `unread = "yes" or title = "Bye" and author = "Nobody"`, nil, true},

		// --- parentheses override precedence ---
		{"parens change grouping", `(unread = "no" or title = "Bye") and author = "Jane Doe"`, nil, false},
		{"parens match without and", `(unread = "yes" or title = "Bye") and author = "Jane Doe"`, nil, true},
		{"nested parens", `((title = "Hello World"))`, nil, true},
		{"parens over or chain", `(title # "Hello" or author # "Doe") and guid = "guid-1"`, nil, true},

		// --- not: binds tighter than and ---
		{"not comparison", `not unread = "yes"`, nil, false},
		{"not not", `not not unread = "yes"`, nil, true},
		{"not triple", `not not not unread = "yes"`, nil, false},
		{"not parenthesized expr", `not (unread = "no" or title = "Bye")`, nil, true},
		// not binds to the comparison, not the whole conjunction:
		// (not contains) and (author mismatch) = false. If "not" scoped
		// over the entire and, this would be true.
		{"not binds tighter than and", `not title # "hello" and author = "Nobody"`, nil, false},

		// --- empty string edge cases ---
		{"empty title eq", `title = ""`, func(s *Subject) { s.Title = "" }, true},
		{"empty title ne", `title != ""`, func(s *Subject) { s.Title = "" }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Compile(tt.expr)
			if err != nil {
				t.Fatalf("Compile(%q) unexpected error: %v", tt.expr, err)
			}
			s := baseSubject()
			if tt.mut != nil {
				tt.mut(&s)
			}
			if got := f.Eval(s); got != tt.want {
				t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantMsg string // substring of SyntaxError.Msg
		wantTok string // expected Token field, "" = don't check
		wantPos int    // expected Pos field, 0 = don't check
	}{
		{"empty expression", "", "empty expression", "", 1},
		{"whitespace only", "   \t ", "empty expression", "", 1},
		{"unterminated string", `title = "abc`, "unterminated string", "abc", 9},
		{"unexpected character", `title ~ "x"`, "unexpected character", "~", 7},
		{"lone bang", `!title = "x"`, "unexpected character", "!", 1},
		{"bad op identifier", `title contains "x"`, "expected operator", "contains", 7},
		{"unknown attribute", `subtitle = "x"`, "unknown attribute", "subtitle", 1},
		{"attribute is a keyword", `title = "x" and and = "y"`, "unknown attribute", "and", 0},
		{"unbalanced open paren", `(title = "x"`, "expected ')'", "end of input", 13},
		{"unbalanced close paren", `title = "x")`, "unexpected token after expression", ")", 12},
		{"trailing garbage", `title = "x" author = "y"`, "unexpected token after expression", "author", 0},
		{"missing value", `title =`, "expected quoted string value", "end of input", 0},
		{"unquoted value", `title = x`, "expected quoted string value", "x", 9},
		{"dangling and", `title = "x" and`, "expected attribute", "end of input", 0},
		{"dangling or", `title = "x" or`, "expected attribute", "end of input", 0},
		{"dangling not", `not`, "expected attribute", "end of input", 0},
		{"empty parens", `()`, "expected attribute", ")", 2},
		{"string as attribute", `"x" = "y"`, "expected attribute", "x", 1},
		{"operator first", `= "x"`, "expected attribute", "=", 1},
		{"attribute without operator", `title`, "expected operator", "end of input", 0},
		{"double equals", `title == "x"`, "expected quoted string value", "=", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Compile(tt.src)
			if err == nil {
				t.Fatalf("Compile(%q) = %v, want error", tt.src, f)
			}
			var serr *SyntaxError
			if !errors.As(err, &serr) {
				t.Fatalf("Compile(%q) error = %T, want *SyntaxError", tt.src, err)
			}
			if !strings.Contains(serr.Msg, tt.wantMsg) {
				t.Errorf("Msg = %q, want substring %q", serr.Msg, tt.wantMsg)
			}
			if tt.wantTok != "" && serr.Token != tt.wantTok {
				t.Errorf("Token = %q, want %q", serr.Token, tt.wantTok)
			}
			if tt.wantPos != 0 && serr.Pos != tt.wantPos {
				t.Errorf("Pos = %d, want %d", serr.Pos, tt.wantPos)
			}
			if serr.Pos < 1 {
				t.Errorf("Pos = %d, want 1-based (>0)", serr.Pos)
			}
		})
	}
}

// TestEvalConcurrent locks in the "Compile once, Eval many" contract; run
// with -race (CI does).
func TestEvalConcurrent(t *testing.T) {
	f, err := Compile(`unread = "yes" and (title # "hello" or tags # "tech")`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	s := baseSubject()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				if !f.Eval(s) {
					t.Errorf("goroutine %d: Eval = false, want true", i)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestParseQueryFeedURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantOK   bool
		wantName string
		wantExpr string
	}{
		{"valid", "query:Tech:unread = \"yes\"", true, "Tech", `unread = "yes"`},
		{"expression with colons", `query:Name:title # "a:b"`, true, "Name", `title # "a:b"`},
		{"empty name is allowed", "query::unread = \"yes\"", true, "", `unread = "yes"`},
		{"non-query http URL", "https://example.com/rss", false, "", ""},
		{"plain tag string", "tech news", false, "", ""},
		{"query prefix only", "query:", false, "", ""},
		{"name but no colon", "query:Name", false, "", ""},
		{"name colon empty expression", "query:Name:", false, "", ""},
		{"uppercase prefix is not a query feed", "Query:Name:unread = \"yes\"", false, "", ""},
		{"prefix inside URL", "https://example.com/?query:x:y", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, expr, ok := ParseQueryFeedURL(tt.url)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if expr != tt.wantExpr {
				t.Errorf("expr = %q, want %q", expr, tt.wantExpr)
			}
		})
	}
}
