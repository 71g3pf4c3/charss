package filter

import (
	"reflect"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/feed"
)

func TestSubjectFromArticle(t *testing.T) {
	published := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	art := feed.Article{
		ID:          "id-1",
		GUID:        "guid-1",
		Title:       "Title",
		URL:         "https://example.com/a",
		ContentHTML: "<p>body</p>",
		Author:      "Author",
		Published:   published,
	}

	t.Run("unread", func(t *testing.T) {
		s := SubjectFromArticle(art, "https://example.com/feed.xml", "Feed", false)
		want := Subject{
			Title:       "Title",
			Link:        "https://example.com/a",
			Author:      "Author",
			GUID:        "guid-1",
			ContentHTML: "<p>body</p>",
			Unread:      true,
			Published:   published,
			FeedTitle:   "Feed",
			FeedLink:    "https://example.com/feed.xml",
		}
		if !reflect.DeepEqual(s, want) {
			t.Errorf("SubjectFromArticle(read=false) = %+v, want %+v", s, want)
		}
	})

	t.Run("read", func(t *testing.T) {
		s := SubjectFromArticle(art, "https://example.com/feed.xml", "Feed", true)
		if s.Unread {
			t.Error("Unread = true for a read article, want false")
		}
	})

	t.Run("eval through adapter", func(t *testing.T) {
		f, err := Compile(`unread = "no" and title # "tit" and feedtitle = "feed"`)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		s := SubjectFromArticle(art, "https://example.com/feed.xml", "Feed", true)
		if !f.Eval(s) {
			t.Error("Eval through SubjectFromArticle = false, want true")
		}
	})
}
