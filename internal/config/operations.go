package config

// Operation names accepted by `bind-key` and `macro`, with newsboat's
// exact spelling (see man newsboat, "Available Operations").
//
// This is a SUBSET of newsboat's full list (~150 operations). Gaps remain
// on purpose: the TUI wiring will request more operations as screens gain
// functionality — add them here with newsboat's spelling. newsboat's
// operation aliases are not mapped.
const (
	// General.
	OpOpen        = "open"
	OpQuit        = "quit"
	OpHardQuit    = "hard-quit"
	OpReload      = "reload"
	OpReloadAll   = "reload-all"
	OpRedraw      = "redraw"
	OpHelp        = "help"
	OpSet         = "set" // only meaningful inside macros
	OpCmdline     = "cmdline"
	OpViewDialogs = "view-dialogs"
	OpRunMacro    = "run-macro"

	// Navigation.
	OpUp             = "up"
	OpDown           = "down"
	OpPageUp         = "page-up"
	OpPageDown       = "page-down"
	OpHalfPageUp     = "halfpage-up"
	OpHalfPageDown   = "halfpage-down"
	OpNext           = "next"
	OpPrev           = "prev"
	OpNextUnread     = "next-unread"
	OpPrevUnread     = "prev-unread"
	OpNextFeed       = "next-feed"
	OpPrevFeed       = "prev-feed"
	OpNextUnreadFeed = "next-unread-feed"
	OpPrevUnreadFeed = "prev-unread-feed"
	OpRandomUnread   = "random-unread"
	OpFirst          = "first" // charss additions beyond newsboat's op set: newsboat
	OpLast           = "last"  // has home/end syskeys instead; charss binds g/G

	// Feeds and articles.
	OpOpenInBrowser                     = "open-in-browser"
	OpOpenInBrowserNoninteractively     = "open-in-browser-noninteractively"
	OpOpenAllUnreadInBrowser            = "open-all-unread-in-browser"
	OpOpenAllUnreadInBrowserAndMarkRead = "open-all-unread-in-browser-and-mark-read"
	OpSave                              = "save"
	OpSaveAll                           = "save-all"
	OpMarkFeedRead                      = "mark-feed-read"
	OpMarkAllFeedsRead                  = "mark-all-feeds-read"
	OpMarkAllAbRead                     = "mark-all-ab-read" // newsboat's odd name: mark all above as read
	OpToggleArticleRead                 = "toggle-article-read"
	OpToggleShowReadFeeds               = "toggle-show-read-feeds"
	OpToggleShowReadArticles            = "toggle-show-read-articles"
	OpToggleFlag                        = "toggle-flag"
	OpToggleSourceView                  = "toggle-source-view"
	OpShowURLs                          = "show-urls"
	OpDeleteArticle                     = "delete-article"
	OpPurgeOldArticles                  = "purge-old-articles"
	OpPurgeDeletedArticles              = "purge-deleted-articles"
	OpSort                              = "sort"
	OpSearch                            = "search"
	OpSetFilter                         = "set-filter"
	OpClearFilter                       = "clear-filter"
	OpGotoURL                           = "goto-url"
	OpEditURLs                          = "edit-urls"

	// Tags.
	OpClearTag  = "clear-tag"
	OpSelectTag = "select-tag"
	OpSetTag    = "set-tag"

	// Podcasts and downloads.
	OpEnqueue        = "enqueue"
	OpDownload       = "download"
	OpDownloadAll    = "download-all"
	OpCancelDownload = "cancel-download"
	OpDelete         = "delete"
)

// Operations is the canonical map from operation name to op string. It
// doubles as the validation set for `bind-key` targets and macro steps:
// names not present produce a warning, not an error.
//
// Alias entries (value != key) normalize newsboat's spelling of an
// operation onto charss's canonical name at parse time, so a config
// written for newsboat keeps working. The only alias today: newsboat
// calls macro execution "macro-prefix"; charss spells it "run-macro".
var Operations = map[string]string{
	OpOpen:         OpOpen,
	OpQuit:         OpQuit,
	OpHardQuit:     OpHardQuit,
	OpReload:       OpReload,
	OpReloadAll:    OpReloadAll,
	OpRedraw:       OpRedraw,
	OpHelp:         OpHelp,
	OpSet:          OpSet,
	OpCmdline:      OpCmdline,
	OpViewDialogs:  OpViewDialogs,
	OpRunMacro:     OpRunMacro,
	"macro-prefix": OpRunMacro,

	OpUp:             OpUp,
	OpDown:           OpDown,
	OpPageUp:         OpPageUp,
	OpPageDown:       OpPageDown,
	OpHalfPageUp:     OpHalfPageUp,
	OpHalfPageDown:   OpHalfPageDown,
	OpNext:           OpNext,
	OpPrev:           OpPrev,
	OpNextUnread:     OpNextUnread,
	OpPrevUnread:     OpPrevUnread,
	OpNextFeed:       OpNextFeed,
	OpPrevFeed:       OpPrevFeed,
	OpNextUnreadFeed: OpNextUnreadFeed,
	OpPrevUnreadFeed: OpPrevUnreadFeed,
	OpRandomUnread:   OpRandomUnread,
	OpFirst:          OpFirst,
	OpLast:           OpLast,

	OpOpenInBrowser:                     OpOpenInBrowser,
	OpOpenInBrowserNoninteractively:     OpOpenInBrowserNoninteractively,
	OpOpenAllUnreadInBrowser:            OpOpenAllUnreadInBrowser,
	OpOpenAllUnreadInBrowserAndMarkRead: OpOpenAllUnreadInBrowserAndMarkRead,
	OpSave:                              OpSave,
	OpSaveAll:                           OpSaveAll,
	OpMarkFeedRead:                      OpMarkFeedRead,
	OpMarkAllFeedsRead:                  OpMarkAllFeedsRead,
	OpMarkAllAbRead:                     OpMarkAllAbRead,
	OpToggleArticleRead:                 OpToggleArticleRead,
	OpToggleShowReadFeeds:               OpToggleShowReadFeeds,
	OpToggleShowReadArticles:            OpToggleShowReadArticles,
	OpToggleFlag:                        OpToggleFlag,
	OpToggleSourceView:                  OpToggleSourceView,
	OpShowURLs:                          OpShowURLs,
	OpDeleteArticle:                     OpDeleteArticle,
	OpPurgeOldArticles:                  OpPurgeOldArticles,
	OpPurgeDeletedArticles:              OpPurgeDeletedArticles,
	OpSort:                              OpSort,
	OpSearch:                            OpSearch,
	OpSetFilter:                         OpSetFilter,
	OpClearFilter:                       OpClearFilter,
	OpGotoURL:                           OpGotoURL,
	OpEditURLs:                          OpEditURLs,

	OpClearTag:  OpClearTag,
	OpSelectTag: OpSelectTag,
	OpSetTag:    OpSetTag,

	OpEnqueue:        OpEnqueue,
	OpDownload:       OpDownload,
	OpDownloadAll:    OpDownloadAll,
	OpCancelDownload: OpCancelDownload,
	OpDelete:         OpDelete,
}
