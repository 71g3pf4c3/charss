package config

// Key-binding contexts, mirroring newsboat's dialog contexts. Bindings
// declared for "all" apply to every context as a fallback.
const (
	CtxAll         = "all"
	CtxFeedList    = "feedlist"
	CtxArticleList = "articlelist"
	CtxArticle     = "article"
	CtxHelp        = "help"
	CtxDialog      = "dialog"
	CtxPodcast     = "podcast"
)

var knownContexts = map[string]bool{
	CtxAll:         true,
	CtxFeedList:    true,
	CtxArticleList: true,
	CtxArticle:     true,
	CtxHelp:        true,
	CtxDialog:      true,
	CtxPodcast:     true,
}

// Bindings holds the resolved key bindings, one map per context plus an
// "all" fallback bucket. A key present in a context's map with an empty
// operation is an explicit per-context unbind that shadows the "all"
// fallback. Bindings is created by Load (defaults plus the parsed
// bind-key/unbind-key directives, in file order); the zero value is
// usable and simply has nothing bound.
type Bindings struct {
	contexts map[string]map[string]string // context -> key -> operation
}

// Lookup returns the operation bound to key in context. A context-specific
// binding wins over one declared for "all"; "" if the key is unbound
// (including explicitly unbound per context). Key names are case-sensitive
// (newsboat names like ENTER, SPACE, NPAGE are uppercase).
func (b Bindings) Lookup(context, key string) string {
	if op, ok := b.contexts[context][key]; ok {
		return op // ok with empty op: explicit unbind shadows the "all" fallback
	}
	return b.contexts[CtxAll][key] // nil-map lookup on unbound context: ""
}

// set binds key to op in context (creating the context bucket if needed).
// An "all" binding applies to every context, so it also clears explicit
// per-context unbinds of the same key (newsboat fans "all" bindings out
// into every context, overriding earlier unbinds).
func (b *Bindings) set(context, key, op string) {
	if b.contexts == nil {
		b.contexts = map[string]map[string]string{}
	}
	if context == CtxAll {
		for ctx, m := range b.contexts {
			if ctx != CtxAll {
				if v, ok := m[key]; ok && v == "" {
					delete(m, key)
				}
			}
		}
	}
	m, ok := b.contexts[context]
	if !ok {
		m = map[string]string{}
		b.contexts[context] = m
	}
	m[key] = op
}

// unset removes key's binding in context. Context "all" removes the key
// from every context (newsboat's `unbind-key <key> all` meaning); a
// specific context records an explicit unbind that shadows the "all"
// fallback.
func (b *Bindings) unset(key, context string) {
	if b.contexts == nil {
		b.contexts = map[string]map[string]string{}
	}
	if context == CtxAll {
		for _, m := range b.contexts {
			delete(m, key)
		}
		return
	}
	m, ok := b.contexts[context]
	if !ok {
		m = map[string]string{}
		b.contexts[context] = m
	}
	m[key] = ""
}

// clear removes every binding in every context (bare `unbind-key`).
func (b *Bindings) clear() {
	b.contexts = map[string]map[string]string{}
}

// apply replays parsed binding directives in order onto b.
func (b *Bindings) apply(dirs []bindingDirective) {
	for _, d := range dirs {
		switch d.kind {
		case bindKey:
			b.set(d.context, d.key, d.op)
		case unbindKey:
			b.unset(d.key, d.context)
		case unbindAll:
			b.clear()
		}
	}
}

func newBindings() Bindings {
	return Bindings{contexts: map[string]map[string]string{}}
}
