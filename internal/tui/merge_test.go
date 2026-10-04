package tui

import (
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

var (
	t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
)

func art(id string, published time.Time, title string) feed.Article {
	return feed.Article{ID: id, Title: title, Published: published}
}

func ids(articles []feed.Article) []string {
	out := make([]string, len(articles))
	for i, a := range articles {
		out[i] = a.ID
	}
	return out
}

func TestMergeArticles(t *testing.T) {
	tests := []struct {
		name     string
		old      []feed.Article
		fresh    []feed.Article
		read     map[string]bool
		wantIDs  []string
		wantRead map[string]bool
	}{
		{
			name:     "fresh only",
			fresh:    []feed.Article{art("b", t1, ""), art("a", t2, "")},
			wantIDs:  []string{"a", "b"}, // newest first
			wantRead: map[string]bool{},
		},
		{
			name:     "old only survives",
			old:      []feed.Article{art("a", t2, "old")},
			fresh:    nil,
			wantIDs:  []string{"a"},
			wantRead: map[string]bool{},
		},
		{
			name:     "duplicate id: fresh wins, single entry",
			old:      []feed.Article{art("a", t2, "old title")},
			fresh:    []feed.Article{art("a", t2, "new title")},
			wantIDs:  []string{"a"},
			wantRead: map[string]bool{},
		},
		{
			name:     "union ordered newest first",
			old:      []feed.Article{art("old1", t1, ""), art("old2", t0, "")},
			fresh:    []feed.Article{art("new1", t2, "")},
			wantIDs:  []string{"new1", "old1", "old2"},
			wantRead: map[string]bool{},
		},
		{
			name:     "zero published sorts last",
			fresh:    []feed.Article{{ID: "nodate"}, art("dated", t1, "")},
			wantIDs:  []string{"dated", "nodate"},
			wantRead: map[string]bool{},
		},
		{
			name:     "read flags survive for present articles; flags for vanished ids are pruned",
			old:      []feed.Article{art("kept", t1, ""), art("gone", t2, "")},
			fresh:    []feed.Article{art("kept", t1, ""), art("added", t2, "")},
			read:     map[string]bool{"kept": true, "gone": true, "ghost": true},
			wantIDs:  []string{"added", "gone", "kept"},           // old-only articles survive
			wantRead: map[string]bool{"kept": true, "gone": true}, // "ghost" pruned, "added" unread
		},
		{
			name:     "duplicate ids within fresh collapse to first",
			fresh:    []feed.Article{art("a", t2, "first"), art("a", t1, "second")},
			wantIDs:  []string{"a"},
			wantRead: map[string]bool{},
		},
		{
			name:     "empty everything",
			wantRead: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotRead := mergeArticles(tt.old, tt.fresh, tt.read)

			gotIDs := ids(got)
			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", gotIDs, tt.wantIDs)
			}
			for i := range gotIDs {
				if gotIDs[i] != tt.wantIDs[i] {
					t.Fatalf("ids = %v, want %v", gotIDs, tt.wantIDs)
				}
			}
			if len(gotRead) != len(tt.wantRead) {
				t.Fatalf("read = %v, want %v", gotRead, tt.wantRead)
			}
			for id, want := range tt.wantRead {
				if gotRead[id] != want {
					t.Fatalf("read[%q] = %v, want %v (full: %v)", id, gotRead[id], want, gotRead)
				}
			}
		})
	}
}

func TestMergeArticlesFreshWinsContent(t *testing.T) {
	old := []feed.Article{{ID: "a", Title: "old", ContentHTML: "old body"}}
	fresh := []feed.Article{{ID: "a", Title: "new", ContentHTML: "new body"}}
	got, _ := mergeArticles(old, fresh, nil)
	if len(got) != 1 {
		t.Fatalf("got %d articles, want 1", len(got))
	}
	if got[0].Title != "new" || got[0].ContentHTML != "new body" {
		t.Errorf("merged article = %+v, want fresh content", got[0])
	}
}

func TestWindowRange(t *testing.T) {
	tests := []struct {
		name                string
		cursor, total, rows int
		wantStart, wantEnd  int
	}{
		{"empty", 0, 0, 10, 0, 0},
		{"fits", 1, 3, 10, 0, 3},
		{"cursor top", 0, 20, 5, 0, 5},
		{"cursor middle centered", 10, 20, 5, 8, 13},
		{"cursor bottom clamped", 19, 20, 5, 15, 20},
		{"rows<1", 0, 5, 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := windowRange(tt.cursor, tt.total, tt.rows)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("windowRange(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.cursor, tt.total, tt.rows, start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"much too long", 8, "much to…"},
		{"x", 1, "x"},
		{"xy", 1, "…"},
		{"anything", 0, ""},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
