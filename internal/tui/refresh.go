package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/store"
	"github.com/71g3pf4c3/charss/internal/urls"
)

const (
	// fetchTimeout bounds one feed refresh (network + parse).
	fetchTimeout = 30 * time.Second
	// maxConcurrentFetches bounds refresh-all fan-out.
	maxConcurrentFetches = 4
)

// refreshedMsg is the result of one feed's refresh. err is nil on
// success, feed.ErrNotModified on 304, or the fetch/parse error.
type refreshedMsg struct {
	feed urls.Feed
	res  feed.Fetched
	err  error
}

// stateSavedMsg reports the outcome of a background store save.
type stateSavedMsg struct {
	feedURL string
	err     error
}

// mergeArticles merges a freshly fetched article set into the stored one.
//
// Articles are deduplicated by ID with the fresh version winning (it has
// up-to-date content fields); articles only present in old are kept. The
// result is ordered newest first (zero Published sorts last, ties keep
// their relative order). Read flags survive for articles still present
// and are pruned otherwise; articles that are new default to unread.
func mergeArticles(old, fresh []feed.Article, read map[string]bool) ([]feed.Article, map[string]bool) {
	articles := make([]feed.Article, 0, len(old)+len(fresh))
	seen := make(map[string]struct{}, len(old)+len(fresh))
	for _, list := range [][]feed.Article{fresh, old} {
		for _, a := range list {
			if _, dup := seen[a.ID]; dup {
				continue
			}
			seen[a.ID] = struct{}{}
			articles = append(articles, a)
		}
	}
	slices.SortStableFunc(articles, func(a, b feed.Article) int {
		return b.Published.Compare(a.Published) // newest first
	})

	mergedRead := make(map[string]bool, len(articles))
	for _, a := range articles {
		if read[a.ID] {
			mergedRead[a.ID] = true
		}
	}
	return articles, mergedRead
}

// refreshFeedCmd fetches one feed, sending the stored validators so the
// server can answer 304. It performs no merge and no save: the model owns
// state, Update merges against the then-current state (so read toggles
// racing the fetch are not lost in memory) and issues the save.
//
// sem bounds concurrency across a refresh-all batch; nil means unbounded.
func refreshFeedCmd(f Fetcher, sem chan struct{}, f2 urls.Feed, etag, lastModified string) tea.Cmd {
	return func() tea.Msg {
		if sem != nil {
			sem <- struct{}{}
			defer func() { <-sem }()
		}
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		res, err := f.FetchIfModified(ctx, f2, etag, lastModified)
		return refreshedMsg{feed: f2, res: res, err: err}
	}
}

// saveStateCmd persists a snapshot of one feed's state. The snapshot is
// taken in Update, the single place that mutates state; a read toggle
// landing between merge and save can still race this write on disk, but
// the in-memory state stays correct and the next toggle rewrites it.
func saveStateCmd(s Storer, feedURL string, st store.State) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return nil
		}
		if err := s.Save(feedURL, st); err != nil {
			return stateSavedMsg{feedURL: feedURL, err: err}
		}
		return nil
	}
}

// applyRefreshed folds one refresh result into the model state and
// returns the save command.
//
// While a batch refresh is still in flight (m.refreshing > 0) no status is
// written: per-feed messages would spam the status line and could overwrite
// an unrelated error (e.g. a browser-not-found from opening an article).
// Progress is shown by the header badge; the final feed's result becomes
// the status message.
func (m Model) applyRefreshed(msg refreshedMsg) (tea.Model, tea.Cmd) {
	if m.refreshing > 0 {
		m.refreshing--
	}
	title := feedTitle(msg.feed)
	st := m.stateFor(msg.feed.URL)

	switch {
	case errors.Is(msg.err, feed.ErrNotModified):
		if msg.res.ETag != "" {
			st.etag = msg.res.ETag
		}
		if msg.res.LastModified != "" {
			st.lastModified = msg.res.LastModified
		}
		st.lastFetched = msg.res.LastFetched
		if m.refreshing == 0 {
			m.batchDoneStatus(fmt.Sprintf("%s: up to date", title))
		}

	case msg.err != nil:
		// Fetch errors are sticky: they survive the rest of the batch.
		// Nothing changed; keep the cached articles and report.
		m.status = fmt.Sprintf("%s: %v", title, msg.err)
		if m.refreshing == 0 {
			m.batchNew = 0 // the batch ends with this error
		}
		return m, nil

	default:
		m.batchNew += countNewArticles(st.articles, msg.res.Articles)
		st.articles, st.read = mergeArticles(st.articles, msg.res.Articles, st.read)
		st.flags = pruneFlags(st.flags, st.articles)
		st.etag = msg.res.ETag
		st.lastModified = msg.res.LastModified
		st.lastFetched = msg.res.LastFetched
		if m.refreshing == 0 {
			m.batchDoneStatus(fmt.Sprintf("%s: %d articles", title, len(st.articles)))
		}
	}

	return m, saveStateCmd(m.store, msg.feed.URL, st.toStore())
}

// batchDoneStatus closes out a refresh batch: when notify-screen is on
// and the batch brought new articles, the notification wins over the
// final feed's status (noise-capped: a batch with no new articles never
// notifies). Either way the counter resets for the next batch.
func (m *Model) batchDoneStatus(normal string) {
	if m.batchNew > 0 && m.notifyScreen {
		m.status = fmt.Sprintf("%d new articles", m.batchNew)
	} else {
		m.status = normal
	}
	m.batchNew = 0
}

// feedTitle resolves a feed's display title: the urls-file title, or the
// URL when unset.
func feedTitle(f urls.Feed) string {
	if f.Title != "" {
		return f.Title
	}
	return f.URL
}
