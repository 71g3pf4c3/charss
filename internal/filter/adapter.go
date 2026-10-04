package filter

import "github.com/71g3pf4c3/charss/internal/feed"

// SubjectFromArticle adapts an internal/feed article to a Subject.
//
// feedURL and feedTitle become feedlink and feedtitle. read is the stored
// read-state of the article and maps to unread = !read.
//
// Fields with no counterpart in feed.Article are left zero: Tags (urls-file
// tags and per-article tags are not modeled yet — callers that have them
// should construct a Subject directly), Flags, and FeedDate. A zero FeedDate
// behaves as an empty string in comparisons (see the package doc).
func SubjectFromArticle(art feed.Article, feedURL, feedTitle string, read bool) Subject {
	return Subject{
		Title:       art.Title,
		Link:        art.URL,
		Author:      art.Author,
		GUID:        art.GUID,
		ContentHTML: art.ContentHTML,
		Unread:      !read,
		Published:   art.Published,
		Tags:        nil,
		FeedTitle:   feedTitle,
		FeedLink:    feedURL,
	}
}
