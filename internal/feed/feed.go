// Package feed fetches and parses RSS/Atom feeds.
//
// Fetching supports conditional requests: pass a previously stored ETag /
// Last-Modified via FetchIfModified and the server may answer 304, in which
// case ErrNotModified is returned. Parse and HTTP failures are reported as
// typed errors (ParseError, HTTPError) so callers can distinguish them from
// network errors.
package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/71g3pf4c3/charss/internal/urls"
	"github.com/71g3pf4c3/charss/internal/version"
)

// Article is one parsed feed item.
type Article struct {
	ID          string    `json:"id"` // stable hash of GUID (preferred) or URL
	GUID        string    `json:"guid,omitempty"`
	Title       string    `json:"title,omitempty"`
	URL         string    `json:"url,omitempty"`          // link
	ContentHTML string    `json:"content_html,omitempty"` // content:encoded, or description if richer/only
	Author      string    `json:"author,omitempty"`
	Published   time.Time `json:"published,omitzero"`
}

// Fetched is the result of a successful (or 304) fetch.
type Fetched struct {
	Feed         urls.Feed
	Articles     []Article
	ETag         string
	LastModified string // raw header value
	LastFetched  time.Time
}

// ErrNotModified is returned when the server answered 304 Not Modified.
// The returned Fetched has no articles; its ETag/LastModified carry the
// values from the 304 response headers (possibly empty — keep the stored
// ones in that case).
var ErrNotModified = errors.New("feed: not modified")

// HTTPError is a non-2xx HTTP response (304 excepted, see ErrNotModified).
type HTTPError struct {
	URL    string
	Status int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("feed %s: HTTP %d %s", e.URL, e.Status, http.StatusText(e.Status))
}

// ParseError is a feed body that could not be parsed as RSS/Atom.
type ParseError struct {
	URL string
	Err error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("feed %s: parse: %v", e.URL, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// Fetcher fetches feeds over HTTP and parses them with gofeed.
type Fetcher struct {
	client  *http.Client
	timeout time.Duration
	ua      string
}

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithHTTPClient uses a custom HTTP client instead of the default one.
func WithHTTPClient(c *http.Client) Option {
	return func(f *Fetcher) {
		if c != nil {
			f.client = c
		}
	}
}

// WithTimeout sets the per-request timeout. Applied to both the default and
// a caller-provided client (the caller's client is not mutated).
func WithTimeout(d time.Duration) Option {
	return func(f *Fetcher) {
		if d > 0 {
			f.timeout = d
		}
	}
}

// WithUserAgent overrides the default "charss/<version>" User-Agent.
// An empty string keeps the default.
func WithUserAgent(ua string) Option {
	return func(f *Fetcher) {
		if ua != "" {
			f.ua = ua
		}
	}
}

// NewFetcher returns a Fetcher with the given options applied.
func NewFetcher(opts ...Option) *Fetcher {
	f := &Fetcher{ua: defaultUserAgent()}
	for _, opt := range opts {
		if opt != nil {
			opt(f)
		}
	}
	if f.client == nil {
		f.client = &http.Client{}
	}
	if f.timeout > 0 {
		c := *f.client // shallow copy: never mutate a caller-provided client
		c.Timeout = f.timeout
		f.client = &c
	}
	return f
}

func defaultUserAgent() string {
	ver, _, _ := version.Info()
	if ver == "" {
		return "charss"
	}
	return "charss/" + ver
}

// Fetch fetches the feed unconditionally.
func (f *Fetcher) Fetch(ctx context.Context, feed urls.Feed) (Fetched, error) {
	return f.FetchIfModified(ctx, feed, "", "")
}

// FetchIfModified fetches the feed, sending If-None-Match / If-Modified-Since
// when etag / lastModified are non-empty. On a 304 it returns ErrNotModified
// with zero articles.
func (f *Fetcher) FetchIfModified(ctx context.Context, feed urls.Feed, etag, lastModified string) (Fetched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return Fetched{}, fmt.Errorf("feed %s: %w", feed.URL, err)
	}
	req.Header.Set("User-Agent", f.ua)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return Fetched{}, fmt.Errorf("feed %s: %w", feed.URL, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		return Fetched{
			Feed:         feed,
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			LastFetched:  time.Now().UTC(),
		}, ErrNotModified
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Fetched{}, &HTTPError{URL: feed.URL, Status: resp.StatusCode}
	}

	parsed, err := gofeed.NewParser().Parse(resp.Body)
	if err != nil {
		return Fetched{}, &ParseError{URL: feed.URL, Err: err}
	}

	return Fetched{
		Feed:         feed,
		Articles:     articlesFrom(parsed),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		LastFetched:  time.Now().UTC(),
	}, nil
}

func articlesFrom(f *gofeed.Feed) []Article {
	if f == nil || len(f.Items) == 0 {
		return nil
	}
	articles := make([]Article, 0, len(f.Items))
	for _, item := range f.Items {
		if item == nil {
			continue
		}
		a := Article{
			GUID:  item.GUID,
			Title: item.Title,
			URL:   item.Link,
		}
		a.ContentHTML = item.Content
		if a.ContentHTML == "" {
			a.ContentHTML = item.Description
		}
		a.Author = authorOf(item)
		switch {
		case item.PublishedParsed != nil:
			a.Published = *item.PublishedParsed
		case item.UpdatedParsed != nil:
			a.Published = *item.UpdatedParsed
		}
		a.ID = articleID(a.GUID, a.URL)
		articles = append(articles, a)
	}
	return articles
}

func authorOf(item *gofeed.Item) string {
	if p := item.Author; p != nil {
		if p.Name != "" {
			return p.Name
		}
		if p.Email != "" {
			return p.Email
		}
	}
	for _, p := range item.Authors {
		if p != nil && p.Name != "" {
			return p.Name
		}
	}
	return ""
}

// articleID returns a stable hash of the GUID (preferred) or the item URL.
func articleID(guid, link string) string {
	key := guid
	if key == "" {
		key = link
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
