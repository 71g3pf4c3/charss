package cmd

import (
	"testing"

	"github.com/71g3pf4c3/charss/internal/urls"
)

func TestMergeFeeds(t *testing.T) {
	tests := []struct {
		name        string
		existing    []urls.Feed
		imported    []urls.Feed
		wantAdded   []urls.Feed
		wantSkipped int
	}{
		{
			name:      "empty urls file imports everything",
			existing:  nil,
			imported:  []urls.Feed{{URL: "https://a.com/rss", Title: "A"}, {URL: "https://b.com/rss"}},
			wantAdded: []urls.Feed{{URL: "https://a.com/rss", Title: "A"}, {URL: "https://b.com/rss"}},
		},
		{
			name:        "already present urls are skipped",
			existing:    []urls.Feed{{URL: "https://a.com/rss", Title: "old title"}},
			imported:    []urls.Feed{{URL: "https://a.com/rss", Title: "new title"}, {URL: "https://b.com/rss"}},
			wantAdded:   []urls.Feed{{URL: "https://b.com/rss"}},
			wantSkipped: 1,
		},
		{
			name:        "duplicates within imported are skipped",
			existing:    nil,
			imported:    []urls.Feed{{URL: "https://a.com/rss"}, {URL: "https://a.com/rss"}, {URL: "https://b.com/rss"}},
			wantAdded:   []urls.Feed{{URL: "https://a.com/rss"}, {URL: "https://b.com/rss"}},
			wantSkipped: 1,
		},
		{
			name:        "nothing new",
			existing:    []urls.Feed{{URL: "https://a.com/rss"}},
			imported:    []urls.Feed{{URL: "https://a.com/rss"}},
			wantAdded:   nil,
			wantSkipped: 1,
		},
		{
			name:      "empty import",
			existing:  []urls.Feed{{URL: "https://a.com/rss"}},
			imported:  nil,
			wantAdded: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, skipped := MergeFeeds(tt.existing, tt.imported)
			if skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tt.wantSkipped)
			}
			if len(added) != len(tt.wantAdded) {
				t.Fatalf("added %d feeds, want %d: %+v", len(added), len(tt.wantAdded), added)
			}
			for i := range added {
				if added[i].URL != tt.wantAdded[i].URL || added[i].Title != tt.wantAdded[i].Title {
					t.Errorf("added[%d] = %+v, want %+v", i, added[i], tt.wantAdded[i])
				}
			}
		})
	}
}

func TestSamePath(t *testing.T) {
	if !samePath("x.opml", "x.opml") {
		t.Error("identical relative paths should be the same file")
	}
	if !samePath("x.opml", "./x.opml") {
		t.Error("paths differing only in ./ should be the same file")
	}
	if samePath("x.opml", "y.opml") {
		t.Error("different paths should not be the same file")
	}
}
