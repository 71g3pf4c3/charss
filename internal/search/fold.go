package search

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Case-insensitive matching uses simple case mapping (rune-by-rune
// unicode.ToLower), not full case folding. Simple mapping is 1:1 in runes,
// so a match in the folded text corresponds rune-for-rune to the original
// text, and the byte offsets reported by foldedString.findAll are always
// exact — even when folding changes byte lengths (e.g. U+212A KELVIN
// SIGN lowercases to a 1-byte 'k' from 3 bytes).

// foldRunes lowercases s rune by rune (simple case mapping).
func foldRunes(s string) string {
	if isASCII(s) {
		return strings.ToLower(s) // ASCII lowering is length-preserving
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// A foldedString is a string pre-lowercased for repeated case-insensitive
// substring search, with enough bookkeeping to map match positions in the
// lowercase text back to byte offsets in the original string.
type foldedString struct {
	lower string
	// ascii is true when lower and the original string have identical
	// bytes at identical offsets (the fast path: pure-ASCII text, where
	// lowering is length-preserving).
	ascii bool
	// For non-ASCII text, runeStarts[i] and origStarts[i] are the byte
	// offsets of the i-th rune in lower and in the original string
	// respectively. Both slices have runeCount+1 entries; the last entry
	// is the total length. Runes correspond 1:1 between the two.
	runeStarts []int32
	origStarts []int32
}

// fold precomputes the lowercase view of s.
func fold(s string) foldedString {
	if isASCII(s) {
		return foldedString{lower: strings.ToLower(s), ascii: true}
	}
	var b strings.Builder
	b.Grow(len(s))
	runeStarts := make([]int32, 0, utf8.RuneCountInString(s)+1)
	origStarts := make([]int32, 0, cap(runeStarts))
	for i, r := range s {
		runeStarts = append(runeStarts, int32(b.Len()))
		origStarts = append(origStarts, int32(i))
		b.WriteRune(unicode.ToLower(r))
	}
	runeStarts = append(runeStarts, int32(b.Len()))
	origStarts = append(origStarts, int32(len(s)))
	return foldedString{
		lower:      b.String(),
		runeStarts: runeStarts,
		origStarts: origStarts,
	}
}

// contains reports whether the folded string contains a case-insensitive
// occurrence of needle. needle must be non-empty and pre-folded with
// foldRunes.
func (f foldedString) contains(needle string) bool {
	return strings.Contains(f.lower, needle)
}

// findAll returns the byte offsets (in the original string) of up to
// limit non-overlapping occurrences of needle. needle must be non-empty
// and pre-folded with foldRunes. Overlapping matches are not reported:
// scanning resumes at the end of each hit.
func (f foldedString) findAll(needle string, limit int) []Range {
	if limit <= 0 || needle == "" {
		return nil
	}
	var hits []Range
	from := 0
	for len(hits) < limit && from <= len(f.lower) {
		j := strings.Index(f.lower[from:], needle)
		if j < 0 {
			break
		}
		p := from + j
		hits = append(hits, f.mapRange(p, p+len(needle)))
		from = p + len(needle)
	}
	return hits
}

// mapRange maps a rune-aligned byte range [lo, hi) in the lowercase text
// back to a byte range in the original string. Matches found by findAll
// are always rune-aligned: the first byte of a folded needle is a UTF-8
// start byte, and UTF-8 start bytes never appear mid-rune in the folded
// haystack.
func (f foldedString) mapRange(lo, hi int) Range {
	if f.ascii {
		return Range{Start: lo, End: hi}
	}
	return Range{
		Start: int(f.origStarts[f.runeIndex(lo)]),
		End:   int(f.origStarts[f.runeIndex(hi)]),
	}
}

// runeIndex returns the index of the rune that starts at byte offset off
// in the lowercase text. off must be rune-aligned (or len(lower)).
func (f foldedString) runeIndex(off int) int {
	return sort.Search(len(f.runeStarts), func(i int) bool {
		return int(f.runeStarts[i]) >= off
	})
}
