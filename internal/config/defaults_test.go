package config

import "testing"

func TestDefaultBindings(t *testing.T) {
	b := defaultBindings()
	cases := []struct {
		context, key, want string
	}{
		// Spec anchors.
		{CtxFeedList, "ENTER", OpOpen},
		{CtxFeedList, "q", OpQuit}, // via "all"
		{CtxHelp, "q", OpQuit},     // "all" applies everywhere
		{CtxFeedList, "r", OpReload},
		{CtxFeedList, "R", OpReloadAll},
		{CtxFeedList, "A", OpMarkFeedRead},
		{CtxFeedList, "j", OpDown},
		{CtxFeedList, "k", OpUp},
		{CtxArticleList, "ENTER", OpOpen},
		{CtxArticleList, "n", OpNextUnread},
		{CtxArticleList, "p", OpPrevUnread},
		{CtxArticleList, "o", OpOpenInBrowser},
		{CtxArticleList, "s", OpSave},
		{CtxArticleList, "N", OpToggleArticleRead},
		{CtxArticle, "u", OpShowURLs},
		{CtxArticle, "j", OpDown},
		{CtxDialog, "?", OpHelp}, // dialog has no own entries, "all" wins
		{CtxPodcast, "j", OpDown},
	}
	for _, c := range cases {
		if got := b.Lookup(c.context, c.key); got != c.want {
			t.Errorf("Lookup(%s, %q) = %q, want %q", c.context, c.key, got, c.want)
		}
	}

	// Nothing bound for a key nobody defines.
	if got := b.Lookup(CtxFeedList, "zz"); got != "" {
		t.Errorf("unexpected binding: %q", got)
	}
}

// Every default binding must reference a known operation, so a typo fails
// here instead of producing a dead binding.
func TestDefaultBindingsOpsAreKnown(t *testing.T) {
	for ctx, keys := range defaultKeys {
		for key, op := range keys {
			if _, ok := Operations[op]; !ok {
				t.Errorf("default binding %s/%s references unknown operation %q", ctx, key, op)
			}
		}
	}
}

// defaultBindings must return an independent copy every call.
func TestDefaultBindingsIsACopy(t *testing.T) {
	b := defaultBindings()
	b.unset("q", CtxAll)
	b.unset("ENTER", CtxFeedList)
	b.set(CtxFeedList, "x", OpOpen)

	fresh := defaultBindings()
	if got := fresh.Lookup(CtxFeedList, "q"); got != OpQuit {
		t.Errorf("default map mutated: q = %q", got)
	}
	if got := fresh.Lookup(CtxFeedList, "ENTER"); got != OpOpen {
		t.Errorf("default map mutated: ENTER = %q", got)
	}
	if got := fresh.Lookup(CtxFeedList, "x"); got != "" {
		t.Errorf("leaked binding: x = %q", got)
	}
}
