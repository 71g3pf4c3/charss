package search

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

func pub(day int) time.Time {
	return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC)
}

// resultSummary is the projection of a Result that the table tests
// assert on.
type resultSummary struct {
	id, feedURL, feedTitle string
	read                   bool
	titleHits, contentHits []Range
}

func summarize(rs []Result) []resultSummary {
	if len(rs) == 0 {
		return nil
	}
	out := make([]resultSummary, len(rs))
	for i, r := range rs {
		out[i] = resultSummary{
			id:          r.Article.ID,
			feedURL:     r.FeedURL,
			feedTitle:   r.FeedTitle,
			read:        r.Read,
			titleHits:   r.TitleHits,
			contentHits: r.ContentHits,
		}
	}
	return out
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		sources []Source
		want    []resultSummary
	}{
		{
			name:  "empty query matches nothing",
			query: "",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "anything"},
			}}},
			want: nil,
		},
		{
			name:  "whitespace query matches nothing",
			query: "   ",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "anything"},
			}}},
			want: nil,
		},
		{
			name:  "AND semantics: every term must match somewhere",
			query: "foo bar",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "foo", Published: pub(1)},
				{ID: "a2", Title: "foo", ContentHTML: "bar", Published: pub(2)},
				{ID: "a3", Title: "misc", ContentHTML: "foo and bar", Published: pub(3)},
			}}},
			want: []resultSummary{
				{id: "a3", feedURL: "http://f", contentHits: []Range{{0, 3}, {8, 11}}},
				{id: "a2", feedURL: "http://f", titleHits: []Range{{0, 3}}, contentHits: []Range{{0, 3}}},
			},
		},
		{
			name:  "term matches in title or content, case-insensitive",
			query: "WORLD",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "Hello World", ContentHTML: "world peace", Published: pub(1)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f", titleHits: []Range{{6, 11}}, contentHits: []Range{{0, 5}}},
			},
		},
		{
			name:  "exclusion wins over matching terms",
			query: "foo -bar",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "foo baz", Published: pub(1)},
				{ID: "a2", Title: "t", ContentHTML: "foo bar", Published: pub(2)},
				{ID: "a3", Title: "bar foo", Published: pub(3)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f", titleHits: []Range{{0, 3}}},
			},
		},
		{
			name:  "exclusion-only query matches the rest",
			query: "-absent",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "f1", Title: "one", ContentHTML: "news", Published: pub(1)},
				{ID: "f2", Title: "two", ContentHTML: "news", Published: pub(2)},
			}}},
			want: []resultSummary{
				{id: "f2", feedURL: "http://f"},
				{id: "f1", feedURL: "http://f"},
			},
		},
		{
			name:  "phrase requires adjacency",
			query: `"foo bar"`,
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "x", ContentHTML: "one foo bar two", Published: pub(1)},
				{ID: "a2", Title: "x", ContentHTML: "foo ... bar", Published: pub(2)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f", contentHits: []Range{{4, 11}}},
			},
		},
		{
			name:  "regex mode, case-insensitive",
			query: "/go+gle/",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "GooGLe docs", Published: pub(1)},
				{ID: "a2", Title: "t", ContentHTML: "goooogle it", Published: pub(2)},
				{ID: "a3", Title: "t", ContentHTML: "ggle", Published: pub(3)},
			}}},
			want: []resultSummary{
				{id: "a2", feedURL: "http://f", contentHits: []Range{{0, 8}}},
				{id: "a1", feedURL: "http://f", titleHits: []Range{{0, 6}}},
			},
		},
		{
			name:  "all regexes must match",
			query: "/a/ /b/",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "t", ContentHTML: "only a", Published: pub(1)},
				{ID: "a2", Title: "t", ContentHTML: "a and b", Published: pub(2)},
			}}},
			want: []resultSummary{
				{id: "a2", feedURL: "http://f", contentHits: []Range{{0, 1}, {2, 3}, {6, 7}}},
			},
		},
		{
			name:  "unicode hit positions (cyrillic)",
			query: "мир",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "Привет мир", ContentHTML: "ПРИВЕТ МИР здесь", Published: pub(1)},
			}}},
			want: []resultSummary{
				// "мир" starts after "Привет " (12 + 1 bytes) and is 6 bytes long.
				{id: "a1", feedURL: "http://f", titleHits: []Range{{13, 19}}, contentHits: []Range{{13, 19}}},
			},
		},
		{
			name:  "unicode hit positions (uppercase needle)",
			query: "МИР",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "Привет мир", ContentHTML: "ПРИВЕТ МИР здесь", Published: pub(1)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f", titleHits: []Range{{13, 19}}, contentHits: []Range{{13, 19}}},
			},
		},
		{
			name:  "unicode hit positions (folding changes byte length)",
			query: "k",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				// \u212A is KELVIN SIGN: 3 bytes, lowercases to 1-byte k.
				{ID: "a1", Title: "Temp", ContentHTML: "300\u212A", Published: pub(1)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f", contentHits: []Range{{3, 6}}},
			},
		},
		{
			name:  "read state and feed identity carried through",
			query: "lorem",
			sources: []Source{
				{
					FeedURL: "http://one", FeedTitle: "One",
					Articles: []feed.Article{{ID: "a1", Title: "lorem", ContentHTML: "lorem", Published: pub(2)}},
					Read:     map[string]bool{"a1": true},
				},
				{
					FeedURL: "http://two", FeedTitle: "Two",
					Articles: []feed.Article{{ID: "a2", Title: "lorem", Published: pub(1)}},
				},
			},
			want: []resultSummary{
				{id: "a1", feedURL: "http://one", feedTitle: "One", read: true,
					titleHits: []Range{{0, 5}}, contentHits: []Range{{0, 5}}},
				{id: "a2", feedURL: "http://two", feedTitle: "Two",
					titleHits: []Range{{0, 5}}},
			},
		},
		{
			name:  "ordering: newest first, ties broken by title",
			query: "lorem",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "b1", Title: "delta", ContentHTML: "lorem", Published: pub(1)},
				{ID: "b2", Title: "beta", ContentHTML: "lorem", Published: pub(3)},
				{ID: "b3", Title: "alpha", ContentHTML: "lorem", Published: pub(3)},
				{ID: "b4", Title: "gamma", ContentHTML: "lorem", Published: pub(2)},
			}}},
			want: []resultSummary{
				{id: "b3", feedURL: "http://f", contentHits: []Range{{0, 5}}},
				{id: "b2", feedURL: "http://f", contentHits: []Range{{0, 5}}},
				{id: "b4", feedURL: "http://f", contentHits: []Range{{0, 5}}},
				{id: "b1", feedURL: "http://f", contentHits: []Range{{0, 5}}},
			},
		},
		{
			name:  "ordering: full ties keep source order",
			query: "x",
			sources: []Source{
				{FeedURL: "http://one", Articles: []feed.Article{{ID: "s1", Title: "same", ContentHTML: "x", Published: pub(1)}}},
				{FeedURL: "http://two", Articles: []feed.Article{{ID: "s2", Title: "same", ContentHTML: "x", Published: pub(1)}}},
			},
			want: []resultSummary{
				{id: "s1", feedURL: "http://one", contentHits: []Range{{0, 1}}},
				{id: "s2", feedURL: "http://two", contentHits: []Range{{0, 1}}},
			},
		},
		{
			name:  "combined terms, exclusion, phrase and regex",
			query: `kubernetes -docker "cluster node" /pod?s/`,
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "kubernetes upgrade", ContentHTML: "a cluster node with pods", Published: pub(1)},
				{ID: "a2", Title: "t", ContentHTML: "kubernetes cluster node docker", Published: pub(2)},
				{ID: "a3", Title: "t", ContentHTML: "kubernetes cluster node", Published: pub(3)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f",
					titleHits:   []Range{{0, 10}},
					contentHits: []Range{{2, 14}, {20, 24}}},
			},
		},
		{
			name:  "query matching everything",
			query: "/.+/",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "e1", Title: "one", ContentHTML: "x", Published: pub(2)},
				{ID: "e2", Title: "two", ContentHTML: "y", Published: pub(1)},
			}}},
			want: []resultSummary{
				{id: "e1", feedURL: "http://f", titleHits: []Range{{0, 3}}, contentHits: []Range{{0, 1}}},
				{id: "e2", feedURL: "http://f", titleHits: []Range{{0, 3}}, contentHits: []Range{{0, 1}}},
			},
		},
		{
			name:  "duplicate ranges from repeated terms are deduped",
			query: "go go",
			sources: []Source{{FeedURL: "http://f", Articles: []feed.Article{
				{ID: "a1", Title: "go", ContentHTML: "go go", Published: pub(1)},
			}}},
			want: []resultSummary{
				{id: "a1", feedURL: "http://f",
					titleHits:   []Range{{0, 2}},
					contentHits: []Range{{0, 2}, {3, 5}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.query, err)
			}
			got := summarize(Run(q, tt.sources))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Run(%q) =\n%+v\nwant\n%+v", tt.query, got, tt.want)
			}
		})
	}
}

func TestRunHitCap(t *testing.T) {
	q, err := Parse("hit")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := Source{FeedURL: "http://f", Articles: []feed.Article{{
		ID:          "a",
		Title:       "spam",
		ContentHTML: strings.Repeat("hit ", 50),
		Published:   pub(1),
	}}}
	got := Run(q, []Source{src})
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	hits := got[0].ContentHits
	if len(hits) != MaxHits {
		t.Fatalf("len(ContentHits) = %d, want %d", len(hits), MaxHits)
	}
	if hits[0] != (Range{0, 3}) {
		t.Errorf("first hit = %v, want {0 3}", hits[0])
	}
	last := Range{(MaxHits - 1) * 4, (MaxHits-1)*4 + 3}
	if hits[MaxHits-1] != last {
		t.Errorf("last hit = %v, want %v", hits[MaxHits-1], last)
	}
}

func TestRunCarriesArticle(t *testing.T) {
	want := feed.Article{
		ID:          "a",
		Title:       "Hello",
		ContentHTML: "hello world",
		Published:   pub(1),
	}
	q, err := Parse("hello")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Run(q, []Source{{FeedURL: "http://f", Articles: []feed.Article{want}}})
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	// feed.Article contains a slice (Enclosures), so compare field-wise
	// instead of != on the whole struct.
	if got[0].Article.ID != want.ID ||
		got[0].Article.Title != want.Title ||
		got[0].Article.ContentHTML != want.ContentHTML ||
		!got[0].Article.Published.Equal(want.Published) {
		t.Errorf("Article = %+v, want %+v", got[0].Article, want)
	}
}

func TestRunDegenerateQueries(t *testing.T) {
	sources := []Source{{FeedURL: "http://f", Articles: []feed.Article{
		{ID: "a1", Title: "anything"},
	}}}
	if got := Run(nil, sources); got != nil {
		t.Errorf("Run(nil) = %v, want nil", got)
	}
	if got := Run(&Query{}, sources); got != nil {
		t.Errorf("Run(&Query{}) = %v, want nil", got)
	}
	// Empty needles from hand-built queries are ignored, not treated as
	// match-everything.
	if got := Run(&Query{Terms: []string{""}, Phrases: []string{""}, Exclude: []string{""}}, sources); got != nil {
		t.Errorf("Run(empty needles) = %v, want nil", got)
	}
}
