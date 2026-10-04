package config

// Default option values (newsboat's `browser` default is a wart away from
// our requirement: charss renders via chawan).
const (
	DefaultBrowser = "chawan"
	DefaultChafa   = "chafa"
)

// defaultOptions is the option map Load starts from; file options overwrite.
// auto-reload / reload-time / notify-screen carry newsboat's defaults.
var defaultOptions = map[string]string{
	"browser":       DefaultBrowser,
	"chafa":         DefaultChafa,
	"auto-reload":   "no",
	"reload-time":   "60",
	"notify-screen": "no",
}

// knownOptions are the options charss itself consumes. Everything else
// found in a config file is kept in Options with a warning — newsboat
// also warns about unknown options but continues.
var knownOptions = map[string]bool{
	"browser":            true,
	"chafa":              true,
	"feed-sort-order":    true,
	"article-sort-order": true,
	"show-read-articles": true,
	"auto-reload":        true,
	"reload-time":        true,
	"notify-screen":      true,
}

// defaultKeys is a subset of newsboat's default key bindings for the
// contexts charss implements (newsboat man, "Default Key Bindings").
// Where newsboat binds the same op in several contexts we repeat it:
// an "all" entry would also leak into help/dialog/podcast lookups.
var defaultKeys = map[string]map[string]string{
	CtxAll: {
		"q":  OpQuit,
		"?":  OpHelp,
		"^L": OpRedraw,
		// first/last live in "all" (like newsboat's home/end syskeys)
		// so a bare `bind-key g <op>` can override them from a config
		// file — per-context defaults would shadow all-context
		// directives in Bindings.Lookup.
		"g":    OpFirst,
		"G":    OpLast,
		"HOME": OpFirst,
		"END":  OpLast,
		// macro execution lives in "all" too: newsboat binds its
		// macro-prefix "," in every newsboat context.
		",": OpRunMacro,
	},
	CtxFeedList: {
		"ENTER": OpOpen,
		"l":     OpOpen,
		"RIGHT": OpOpen,
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"J":     OpNext,
		"K":     OpPrev,
		"n":     OpNextUnread,
		"p":     OpPrevUnread,
		"r":     OpReload,
		"R":     OpReloadAll,
		"A":     OpMarkFeedRead,
		"C":     OpMarkAllFeedsRead,
		"o":     OpOpenInBrowser,
		"/":     OpSearch,
		"F":     OpSetFilter,
		"^F":    OpClearFilter,
	},
	CtxArticleList: {
		"ENTER": OpOpen,
		"l":     OpOpen,
		"RIGHT": OpOpen,
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"J":     OpNext,
		"K":     OpPrev,
		"n":     OpNextUnread,
		"p":     OpPrevUnread,
		"r":     OpReload,
		"R":     OpReloadAll,
		"A":     OpMarkFeedRead,
		"o":     OpOpenInBrowser,
		"s":     OpSave,
		"u":     OpShowURLs,
		"N":     OpToggleArticleRead,
		"m":     OpToggleArticleRead,
		"^":     OpToggleFlag,
		"D":     OpDeleteArticle,
		"e":     OpEnqueue,
		"/":     OpSearch,
		"F":     OpSetFilter,
		"^F":    OpClearFilter,
		"ESC":   OpQuit,
		"h":     OpQuit,
		"LEFT":  OpQuit,
	},
	CtxArticle: {
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"n":     OpNextUnread,
		"p":     OpPrevUnread,
		"o":     OpOpenInBrowser,
		"s":     OpSave,
		"u":     OpShowURLs,
	},
	CtxHelp: {
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"ESC":   OpQuit,
	},
	// CtxDialog is also the binding context of the show-urls URL view
	// (newsboat has a dedicated "urlview" context with the same
	// navigation defaults).
	CtxDialog: {
		"ENTER": OpOpen,
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"ESC":   OpQuit,
	},
	CtxPodcast: {
		"DOWN": OpDown,
		"UP":   OpUp,
		"j":    OpDown,
		"k":    OpUp,
	},
}

// defaultBindings returns a fresh copy of the default bindings. Mutating
// the result does not affect subsequent calls.
func defaultBindings() Bindings {
	b := newBindings()
	for ctx, keys := range defaultKeys {
		for key, op := range keys {
			b.set(ctx, key, op)
		}
	}
	return b
}

// defaultOptionsCopy returns a copy of the default option map.
func defaultOptionsCopy() map[string]string {
	m := make(map[string]string, len(defaultOptions))
	for k, v := range defaultOptions {
		m[k] = v
	}
	return m
}
