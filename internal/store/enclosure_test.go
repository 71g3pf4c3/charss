package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/71g3pf4c3/charss/internal/feed"
)

// Enclosures are plain JSON on feed.Article, so Save/Load must keep them
// without any store changes. Regression guard for the podcast queue work.
func TestSaveLoadKeepsEnclosures(t *testing.T) {
	t.Parallel()

	s, err := Open(filepath.Join(t.TempDir(), "feeds"))
	if err != nil {
		t.Fatal(err)
	}
	st := State{
		Articles: []feed.Article{
			{
				ID:    "enc-1",
				Title: "Episode 42",
				Enclosures: []feed.Enclosure{
					{URL: "https://example.com/ep42.mp3", MimeType: "audio/mpeg", Size: 123456},
					{URL: "https://example.com/ep42.ogg", MimeType: "audio/ogg", Size: -1},
				},
			},
			{ID: "no-enc", Title: "Text only"},
		},
	}
	if err := s.Save("https://example.com/feed.xml", st); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Load("https://example.com/feed.xml")
	if err != nil || !ok {
		t.Fatalf("Load: %v (present: %v)", err, ok)
	}
	if !reflect.DeepEqual(got.Articles, st.Articles) {
		t.Errorf("articles mismatch after roundtrip:\n got: %#v\nwant: %#v", got.Articles, st.Articles)
	}
}
