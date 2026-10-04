package urls

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Feed
		wantErr bool
	}{
		{
			name:  "plain url",
			input: "https://example.com/feed.xml",
			want:  []Feed{{URL: "https://example.com/feed.xml", Line: 1}},
		},
		{
			name:  "url with title and tags",
			input: `https://example.com/feed.xml "Example Feed" tech news`,
			want: []Feed{{
				URL:   "https://example.com/feed.xml",
				Title: "Example Feed",
				Tags:  []string{"tech", "news"},
				Line:  1,
			}},
		},
		{
			name:  "quoted url with spaces",
			input: `"https://example.com/some feed.xml" "Quoted"`,
			want: []Feed{{
				URL:   "https://example.com/some feed.xml",
				Title: "Quoted",
				Line:  1,
			}},
		},
		{
			name:  "comments and blanks skipped",
			input: "# a comment\n\nhttps://a.com/rss\n   \n# another\nhttps://b.com/rss \"B\"\n",
			want: []Feed{
				{URL: "https://a.com/rss", Line: 3},
				{URL: "https://b.com/rss", Title: "B", Line: 6},
			},
		},
		{
			name:  "per-feed config pairs preserved",
			input: `https://example.com/rss "T" tag "browser: chawan"`,
			want: []Feed{{
				URL:   "https://example.com/rss",
				Title: "T",
				Tags:  []string{"tag"},
				Extra: []string{"browser: chawan"},
				Line:  1,
			}},
		},
		{
			name:    "unterminated quote",
			input:   `https://example.com/rss "Title`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
