package config

import "testing"

func TestBindingsLookupPrecedence(t *testing.T) {
	var b Bindings // zero value: nothing bound, must not panic
	if got := b.Lookup("feedlist", "j"); got != "" {
		t.Errorf("zero Bindings Lookup = %q, want \"\"", got)
	}

	b = newBindings()
	b.set(CtxAll, "j", OpDown)
	b.set(CtxFeedList, "j", OpOpen)
	b.set(CtxFeedList, "x", OpQuit)

	if got := b.Lookup("feedlist", "j"); got != OpOpen {
		t.Errorf("context-specific should beat all: %q", got)
	}
	if got := b.Lookup("article", "j"); got != OpDown {
		t.Errorf("all should not leak into unbound contexts: %q", got)
	}
	if got := b.Lookup("articlelist", "x"); got != "" {
		t.Errorf("context binding leaked: %q", got)
	}
	if got := b.Lookup("nosuchcontext", "j"); got != OpDown {
		t.Errorf("unknown context should fall back to all: %q", got)
	}
}

func TestBindingsUnset(t *testing.T) {
	b := newBindings()
	b.set(CtxAll, "q", OpQuit)
	b.set(CtxHelp, "q", OpHardQuit)
	b.set(CtxFeedList, "ENTER", OpOpen)

	// Unbind in one context shadows the "all" fallback only there.
	b.unset("q", CtxHelp)
	if got := b.Lookup("help", "q"); got != "" {
		t.Errorf("help q = %q, want \"\" (explicit unbind)", got)
	}
	if got := b.Lookup("feedlist", "q"); got != OpQuit {
		t.Errorf("feedlist q = %q, want %q", got, OpQuit)
	}

	// A later "all" binding rebinds everywhere, clearing the shadow.
	b.set(CtxAll, "q", OpQuit)
	if got := b.Lookup("help", "q"); got != OpQuit {
		t.Errorf("help q after all-bind = %q, want %q", got, OpQuit)
	}

	// Unbind with context "all" removes everywhere.
	b.unset("q", CtxAll)
	if got := b.Lookup("feedlist", "q"); got != "" {
		t.Errorf("q survived all-unbind: %q", got)
	}
	if got := b.Lookup("help", "q"); got != "" {
		t.Errorf("q survived all-unbind in help: %q", got)
	}

	// Unbind of an unbound key is a no-op.
	b.unset("zzz", CtxAll)
	if got := b.Lookup("feedlist", "ENTER"); got != OpOpen {
		t.Errorf("ENTER lost: %q", got)
	}
}

func TestBindingsClear(t *testing.T) {
	b := newBindings()
	b.set(CtxFeedList, "ENTER", OpOpen)
	b.set(CtxAll, "q", OpQuit)
	b.clear()
	if got := b.Lookup("feedlist", "ENTER"); got != "" {
		t.Errorf("ENTER survived clear: %q", got)
	}
	if got := b.Lookup("help", "q"); got != "" {
		t.Errorf("q survived clear: %q", got)
	}
}
