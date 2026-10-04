package config

import "testing"

// The operation set the task pinned down; each must exist with newsboat's
// exact spelling.
func TestOperationsContainNewsboatNames(t *testing.T) {
	names := []string{
		"open", "quit", "reload", "reload-all",
		"mark-feed-read", "mark-all-feeds-read", "mark-all-ab-read",
		"save", "next-unread", "prev-unread",
		"next-feed", "prev-feed",
		"next-unread-feed", "prev-unread-feed", "random-unread",
		"open-in-browser", "help",
		"toggle-source-view", "toggle-article-read",
		"toggle-show-read-feeds", "toggle-show-read-articles",
		"show-urls", "clear-tag", "select-tag", "set-tag",
		"search", "goto-url", "enqueue",
		"download", "cancel-download",
		"delete-article", "purge-old-articles", "edit-urls", "sort",
	}
	for _, name := range names {
		if got, ok := Operations[name]; !ok || got != name {
			t.Errorf("Operations[%q] = %q, ok=%v", name, got, ok)
		}
	}
}

func TestOpConstantsMatchNewsboatSpelling(t *testing.T) {
	cases := map[string]string{
		OpOpen:              "open",
		OpQuit:              "quit",
		OpMarkAllAbRead:     "mark-all-ab-read",
		OpMarkAllFeedsRead:  "mark-all-feeds-read",
		OpOpenInBrowser:     "open-in-browser",
		OpToggleArticleRead: "toggle-article-read",
		OpShowURLs:          "show-urls",
		OpGotoURL:           "goto-url",
		OpCancelDownload:    "cancel-download",
		OpDeleteArticle:     "delete-article",
		OpPurgeOldArticles:  "purge-old-articles",
		OpEditURLs:          "edit-urls",
	}
	for constant, want := range cases {
		if constant != want {
			t.Errorf("constant = %q, want %q", constant, want)
		}
	}
}
