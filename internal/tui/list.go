package tui

import (
	"slices"
	"strings"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/filter"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// feedRow is one visible row of the feed list. Real feeds and query
// feeds (urls-file entries of the form "query:Name:EXPR") both produce
// rows; query feeds are virtual — nothing is fetched or stored for them.
type feedRow struct {
	feed     urls.Feed
	title    string // display title: urls title, query name or URL
	tags     []string
	query    *filter.Filter // non-nil for a query feed with a valid expression
	queryErr error          // non-nil for a query feed with a broken expression
	unread   int            // real feeds: unread articles; query feeds: matching unread
}

// articleRef is one article in the article list together with the feed
// it belongs to. All list flavors (real feed, query feed, search
// results) produce refs so read-state mutations always hit the owning
// feed's state.
type articleRef struct {
	art       feed.Article
	feedURL   string
	feedTitle string
}

// queryFeedInfo describes a query-feed urls entry: its declared name
// and its compiled expression. Exactly one of flt/err is set; the
// broken-expression case keeps the feed visible with an error marker.
type queryFeedInfo struct {
	name string
	flt  *filter.Filter
	err  error
}

// parseQueryFeed compiles a query-feed entry. ok is false for entries
// that are not query feeds at all.
func parseQueryFeed(f urls.Feed) (queryFeedInfo, bool) {
	name, expr, ok := filter.ParseQueryFeedURL(f.URL)
	if !ok {
		return queryFeedInfo{}, false
	}
	flt, err := filter.Compile(expr)
	return queryFeedInfo{name: name, flt: flt, err: err}, true
}

// feedRows builds the feed list: urls-file order, then the feed filter
// applied over a feed-level subject (title/link/tags; content is empty —
// feeds have no article content), then the configured stable sort.
func (m Model) feedRows() []feedRow {
	rows := make([]feedRow, 0, len(m.feeds))
	for _, f := range m.feeds {
		row := feedRow{feed: f, title: feedTitle(f), tags: f.Tags}
		if q, ok := parseQueryFeed(f); ok {
			if q.name != "" {
				row.title = q.name
			} else {
				row.title = f.URL
			}
			row.query, row.queryErr = q.flt, q.err
			if q.flt != nil {
				row.unread = m.queryUnread(q.flt)
			}
		} else {
			row.unread = m.states[f.URL].unread() // nil-safe receiver
		}
		rows = append(rows, row)
	}

	if m.feedFilter != nil {
		rows = slices.DeleteFunc(rows, func(r feedRow) bool {
			return !m.feedFilter.Eval(feedSubject(r))
		})
	}
	sortFeedRows(rows, m.feedSortOrder)
	return rows
}

// queryUnread counts unread articles across all real feeds matching
// the query feed's expression (the expression is evaluated over the
// unread articles, newsboat semantics).
func (m Model) queryUnread(flt *filter.Filter) int {
	n := 0
	for _, f := range realFeeds(m.feeds) {
		st := m.states[f.URL]
		if st == nil {
			continue
		}
		title := feedTitle(f)
		for _, a := range st.articles {
			if st.read[a.ID] {
				continue
			}
			if flt.Eval(filter.SubjectFromArticle(a, f.URL, title, false)) {
				n++
			}
		}
	}
	return n
}

// feedSubject adapts a feed row to a filter subject. The feed's own
// title/URL/tags are exposed both directly (title, link, tags) and as
// the feed-level attributes (feedtitle, feedlink), so expressions like
// `tags # "tech"` and `feedtitle # "blog"` both work on the feed list.
func feedSubject(r feedRow) filter.Subject {
	return filter.Subject{
		Title:     r.title,
		Link:      r.feed.URL,
		Tags:      r.tags,
		FeedTitle: r.title,
		FeedLink:  r.feed.URL,
	}
}

// feedSortOrder is the configured feed-sort-order option value.
type feedSortOrder int

const (
	feedSortNone feedSortOrder = iota // default: urls-file order
	feedSortTitle
	feedSortUnreadCount
)

func parseFeedSortOrder(v string) feedSortOrder {
	switch v {
	case "title":
		return feedSortTitle
	case "unreadcount":
		return feedSortUnreadCount
	default: // "", "none", unknown
		return feedSortNone
	}
}

// sortFeedRows sorts rows in place, stably, per the configured order.
func sortFeedRows(rows []feedRow, order feedSortOrder) {
	switch order {
	case feedSortTitle:
		slices.SortStableFunc(rows, func(a, b feedRow) int {
			return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
		})
	case feedSortUnreadCount:
		slices.SortStableFunc(rows, func(a, b feedRow) int {
			return b.unread - a.unread // most unread first
		})
	}
}

// articleSortOrder is the configured article-sort-order option value.
type articleSortOrder int

const (
	articleSortDateDesc articleSortOrder = iota // default: newest first
	articleSortDate                             // oldest first
	articleSortTitle
)

func parseArticleSortOrder(v string) articleSortOrder {
	switch v {
	case "date":
		return articleSortDate
	case "title":
		return articleSortTitle
	default: // "", "date-desc", unknown
		return articleSortDateDesc
	}
}

// sortArticleRefs sorts refs in place, stably, per the configured order.
func sortArticleRefs(refs []articleRef, order articleSortOrder) {
	switch order {
	case articleSortDate:
		slices.SortStableFunc(refs, func(a, b articleRef) int {
			return a.art.Published.Compare(b.art.Published)
		})
	case articleSortTitle:
		slices.SortStableFunc(refs, func(a, b articleRef) int {
			return strings.Compare(strings.ToLower(a.art.Title), strings.ToLower(b.art.Title))
		})
	default: // date-desc: newest first
		slices.SortStableFunc(refs, func(a, b articleRef) int {
			return b.art.Published.Compare(a.art.Published)
		})
	}
}

// articleRefs builds the article list for the active screen:
// a real feed's articles, a query feed's matches across all feeds, or
// the stored search results. The article filter and the
// show-read-articles toggle apply on top, then the configured sort.
func (m Model) articleRefs() []articleRef {
	var refs []articleRef
	if m.searchOn {
		refs = make([]articleRef, 0, len(m.searchResults))
		for _, r := range m.searchResults {
			refs = append(refs, articleRef{art: r.Article, feedURL: r.FeedURL, feedTitle: r.FeedTitle})
		}
	} else if q, ok := parseQueryFeed(m.activeFeed()); ok {
		if q.err != nil {
			return nil // broken query feed: nothing to show
		}
		refs = m.queryArticles(q.flt)
	} else {
		refs = m.feedArticles(m.active)
	}

	if m.artFilter != nil {
		refs = slices.DeleteFunc(refs, func(r articleRef) bool {
			return !m.artFilter.Eval(m.articleSubject(r))
		})
	}
	if !m.showRead {
		refs = slices.DeleteFunc(refs, func(r articleRef) bool {
			return m.readOf(r.feedURL, r.art.ID)
		})
	}
	sortArticleRefs(refs, m.articleSort)
	return refs
}

// feedArticles adapts one real feed's stored articles to refs.
func (m Model) feedArticles(feedURL string) []articleRef {
	st := m.states[feedURL]
	arts := articlesOf(st) // nil-safe
	refs := make([]articleRef, 0, len(arts))
	title := m.feedTitle(feedURL)
	for _, a := range arts {
		refs = append(refs, articleRef{art: a, feedURL: feedURL, feedTitle: title})
	}
	return refs
}

// queryArticles collects the articles matching a query feed's
// expression across all real feeds.
func (m Model) queryArticles(flt *filter.Filter) []articleRef {
	var refs []articleRef
	for _, f := range realFeeds(m.feeds) {
		title := feedTitle(f)
		for _, a := range articlesOf(m.states[f.URL]) { // nil-safe
			refs = append(refs, articleRef{art: a, feedURL: f.URL, feedTitle: title})
		}
	}
	return slices.DeleteFunc(refs, func(r articleRef) bool {
		return !flt.Eval(m.articleSubject(r))
	})
}

// articleSubject adapts an article ref to a filter subject with the
// live read state.
func (m Model) articleSubject(r articleRef) filter.Subject {
	return filter.SubjectFromArticle(r.art, r.feedURL, r.feedTitle, m.readOf(r.feedURL, r.art.ID))
}

// readOf reports the live read state of an article in its owning feed.
func (m Model) readOf(feedURL, id string) bool {
	if st := m.states[feedURL]; st != nil {
		return st.read[id]
	}
	return false
}

// activeFeed returns the urls entry the article list was opened from
// (zero value when opened from search results).
func (m Model) activeFeed() urls.Feed {
	for _, f := range m.feeds {
		if f.URL == m.active {
			return f
		}
	}
	return urls.Feed{}
}

// realFeeds returns the urls entries that are fetched and stored
// (query feeds excluded).
func realFeeds(feeds []urls.Feed) []urls.Feed {
	out := make([]urls.Feed, 0, len(feeds))
	for _, f := range feeds {
		if _, _, ok := filter.ParseQueryFeedURL(f.URL); !ok {
			out = append(out, f)
		}
	}
	return out
}

// articlesOf returns the article list (nil-safe).
func articlesOf(st *feedState) []feed.Article {
	if st == nil {
		return nil
	}
	return st.articles
}
