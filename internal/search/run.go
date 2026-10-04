package search

import (
	"regexp"
	"sort"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// MaxHits is the maximum number of match positions reported per field
// (title, content) per result.
const MaxHits = 32

// A Range is a half-open byte range into a string field.
type Range struct{ Start, End int }

// A Source pairs a feed's identity and articles (already loaded from the
// store) with their read state.
type Source struct {
	FeedURL, FeedTitle string
	Articles           []feed.Article
	Read               map[string]bool // article ID -> read
}

// A Result is one matching article with the positions of its matches, so
// the TUI can highlight them later.
type Result struct {
	Article   feed.Article
	FeedURL   string
	FeedTitle string
	// Read reports the article's read state from the Source it came from.
	Read bool
	// TitleHits and ContentHits hold up to MaxHits match positions each,
	// sorted by Start: byte offsets into Article.Title and
	// Article.ContentHTML respectively. Matches from all positive
	// criteria (terms, phrases, regexes) are merged; exclusions produce
	// no hits.
	TitleHits   []Range
	ContentHits []Range
}

// Run executes q over the given sources and returns matches ordered
// newest first (by Article.Published, ties broken by Article.Title
// ascending, then stable by source order). A nil query or a query with
// no criteria matches nothing.
//
// Each field is folded once per article and every needle is scanned
// against that single folded copy; article slices are not copied — only
// matching articles are copied into results. Regexes run against the
// original (unfolded) fields.
func Run(q *Query, sources []Source) []Result {
	if q == nil {
		return nil
	}
	terms := foldAll(q.Terms)
	exclude := foldAll(q.Exclude)
	phrases := foldAll(q.Phrases)
	regexps := nonNil(q.Regexps)
	if len(terms) == 0 && len(exclude) == 0 && len(phrases) == 0 && len(regexps) == 0 {
		return nil // empty query matches nothing
	}

	var results []Result
	for _, src := range sources {
		for i := range src.Articles {
			a := &src.Articles[i]
			title := fold(a.Title)
			content := fold(a.ContentHTML)

			matched := true
			for _, ex := range exclude {
				if title.contains(ex) || content.contains(ex) {
					matched = false
					break
				}
			}

			var titleHits, contentHits []Range
			matchAll := func(needles []string) bool {
				for _, n := range needles {
					th := title.findAll(n, MaxHits)
					ch := content.findAll(n, MaxHits)
					if len(th) == 0 && len(ch) == 0 {
						return false
					}
					titleHits = append(titleHits, th...)
					contentHits = append(contentHits, ch...)
				}
				return true
			}
			if matched && !matchAll(terms) {
				matched = false
			}
			if matched && !matchAll(phrases) {
				matched = false
			}
			if matched {
				for _, re := range regexps {
					th := regexHits(re, a.Title, MaxHits)
					ch := regexHits(re, a.ContentHTML, MaxHits)
					if len(th) == 0 && len(ch) == 0 {
						matched = false
						break
					}
					titleHits = append(titleHits, th...)
					contentHits = append(contentHits, ch...)
				}
			}
			if !matched {
				continue
			}

			results = append(results, Result{
				Article:     *a,
				FeedURL:     src.FeedURL,
				FeedTitle:   src.FeedTitle,
				Read:        src.Read[a.ID],
				TitleHits:   normalizeHits(titleHits),
				ContentHits: normalizeHits(contentHits),
			})
		}
	}

	sort.SliceStable(results, func(x, y int) bool {
		a, b := results[x].Article, results[y].Article
		if !a.Published.Equal(b.Published) {
			return a.Published.After(b.Published)
		}
		return a.Title < b.Title
	})
	return results
}

// regexHits returns up to limit match positions of re in s.
func regexHits(re *regexp.Regexp, s string, limit int) []Range {
	if limit <= 0 {
		return nil
	}
	found := re.FindAllStringIndex(s, limit)
	if found == nil {
		return nil
	}
	hits := make([]Range, len(found))
	for i, p := range found {
		hits[i] = Range{Start: p[0], End: p[1]}
	}
	return hits
}

// normalizeHits sorts hits by Start (then End), drops exact duplicates
// (the same term listed twice in the query), and caps the slice at
// MaxHits.
func normalizeHits(hits []Range) []Range {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Start != hits[j].Start {
			return hits[i].Start < hits[j].Start
		}
		return hits[i].End < hits[j].End
	})
	out := hits[:0]
	for _, h := range hits {
		if len(out) > 0 && h == out[len(out)-1] {
			continue
		}
		if len(out) == MaxHits {
			break
		}
		out = append(out, h)
	}
	return out
}

// foldAll folds every needle once (so Run does not re-fold them per
// article) and drops empty strings. Parse never produces empty needles;
// this is defensive for hand-built Query values.
func foldAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			out = append(out, foldRunes(s))
		}
	}
	return out
}

// nonNil drops nil regexps, defensively for hand-built Query values.
func nonNil(rs []*regexp.Regexp) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(rs))
	for _, r := range rs {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}
