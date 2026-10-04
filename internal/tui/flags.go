package tui

import (
	"slices"
	"strings"
)

// Flag handling. A flag is a single character, a-z, A-Z or 0-9 (the task
// spec's alphabet — newsboat itself only allows ASCII letters; the
// deviation is deliberate and recorded in the wave-3 commit). An article
// carries a set of flag chars stored as one sorted string ("aZ"), so
// repeated prompts can accumulate flags like newsboat does.

// validFlag reports whether s is exactly one valid flag character.
func validFlag(s string) bool {
	if len([]rune(s)) != 1 {
		return false
	}
	c := []rune(s)[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// toggleFlag adds ch to flags when absent and removes it when present,
// keeping the result sorted (newsboat keeps flags ordered at all times,
// which filter expressions can rely on).
func toggleFlag(flags string, ch byte) string {
	i := strings.IndexByte(flags, ch)
	if i >= 0 {
		return flags[:i] + flags[i+1:]
	}
	out := flags + string(ch)
	b := []byte(out)
	slices.Sort(b)
	return string(b)
}

// flagOf returns the flag chars of an article in its owning feed's state
// ("" when unflagged; nil-state safe).
func (m Model) flagOf(feedURL, id string) string {
	if st := m.states[feedURL]; st != nil {
		return st.flags[id]
	}
	return ""
}

// setFlagOf stores the flag chars for an article, creating the feed's
// state and map as needed.
func (m *Model) setFlagOf(feedURL, id, flags string) {
	st := m.stateFor(feedURL)
	if flags == "" {
		delete(st.flags, id)
		return
	}
	if st.flags == nil {
		st.flags = make(map[string]string)
	}
	st.flags[id] = flags
}
