package search

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  string // substring expected in Msg; "" means no error
		wantPos  int    // expected SyntaxError.Pos when wantErr != ""
		terms    []string
		excludes []string
		phrases  []string
		regexps  []string // expected pattern source, e.g. "(?i)go+gle"
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "whitespace only",
			input: " \t\n ",
		},
		{
			name:  "single plain term",
			input: "foo",
			terms: []string{"foo"},
		},
		{
			name:  "multiple plain terms",
			input: "foo bar baz",
			terms: []string{"foo", "bar", "baz"},
		},
		{
			name:  "mixed case preserved",
			input: "Foo BAR bAz",
			terms: []string{"Foo", "BAR", "bAz"},
		},
		{
			name:  "extra whitespace between terms",
			input: "  foo   bar  ",
			terms: []string{"foo", "bar"},
		},
		{
			name:  "unicode terms",
			input: "привет мир",
			terms: []string{"привет", "мир"},
		},
		{
			name:     "exclusion",
			input:    "foo -bar",
			terms:    []string{"foo"},
			excludes: []string{"bar"},
		},
		{
			name:     "exclusion only",
			input:    "-bar",
			excludes: []string{"bar"},
		},
		{
			name:     "double dash excludes dashed term",
			input:    "--foo",
			excludes: []string{"-foo"},
		},
		{
			name:  "lone dash is a plain term",
			input: "-",
			terms: []string{"-"},
		},
		{
			name:  "dash followed by space is a plain term",
			input: "a - b",
			terms: []string{"a", "-", "b"},
		},
		{
			name:  "dash inside term is literal",
			input: "foo-bar",
			terms: []string{"foo-bar"},
		},
		{
			name:    "quoted phrase",
			input:   `"hello world"`,
			phrases: []string{"hello world"},
		},
		{
			name:    "phrase with dash and slash",
			input:   `"a/b-c d"`,
			phrases: []string{"a/b-c d"},
		},
		{
			name:    "phrase keeps case",
			input:   `"Hello World"`,
			phrases: []string{"Hello World"},
		},
		{
			name:    "two phrases",
			input:   `"a b" "c d"`,
			phrases: []string{"a b", "c d"},
		},
		{
			name:    "regex",
			input:   "/go+gle/",
			regexps: []string{"(?i)go+gle"},
		},
		{
			name:    "regex with own flags",
			input:   "/(?s)a.b/",
			regexps: []string{"(?i)(?s)a.b"},
		},
		{
			name:    "regex with quote and dash",
			input:   `/a"b-c/`,
			regexps: []string{`(?i)a"b-c`},
		},
		{
			name:     "all token kinds combined",
			input:    `foo -bar "a b" /re/`,
			terms:    []string{"foo"},
			excludes: []string{"bar"},
			phrases:  []string{"a b"},
			regexps:  []string{"(?i)re"},
		},
		{
			name:    "backslash inside phrase is literal (no escapes)",
			input:   `"a\b"`,
			phrases: []string{`a\b`},
		},
		{
			name:  "backslash inside term is literal (no escapes)",
			input: `a\b`,
			terms: []string{`a\b`},
		},
		{
			name:    "term stops at quote, rest is a phrase",
			input:   `foo"bar baz"`,
			terms:   []string{"foo"},
			phrases: []string{"bar baz"},
		},
		{
			name:    "term stops at slash, rest is an unterminated regex",
			input:   "a/b",
			wantErr: "unterminated regex",
			wantPos: 1,
		},
		{
			name:    "unterminated quote",
			input:   `"foo`,
			wantErr: "unterminated quoted phrase",
			wantPos: 0,
		},
		{
			name:    "unterminated quote after a term",
			input:   `foo "bar`,
			wantErr: "unterminated quoted phrase",
			wantPos: 4,
		},
		{
			name:    "unterminated regex",
			input:   "/foo",
			wantErr: "unterminated regex",
			wantPos: 0,
		},
		{
			name:    "unterminated regex after a term",
			input:   "foo /bar",
			wantErr: "unterminated regex",
			wantPos: 4,
		},
		{
			name:    "invalid regex",
			input:   "/[/",
			wantErr: "invalid regex",
			wantPos: 0,
		},
		{
			name:    "empty quoted phrase",
			input:   `""`,
			wantErr: "empty quoted phrase",
			wantPos: 0,
		},
		{
			name:    "empty regex",
			input:   "//",
			wantErr: "empty regex",
			wantPos: 0,
		},
		{
			name:    "exclusion of a quoted phrase is rejected",
			input:   `-"a b"`,
			wantErr: "exclusion must be a plain term",
			wantPos: 0,
		},
		{
			name:    "exclusion of a regex is rejected",
			input:   `-/re/`,
			wantErr: "exclusion must be a plain term",
			wantPos: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, want error containing %q", tt.input, q, tt.wantErr)
				}
				se, ok := err.(*SyntaxError)
				if !ok {
					t.Fatalf("Parse(%q) error = %T (%v), want *SyntaxError", tt.input, err, err)
				}
				if !strings.Contains(se.Msg, tt.wantErr) {
					t.Errorf("Msg = %q, want containing %q", se.Msg, tt.wantErr)
				}
				if se.Pos != tt.wantPos {
					t.Errorf("Pos = %d, want %d", se.Pos, tt.wantPos)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if !eqStrings(q.Terms, tt.terms) {
				t.Errorf("Terms = %q, want %q", q.Terms, tt.terms)
			}
			if !eqStrings(q.Exclude, tt.excludes) {
				t.Errorf("Exclude = %q, want %q", q.Exclude, tt.excludes)
			}
			if !eqStrings(q.Phrases, tt.phrases) {
				t.Errorf("Phrases = %q, want %q", q.Phrases, tt.phrases)
			}
			if len(q.Regexps) != len(tt.regexps) {
				t.Fatalf("len(Regexps) = %d, want %d", len(q.Regexps), len(tt.regexps))
			}
			for i, re := range q.Regexps {
				if re.String() != tt.regexps[i] {
					t.Errorf("Regexps[%d] = %q, want %q", i, re.String(), tt.regexps[i])
				}
			}
		})
	}
}

func TestSyntaxErrorMessage(t *testing.T) {
	_, err := Parse(`"oops`)
	se, ok := err.(*SyntaxError)
	if !ok {
		t.Fatalf("error = %T (%v), want *SyntaxError", err, err)
	}
	if got := se.Error(); got != "search: unterminated quoted phrase at position 0" {
		t.Errorf("Error() = %q", got)
	}
}

// eqStrings compares string slices, treating nil and empty as equal.
func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
