package config

// Default option values (newsboat's `browser` default is a wart away from
// our requirement: charss renders via chawan).
const (
	DefaultBrowser = "chawan"
	DefaultChafa   = "chafa"
)

// defaultOptions is the option map Load starts from; file options overwrite.
var defaultOptions = map[string]string{
	"browser": DefaultBrowser,
	"chafa":   DefaultChafa,
}

// knownOptions are the options charss itself consumes. Everything else
// found in a config file is kept in Options with a warning — newsboat
// also warns about unknown options but continues.
var knownOptions = map[string]bool{
	"browser": true,
	"chafa":   true,
}

// defaultKeys is a subset of newsboat's default key bindings for the
// contexts charss implements (newsboat man, "Default Key Bindings").
// Where newsboat binds the same op in several contexts we repeat it:
// an "all" entry would also leak into help/dialog/podcast lookups.
var defaultKeys = map[string]map[string]string{
	CtxAll: {
		"q": OpQuit,
		"?": OpHelp,
	},
	CtxFeedList: {
		"ENTER": OpOpen,
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"n":     OpNextUnread,
		"p":     OpPrevUnread,
		"r":     OpReload,
		"R":     OpReloadAll,
		"A":     OpMarkFeedRead,
		"o":     OpOpenInBrowser,
	},
	CtxArticleList: {
		"ENTER": OpOpen,
		"DOWN":  OpDown,
		"UP":    OpUp,
		"j":     OpDown,
		"k":     OpUp,
		"NPAGE": OpPageDown,
		"PPAGE": OpPageUp,
		"n":     OpNextUnread,
		"p":     OpPrevUnread,
		"r":     OpReload,
		"R":     OpReloadAll,
		"o":     OpOpenInBrowser,
		"s":     OpSave,
		"N":     OpToggleArticleRead,
		"e":     OpEnqueue,
		"D":     OpDeleteArticle,
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
	},
	CtxDialog: {},
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
