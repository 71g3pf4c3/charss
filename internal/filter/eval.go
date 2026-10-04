package filter

import (
	"strings"
	"time"
)

// attribute is a filter expression attribute (left side of a comparison).
type attribute int

const (
	attrTitle attribute = iota
	attrLink
	attrAuthor
	attrGUID
	attrContent
	attrUnread
	attrDate
	attrTags
	attrFlags
	attrFeedTitle
	attrFeedLink
	attrFeedDate
)

var attrByName = map[string]attribute{
	"title":     attrTitle,
	"link":      attrLink,
	"author":    attrAuthor,
	"guid":      attrGUID,
	"content":   attrContent,
	"unread":    attrUnread,
	"date":      attrDate,
	"tags":      attrTags,
	"flags":     attrFlags,
	"feedtitle": attrFeedTitle,
	"feedlink":  attrFeedLink,
	"feeddate":  attrFeedDate,
}

// operator is a comparison operator.
type operator int

const (
	opEq operator = iota
	opNe
	opContains
	opNotContains
)

func opByName(s string) (operator, bool) {
	switch s {
	case "=":
		return opEq, true
	case "!=":
		return opNe, true
	case "#":
		return opContains, true
	case "!#":
		return opNotContains, true
	}
	return 0, false
}

func (n orNode) eval(s Subject) bool  { return n.left.eval(s) || n.right.eval(s) }
func (n andNode) eval(s Subject) bool { return n.left.eval(s) && n.right.eval(s) }
func (n notNode) eval(s Subject) bool { return !n.inner.eval(s) }

func (c cmpNode) eval(s Subject) bool {
	switch c.attr {
	case attrTags:
		return c.evalList(s.Tags)
	case attrDate:
		return c.evalDate(s.Published)
	case attrFeedDate:
		return c.evalDate(s.FeedDate)
	default:
		return c.evalString(attrValue(c.attr, s))
	}
}

// attrValue renders a single-valued attribute as the string compared by
// evalString.
func attrValue(a attribute, s Subject) string {
	switch a {
	case attrTitle:
		return s.Title
	case attrLink:
		return s.Link
	case attrAuthor:
		return s.Author
	case attrGUID:
		return s.GUID
	case attrContent:
		return s.ContentHTML
	case attrUnread:
		if s.Unread {
			return "yes"
		}
		return "no"
	case attrFlags:
		return s.Flags
	case attrFeedTitle:
		return s.FeedTitle
	case attrFeedLink:
		return s.FeedLink
	}
	return ""
}

// evalString applies the operator to a single-valued attribute string.
// Both sides are already (or will be) lowercased.
func (c cmpNode) evalString(str string) bool {
	str = strings.ToLower(str)
	switch c.op {
	case opEq:
		return str == c.value
	case opNe:
		return str != c.value
	case opContains:
		return strings.Contains(str, c.value)
	case opNotContains:
		return !strings.Contains(str, c.value)
	}
	return false
}

// evalList implements tag semantics: "#" and "!#" match if any single
// element contains the value; "=" and "!=" compare against the elements
// joined with single spaces. An empty list satisfies "= "" and never
// satisfies "#".
func (c cmpNode) evalList(items []string) bool {
	switch c.op {
	case opEq, opNe:
		eq := strings.ToLower(strings.Join(items, " ")) == c.value
		if c.op == opEq {
			return eq
		}
		return !eq
	}
	contains := false
	for _, it := range items {
		if strings.Contains(strings.ToLower(it), c.value) {
			contains = true
			break
		}
	}
	if c.op == opNotContains {
		return !contains
	}
	return contains
}

// dateForms returns the string renderings a date attribute is matched
// against: the date-only form and the full RFC3339 timestamp. A zero time
// behaves as an empty string.
func dateForms(t time.Time) []string {
	if t.IsZero() {
		return []string{""}
	}
	return []string{t.Format(dateOnlyLayout), t.Format(time.RFC3339)}
}

// evalDate applies the operator against both date renderings: the
// comparison holds if either form satisfies it.
func (c cmpNode) evalDate(t time.Time) bool {
	eq, contains := false, false
	for _, form := range dateForms(t) {
		form = strings.ToLower(form)
		if form == c.value {
			eq = true
		}
		if strings.Contains(form, c.value) {
			contains = true
		}
	}
	switch c.op {
	case opEq:
		return eq
	case opNe:
		return !eq
	case opContains:
		return contains
	case opNotContains:
		return !contains
	}
	return false
}
