package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestFoldFindAll(t *testing.T) {
	tests := []struct {
		name   string
		hay    string
		needle string // folded with foldRunes before the call, like Run does
		limit  int
		want   []Range
	}{
		{
			name:   "ascii",
			hay:    "Hello WORLD",
			needle: "world",
			limit:  32,
			want:   []Range{{6, 11}},
		},
		{
			name:   "ascii multiple hits",
			hay:    "ab ab ab",
			needle: "ab",
			limit:  32,
			want:   []Range{{0, 2}, {3, 5}, {6, 8}},
		},
		{
			name:   "limit caps hits",
			hay:    "a a a a",
			needle: "a",
			limit:  2,
			want:   []Range{{0, 1}, {2, 3}},
		},
		{
			name:   "non-overlapping hits",
			hay:    "aaa",
			needle: "aa",
			limit:  32,
			want:   []Range{{0, 2}},
		},
		{
			name:   "no match",
			hay:    "abc",
			needle: "z",
			limit:  32,
			want:   nil,
		},
		{
			name:   "needle longer than hay",
			hay:    "ab",
			needle: "abc",
			limit:  32,
			want:   nil,
		},
		{
			name:   "cyrillic same byte length",
			hay:    "Привет МИР",
			needle: "мир",
			limit:  32,
			want:   []Range{{13, 19}},
		},
		{
			name:   "cyrillic needle in different case",
			hay:    "привет",
			needle: "ПРИВЕТ",
			limit:  32,
			want:   []Range{{0, 12}},
		},
		{
			name:   "latin umlaut same byte length",
			hay:    "ÜBER alles",
			needle: "über",
			limit:  32,
			want:   []Range{{0, 5}}, // Ü is 2 bytes
		},
		{
			name:   "kelvin sign folds to a shorter rune",
			hay:    "Temperature: \u212A",
			needle: "k",
			limit:  32,
			want:   []Range{{13, 16}},
		},
		{
			name:   "match after a length-changing rune",
			hay:    "\u212A k",
			needle: "k",
			limit:  32,
			want:   []Range{{0, 3}, {4, 5}},
		},
		{
			name:   "needle after folded needle is longer",
			hay:    "\u212A",
			needle: "\u212A",
			limit:  32,
			want:   []Range{{0, 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fold(tt.hay)
			got := f.findAll(foldRunes(tt.needle), tt.limit)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findAll(%q, %q, %d) = %v, want %v", tt.hay, tt.needle, tt.limit, got, tt.want)
			}
		})
	}
}

func TestFoldContains(t *testing.T) {
	f := fold("Hello World")
	if !f.contains(foldRunes("WORLD")) {
		t.Error("contains(WORLD) = false, want true")
	}
	if f.contains(foldRunes("missing")) {
		t.Error("contains(missing) = true, want false")
	}
}

func TestFoldRunes(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"ABC", "abc"},
		{"Привет", "привет"},
		{"\u212A", "k"}, // KELVIN SIGN -> 1-byte k
	}
	for _, tt := range tests {
		if got := foldRunes(tt.in); got != tt.want {
			t.Errorf("foldRunes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFoldASCIIFastPathMatchesGeneralPath(t *testing.T) {
	// The ascii fast path must behave exactly like the table-based path.
	for _, s := range []string{"Hello", "a b c", "", "no-upper"} {
		f := fold(s)
		if !f.ascii {
			t.Errorf("fold(%q): ascii = false, want true", s)
		}
		got := f.findAll("hello", 32)
		want := strings.Count(strings.ToLower(s), "hello")
		if len(got) != want {
			t.Errorf("fold(%q).findAll(\"hello\") = %v, want %d hits", s, got, want)
		}
	}
}
