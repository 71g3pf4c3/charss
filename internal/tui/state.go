package tui

import (
	"slices"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/store"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// feedState is the in-memory state of one feed: the article list (newest
// first), per-article read flags, per-article flag chars and the
// conditional-request validators. It is always held behind a pointer so
// Update's value-copy of the Model still mutates the same state.
type feedState struct {
	articles     []feed.Article // newest first
	read         map[string]bool
	flags        map[string]string // article ID -> flag chars, e.g. "aZ"
	etag         string
	lastModified string
	lastFetched  time.Time
}

// unread counts articles not marked read. Safe on a nil receiver (feed
// with no state yet).
func (s *feedState) unread() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, a := range s.articles {
		if !s.read[a.ID] {
			n++
		}
	}
	return n
}

// toStore converts to the persisted representation.
func (s *feedState) toStore() store.State {
	return store.State{
		ETag:         s.etag,
		LastModified: s.lastModified,
		LastFetched:  s.lastFetched,
		Articles:     s.articles,
		Read:         s.read,
		Flags:        s.flags,
	}
}

// stateFromStore rebuilds in-memory state, sorting articles newest first
// defensively (the store preserves our ordering, but do not rely on it).
func stateFromStore(ss store.State) *feedState {
	arts := slices.Clone(ss.Articles)
	slices.SortStableFunc(arts, func(a, b feed.Article) int {
		return b.Published.Compare(a.Published)
	})
	read := ss.Read
	if read == nil {
		read = make(map[string]bool)
	}
	return &feedState{
		articles:     arts,
		read:         read,
		flags:        ss.Flags,
		etag:         ss.ETag,
		lastModified: ss.LastModified,
		lastFetched:  ss.LastFetched,
	}
}

// pruneFlags drops flag entries for articles that fell out of the list
// (the read map gets the same treatment inside mergeArticles).
func pruneFlags(flags map[string]string, articles []feed.Article) map[string]string {
	if len(flags) == 0 {
		return nil
	}
	out := make(map[string]string, len(flags))
	for _, a := range articles {
		if f := flags[a.ID]; f != "" {
			out[a.ID] = f
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// statesLoadedMsg carries the locally cached state of all feeds. Loading
// from disk is fast, so this is one message, not one per feed.
type statesLoadedMsg struct {
	states map[string]*feedState
	errs   []error // per-feed load failures; non-fatal
}

// loadStatesCmd reads every feed's cached state from the store. It only
// touches local disk and is safe to run as the first Init command — the
// first render never waits on the network.
func loadStatesCmd(s Storer, feeds []urls.Feed) tea.Cmd {
	return func() tea.Msg {
		msg := statesLoadedMsg{states: make(map[string]*feedState, len(feeds))}
		for _, f := range feeds {
			ss, ok, err := s.Load(f.URL)
			if err != nil {
				msg.errs = append(msg.errs, err)
				continue
			}
			if !ok {
				continue
			}
			msg.states[f.URL] = stateFromStore(ss)
		}
		return msg
	}
}
