package urls

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		feed Feed
		want string
	}{
		{
			name: "plain url",
			feed: Feed{URL: "https://example.com/feed.xml"},
			want: "https://example.com/feed.xml",
		},
		{
			name: "url with title and tags",
			feed: Feed{URL: "https://example.com/feed.xml", Title: "Example Feed", Tags: []string{"tech", "news"}},
			want: `https://example.com/feed.xml "Example Feed" tech news`,
		},
		{
			name: "url with spaces is quoted",
			feed: Feed{URL: "https://example.com/some feed.xml", Title: "Quoted"},
			want: `"https://example.com/some feed.xml" "Quoted"`,
		},
		{
			name: "per-feed config pairs are quoted",
			feed: Feed{URL: "https://example.com/rss", Title: "T", Tags: []string{"tag"}, Extra: []string{"browser: chawan"}},
			want: `https://example.com/rss "T" tag "browser: chawan"`,
		},
		{
			name: "empty title omitted",
			feed: Feed{URL: "https://example.com/rss", Tags: []string{"a"}},
			want: "https://example.com/rss a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.feed); got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		feed Feed
	}{
		{
			name: "plain url",
			feed: Feed{URL: "https://example.com/feed.xml"},
		},
		{
			name: "title with spaces and unicode",
			feed: Feed{URL: "https://example.com/feed.xml", Title: "Лента новостей: tech & science"},
		},
		{
			name: "tags and extras",
			feed: Feed{URL: "https://example.com/rss", Title: "T", Tags: []string{"tech", "news"}, Extra: []string{"browser: chawan", "filter: foo"}},
		},
		{
			name: "url with spaces",
			feed: Feed{URL: "https://example.com/some feed.xml", Title: "Quoted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := Format(tt.feed)
			feeds, err := Parse(line)
			if err != nil {
				t.Fatalf("Parse(Format(feed)) error: %v", err)
			}
			if len(feeds) != 1 {
				t.Fatalf("Parse(Format(feed)) returned %d feeds, want 1 (line was %q)", len(feeds), line)
			}
			got := feeds[0]
			if got.URL != tt.feed.URL {
				t.Errorf("URL = %q, want %q", got.URL, tt.feed.URL)
			}
			if got.Title != tt.feed.Title {
				t.Errorf("Title = %q, want %q", got.Title, tt.feed.Title)
			}
			if strings.Join(got.Tags, "\x00") != strings.Join(tt.feed.Tags, "\x00") {
				t.Errorf("Tags = %q, want %q", got.Tags, tt.feed.Tags)
			}
			if strings.Join(got.Extra, "\x00") != strings.Join(tt.feed.Extra, "\x00") {
				t.Errorf("Extra = %q, want %q", got.Extra, tt.feed.Extra)
			}
		})
	}
}
