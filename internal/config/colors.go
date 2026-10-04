package config

// Color element names accepted by the `color` directive, mirroring the
// newsboat set. Unknown elements are accepted without a warning — charss
// carries them through and ignores the ones it does not render itself.
const (
	ColorList             = "list"
	ColorListFocus        = "listfocus"
	ColorListNormal       = "listnormal"
	ColorListNormalUnread = "listnormal_unread"
	ColorListFocusUnread  = "listfocus_unread"
	ColorInfo             = "info"
	ColorBackground       = "background"
	ColorArticle          = "article"
	ColorArticleInfo      = "articleinfo"
	ColorArticleTitle     = "articletitle"
	ColorArticleList      = "articlelist"
	ColorHint             = "hint"
	ColorTitle            = "title"
)

// knownAttrs are the attributes accepted after the fg/bg pair of a
// `color` directive. Unknown attributes are kept but reported as warnings.
var knownAttrs = map[string]bool{
	"bold":      true,
	"underline": true,
	"reverse":   true,
	"standout":  true,
	"blink":     true,
	"dim":       true,
	"italics":   true,
	"protected": true,
	"invis":     true,
}

// ColorRule is one parsed `color <element> <fg> <bg> [<attr>...]`
// directive, in file order.
type ColorRule struct {
	Element string
	Fg, Bg  string
	Attrs   []string
}
