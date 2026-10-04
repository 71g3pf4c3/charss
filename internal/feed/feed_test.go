package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/71g3pf4c3/charss/internal/urls"
)

const rssFixture = `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
  <title>Example Feed</title>
  <link>https://example.com/</link>
  <description>example</description>
  <item>
    <title>First</title>
    <link>https://example.com/1</link>
    <guid isPermaLink="false">guid-1</guid>
    <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
    <description>short summary</description>
    <content:encoded><![CDATA[<p>rich content</p>]]></content:encoded>
  </item>
  <item>
    <title>Second</title>
    <link>https://example.com/2</link>
    <description>only a description</description>
  </item>
  <item>
    <title>Third</title>
    <link>https://example.com/3</link>
    <guid isPermaLink="false">guid-3</guid>
    <description>podcast episode</description>
    <enclosure url="https://example.com/audio/ep3.mp3" type="audio/mpeg" length="123"/>
    <enclosure url="https://example.com/audio/ep3.ogg" type="audio/ogg"/>
    <enclosure url="https://example.com/audio/ep3-bad.mp3" type="audio/mpeg" length="not-a-number"/>
  </item>
</channel>
</rss>`

const atomFixture = `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <id>urn:uuid:feed</id>
  <updated>2020-01-01T00:00:00Z</updated>
  <entry>
    <title>Entry One</title>
    <id>urn:uuid:entry-1</id>
    <link href="https://example.com/e1"/>
    <author><name>Alice</name></author>
    <published>2006-01-02T15:04:05Z</published>
    <content type="html">&lt;p&gt;hello&lt;/p&gt;</content>
  </entry>
</feed>`

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func feedFor(ts *httptest.Server) urls.Feed {
	return urls.Feed{URL: ts.URL, Title: "test", Line: 1}
}

func TestFetchIfModified(t *testing.T) {
	t.Parallel()

	rssWant := []Article{
		{
			ID:          hashOf("guid-1"),
			GUID:        "guid-1",
			Title:       "First",
			URL:         "https://example.com/1",
			ContentHTML: "<p>rich content</p>",
			Published:   time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
		},
		{
			ID:          hashOf("https://example.com/2"),
			Title:       "Second",
			URL:         "https://example.com/2",
			ContentHTML: "only a description",
		},
		{
			ID:          hashOf("guid-3"),
			GUID:        "guid-3",
			Title:       "Third",
			URL:         "https://example.com/3",
			ContentHTML: "podcast episode",
			Enclosures: []Enclosure{
				{URL: "https://example.com/audio/ep3.mp3", MimeType: "audio/mpeg", Size: 123},
				{URL: "https://example.com/audio/ep3.ogg", MimeType: "audio/ogg", Size: -1},      // no length attr
				{URL: "https://example.com/audio/ep3-bad.mp3", MimeType: "audio/mpeg", Size: -1}, // invalid length
			},
		},
	}
	atomWant := []Article{
		{
			ID:          hashOf("urn:uuid:entry-1"),
			GUID:        "urn:uuid:entry-1",
			Title:       "Entry One",
			URL:         "https://example.com/e1",
			ContentHTML: "<p>hello</p>",
			Author:      "Alice",
			Published:   time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
		},
	}

	cases := []struct {
		name          string
		handler       http.HandlerFunc
		opts          []Option
		etag          string
		lastModified  string
		cancelContext bool
		check         func(t *testing.T, got Fetched, err error)
	}{
		{
			name: "rss 2.0 happy path",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"etag-1"`)
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.Header().Set("Content-Type", "application/rss+xml")
				_, _ = w.Write([]byte(rssFixture))
			},
			check: func(t *testing.T, got Fetched, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got.Articles, rssWant) {
					t.Errorf("articles mismatch:\n got: %#v\nwant: %#v", got.Articles, rssWant)
				}
				if got.ETag != `"etag-1"` {
					t.Errorf("ETag = %q, want %q", got.ETag, `"etag-1"`)
				}
				if got.LastModified != "Mon, 02 Jan 2006 15:04:05 GMT" {
					t.Errorf("LastModified = %q", got.LastModified)
				}
				if got.LastFetched.IsZero() {
					t.Error("LastFetched is zero")
				}
				if got.Feed.URL == "" || got.Feed.Title != "test" {
					t.Errorf("Feed not echoed back: %#v", got.Feed)
				}
			},
		},
		{
			name: "atom happy path",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/atom+xml")
				_, _ = w.Write([]byte(atomFixture))
			},
			check: func(t *testing.T, got Fetched, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got.Articles, atomWant) {
					t.Errorf("articles mismatch:\n got: %#v\nwant: %#v", got.Articles, atomWant)
				}
			},
		},
		{
			name: "304 not modified",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("If-None-Match") == `"etag-1"` {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				_, _ = w.Write([]byte(rssFixture))
			},
			etag: `"etag-1"`,
			check: func(t *testing.T, got Fetched, err error) {
				if !errors.Is(err, ErrNotModified) {
					t.Fatalf("error = %v, want ErrNotModified", err)
				}
				if len(got.Articles) != 0 {
					t.Errorf("articles = %d, want 0 on 304", len(got.Articles))
				}
			},
		},
		{
			name:    "404 is HTTPError",
			handler: http.NotFound,
			check: func(t *testing.T, got Fetched, err error) {
				var he *HTTPError
				if !errors.As(err, &he) {
					t.Fatalf("error = %v, want *HTTPError", err)
				}
				if he.Status != http.StatusNotFound {
					t.Errorf("HTTPError.Status = %d, want 404", he.Status)
				}
				if got.Feed.URL != "" {
					t.Errorf("Fetched should be zero on error, got %#v", got)
				}
			},
		},
		{
			name: "malformed xml is ParseError",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<rss version=\"2.0\"><channel><title>broken"))
			},
			check: func(t *testing.T, got Fetched, err error) {
				var pe *ParseError
				if !errors.As(err, &pe) {
					t.Fatalf("error = %v, want *ParseError", err)
				}
				var he *HTTPError
				if errors.As(err, &he) {
					t.Error("parse failure must not be an HTTPError")
				}
			},
		},
		{
			name: "timeout is a network error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(300 * time.Millisecond)
				_, _ = w.Write([]byte(rssFixture))
			},
			opts: []Option{WithTimeout(50 * time.Millisecond)},
			check: func(t *testing.T, got Fetched, err error) {
				if err == nil {
					t.Fatal("expected timeout error, got nil")
				}
				var he *HTTPError
				var pe *ParseError
				if errors.As(err, &he) || errors.As(err, &pe) {
					t.Fatalf("timeout surfaced as wrong type: %v", err)
				}
				var ue *url.Error
				if !errors.As(err, &ue) || !ue.Timeout() {
					t.Errorf("error = %v, want a timeout *url.Error", err)
				}
				if !strings.Contains(err.Error(), "feed ") {
					t.Errorf("error should mention the feed URL: %v", err)
				}
			},
		},
		{
			name: "canceled context",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(rssFixture))
			},
			cancelContext: true,
			check: func(t *testing.T, got Fetched, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v, want context.Canceled", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			ctx := context.Background()
			if tc.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			f := NewFetcher(tc.opts...)
			got, err := f.FetchIfModified(ctx, feedFor(ts), tc.etag, tc.lastModified)
			tc.check(t, got, err)
		})
	}
}

func TestConditionalHeaders(t *testing.T) {
	t.Parallel()

	var gotIfNoneMatch, gotIfModifiedSince string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		gotIfModifiedSince = r.Header.Get("If-Modified-Since")
		_, _ = w.Write([]byte(rssFixture))
	}))
	defer ts.Close()

	f := NewFetcher()
	if _, err := f.Fetch(context.Background(), feedFor(ts)); err != nil {
		t.Fatalf("plain fetch: %v", err)
	}
	if gotIfNoneMatch != "" || gotIfModifiedSince != "" {
		t.Errorf("plain fetch sent conditional headers: %q / %q", gotIfNoneMatch, gotIfModifiedSince)
	}

	if _, err := f.FetchIfModified(context.Background(), feedFor(ts), `"etag-x"`, "Tue, 03 Jan 2006 00:00:00 GMT"); err != nil {
		t.Fatalf("conditional fetch: %v", err)
	}
	if gotIfNoneMatch != `"etag-x"` {
		t.Errorf("If-None-Match = %q, want %q", gotIfNoneMatch, `"etag-x"`)
	}
	if gotIfModifiedSince != "Tue, 03 Jan 2006 00:00:00 GMT" {
		t.Errorf("If-Modified-Since = %q", gotIfModifiedSince)
	}
}

func TestUserAgent(t *testing.T) {
	t.Parallel()

	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(rssFixture))
	}))
	defer ts.Close()

	// Default: "charss/<version>" (or bare "charss" when version is empty).
	if _, err := NewFetcher().Fetch(context.Background(), feedFor(ts)); err != nil {
		t.Fatalf("default UA fetch: %v", err)
	}
	if !strings.HasPrefix(gotUA, "charss/") && gotUA != "charss" {
		t.Errorf("default User-Agent = %q, want charss/<version>", gotUA)
	}

	if _, err := NewFetcher(WithUserAgent("test-agent/1")).Fetch(context.Background(), feedFor(ts)); err != nil {
		t.Fatalf("custom UA fetch: %v", err)
	}
	if gotUA != "test-agent/1" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "test-agent/1")
	}
}

func TestArticleIDStability(t *testing.T) {
	t.Parallel()

	// GUID wins over link.
	if got, want := articleID("guid-1", "https://x/1"), hashOf("guid-1"); got != want {
		t.Errorf("articleID(guid, link) = %s, want %s", got, want)
	}
	if got, want := articleID("", "https://x/1"), hashOf("https://x/1"); got != want {
		t.Errorf("articleID(\"\", link) = %s, want %s", got, want)
	}

	// Same feed fetched twice yields identical IDs.
	var articles [][]Article
	for i := 0; i < 2; i++ {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(rssFixture))
		}))
		got, err := NewFetcher().Fetch(context.Background(), feedFor(ts))
		ts.Close()
		if err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
		articles = append(articles, got.Articles)
	}
	if !reflect.DeepEqual(articles[0], articles[1]) {
		t.Error("article IDs not stable across fetches")
	}
}
