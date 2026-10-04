package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// parseContent writes content to a temp config file, parses it, and
// returns the parse result plus its binding directives applied onto an
// empty Bindings (no defaults — that is Load's job).
func parseContent(t *testing.T, content string) (*parseResult, Bindings) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := parseConfig(path)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	b := newBindings()
	b.apply(res.bindings)
	return res, b
}

// parseContentErr parses content that is expected to fail and returns the
// error message.
func parseContentErr(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseConfig(path)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	return err.Error()
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestParseOptions(t *testing.T) {
	res, _ := parseContent(t, `
# full-line comment

browser chawan-browser
max-items 100
chafa "chafa --format sixel"

# escaped comment marker is a real option line
\#weird value-with-hash
`)
	if got := res.options["browser"]; got != "chawan-browser" {
		t.Errorf(`options["browser"] = %q`, got)
	}
	if got := res.options["max-items"]; got != "100" {
		t.Errorf(`options["max-items"] = %q`, got)
	}
	// Quoted value: quotes stripped, spaces kept.
	if got := res.options["chafa"]; got != "chafa --format sixel" {
		t.Errorf(`options["chafa"] = %q`, got)
	}
	if got := res.options["#weird"]; got != "value-with-hash" {
		t.Errorf(`options["#weird"] = %q`, got)
	}
	// Unknown options are kept and warned about, not fatal.
	if got := res.options["max-items"]; got != "100" {
		t.Errorf("unknown option not kept: %q", got)
	}
	if !hasWarning(res.warnings, `unknown option "max-items"`) {
		t.Errorf("missing unknown-option warning, got %v", res.warnings)
	}
	if hasWarning(res.warnings, `"browser"`) {
		t.Errorf("known option warned about: %v", res.warnings)
	}
}

func TestParseOptionLastWins(t *testing.T) {
	res, _ := parseContent(t, `
browser first
browser second
`)
	if got := res.options["browser"]; got != "second" {
		t.Errorf(`options["browser"] = %q, want "second"`, got)
	}
}

func TestParseBindKey(t *testing.T) {
	res, b := parseContent(t, `
bind-key j down
bind-key x quit feedlist
bind-key j up
bind-key x quit feedlist
bind-key z not-an-op
bind-key y down bogus-context
`)
	// No context = "all".
	if got := b.Lookup("feedlist", "j"); got != OpUp {
		t.Errorf(`Lookup(feedlist, j) = %q, want %q (last wins)`, got, OpUp)
	}
	if got := b.Lookup("articlelist", "j"); got != OpUp {
		t.Errorf(`Lookup(articlelist, j) = %q, want %q`, got, OpUp)
	}
	// Same key+context: last wins.
	if got := b.Lookup("feedlist", "x"); got != OpQuit {
		t.Errorf(`Lookup(feedlist, x) = %q, want %q`, got, OpQuit)
	}
	// Context-specific does not leak.
	if got := b.Lookup("articlelist", "x"); got != "" {
		t.Errorf(`Lookup(articlelist, x) = %q, want ""`, got)
	}
	// Unknown op/context: warnings, still stored.
	if !hasWarning(res.warnings, `unknown operation "not-an-op"`) {
		t.Errorf("missing unknown-operation warning, got %v", res.warnings)
	}
	if !hasWarning(res.warnings, `unknown context "bogus-context"`) {
		t.Errorf("missing unknown-context warning, got %v", res.warnings)
	}
	if got := b.Lookup("feedlist", "z"); got != "not-an-op" {
		t.Errorf(`Lookup(feedlist, z) = %q, want "not-an-op"`, got)
	}
}

func TestParseBindKeyContextBeatsAll(t *testing.T) {
	// Resolution rule: a context-specific binding beats "all" regardless
	// of which appears later in the file.
	_, b := parseContent(t, `
bind-key j down feedlist
bind-key j up
`)
	if got := b.Lookup("feedlist", "j"); got != OpDown {
		t.Errorf(`Lookup(feedlist, j) = %q, want %q`, got, OpDown)
	}
	if got := b.Lookup("article", "j"); got != OpUp {
		t.Errorf(`Lookup(article, j) = %q, want %q`, got, OpUp)
	}
}

func TestParseBindKeyMalformed(t *testing.T) {
	msg := parseContentErr(t, "browser chawan\nbind-key j\n")
	if !strings.Contains(msg, ":2:") {
		t.Errorf("error lacks line number: %s", msg)
	}
	if !strings.Contains(msg, "bind-key") {
		t.Errorf("error lacks directive name: %s", msg)
	}
}

func TestParseUnbindKey(t *testing.T) {
	// Unbind against the defaults, like Load does.
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("unbind-key r all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := parseConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	b := defaultBindings()
	b.apply(res.bindings)
	if got := b.Lookup("feedlist", "r"); got != "" {
		t.Errorf(`r still bound in feedlist: %q`, got)
	}
	if got := b.Lookup("feedlist", "q"); got != OpQuit {
		t.Errorf(`q unbound too: %q`, got)
	}

	// `unbind-key q help` only affects the help context.
	res, _ = parseContent(t, "unbind-key q help\n")
	b = defaultBindings()
	b.apply(res.bindings)
	if got := b.Lookup("help", "q"); got != "" {
		t.Errorf(`q still bound in help: %q`, got)
	}
	if got := b.Lookup("feedlist", "q"); got != OpQuit {
		t.Errorf(`q unbound in feedlist: %q`, got)
	}

	// One-argument form removes the key everywhere.
	res, _ = parseContent(t, "unbind-key q\n")
	b = defaultBindings()
	b.apply(res.bindings)
	for _, ctx := range []string{CtxAll, CtxFeedList, CtxHelp} {
		if got := b.Lookup(ctx, "q"); got != "" {
			t.Errorf(`q still bound in %s: %q`, ctx, got)
		}
	}

	// Bare unbind-key clears everything.
	res, _ = parseContent(t, "unbind-key\n")
	b = defaultBindings()
	b.apply(res.bindings)
	if got := b.Lookup("feedlist", "ENTER"); got != "" {
		t.Errorf(`ENTER still bound: %q`, got)
	}

	// A later all-context bind rebinds a key unbound earlier per context.
	res, _ = parseContent(t, "unbind-key q help\nbind-key q hard-quit\n")
	b = defaultBindings()
	b.apply(res.bindings)
	if got := b.Lookup("help", "q"); got != OpHardQuit {
		t.Errorf(`Lookup(help, q) = %q, want %q (later all-bind wins)`, got, OpHardQuit)
	}
	if got := b.Lookup("feedlist", "q"); got != OpHardQuit {
		t.Errorf(`Lookup(feedlist, q) = %q, want %q`, got, OpHardQuit)
	}
}

func TestParseMacro(t *testing.T) {
	res, _ := parseContent(t, `
macro , open ; reload ; quit
macro b set browser "firefox --new-tab"; open
macro d quit
macro d reload
macro s set query "a;b"; quit
`)
	m := res.macros[","]
	if len(m.Ops) != 3 {
		t.Fatalf(`macros[","] has %d ops, want 3`, len(m.Ops))
	}
	want := []MacroOp{{Op: "open"}, {Op: "reload"}, {Op: "quit"}}
	for i, w := range want {
		if m.Ops[i] != w {
			t.Errorf("op %d = %+v, want %+v", i, m.Ops[i], w)
		}
	}

	// Arg carries the raw tokens after the op name; quotes stripped,
	// spaces preserved.
	m = res.macros["b"]
	if len(m.Ops) != 2 {
		t.Fatalf(`macros["b"] has %d ops, want 2: %+v`, len(m.Ops), m.Ops)
	}
	if m.Ops[0].Op != "set" || m.Ops[0].Arg != "browser firefox --new-tab" {
		t.Errorf(`macros["b"] op 0 = %+v`, m.Ops[0])
	}
	if m.Ops[1].Op != "open" || m.Ops[1].Arg != "" {
		t.Errorf(`macros["b"] op 1 = %+v`, m.Ops[1])
	}

	// Redefinition wins.
	m = res.macros["d"]
	if len(m.Ops) != 1 || m.Ops[0].Op != "reload" {
		t.Errorf(`macros["d"] = %+v, want single reload`, m.Ops)
	}

	// A semicolon inside quotes does not split the operation.
	m = res.macros["s"]
	if len(m.Ops) != 2 {
		t.Fatalf(`macros["s"] has %d ops, want 2: %+v`, len(m.Ops), m.Ops)
	}
	if m.Ops[0].Arg != `query a;b` {
		t.Errorf(`macros["s"] first arg = %q`, m.Ops[0].Arg)
	}
}

func TestParseMacroGluedSemicolons(t *testing.T) {
	res, _ := parseContent(t, `macro m open;reload;quit`)
	m := res.macros["m"]
	if len(m.Ops) != 3 {
		t.Fatalf(`macros["m"] has %d ops, want 3`, len(m.Ops))
	}
	if m.Ops[1].Op != "reload" {
		t.Errorf("op 1 = %q", m.Ops[1].Op)
	}
}

func TestParseMacroErrorsAndWarnings(t *testing.T) {
	if msg := parseContentErr(t, "macro ,\n"); !strings.Contains(msg, ":1:") {
		t.Errorf("macro without ops should be a line-numbered error: %s", msg)
	}
	if msg := parseContentErr(t, "macro\n"); !strings.Contains(msg, ":1:") {
		t.Errorf("bare macro should be a line-numbered error: %s", msg)
	}
	res, _ := parseContent(t, "macro m frobnicate\n")
	if !hasWarning(res.warnings, `unknown operation "frobnicate"`) {
		t.Errorf("missing unknown macro op warning: %v", res.warnings)
	}
}

func TestParseColor(t *testing.T) {
	res, _ := parseContent(t, `
color list default default
color listfocus black yellow bold
color article cyan blue bold underline
color weird-element red green sparkle
`)
	if len(res.colors) != 4 {
		t.Fatalf("got %d color rules, want 4", len(res.colors))
	}
	if !reflect.DeepEqual(res.colors[0], ColorRule{Element: "list", Fg: "default", Bg: "default"}) {
		t.Errorf("rule 0 = %+v", res.colors[0])
	}
	if !reflect.DeepEqual(res.colors[1], ColorRule{Element: "listfocus", Fg: "black", Bg: "yellow", Attrs: []string{"bold"}}) {
		t.Errorf("rule 1 = %+v", res.colors[1])
	}
	if got := res.colors[2].Attrs; len(got) != 2 || got[0] != "bold" || got[1] != "underline" {
		t.Errorf("rule 2 attrs = %v", got)
	}
	// Unknown elements accepted silently, unknown attrs warned but kept.
	if hasWarning(res.warnings, "weird-element") {
		t.Errorf("unknown element should not warn: %v", res.warnings)
	}
	if !hasWarning(res.warnings, `unknown attribute "sparkle"`) {
		t.Errorf("missing unknown-attribute warning: %v", res.warnings)
	}
	if got := res.colors[3].Attrs; len(got) != 1 || got[0] != "sparkle" {
		t.Errorf("unknown attr not kept: %v", got)
	}
}

func TestParseColorMalformed(t *testing.T) {
	msg := parseContentErr(t, "color list default\n")
	if !strings.Contains(msg, ":1:") || !strings.Contains(msg, "color") {
		t.Errorf("malformed color should be a line-numbered error: %s", msg)
	}
}

func TestParseInclude(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Nested includes resolve relative to the INCLUDING file's dir.
	write(filepath.Join(dir, "a.conf"), "browser from-a\ninclude sub/b.conf\n")
	write(filepath.Join(sub, "b.conf"), "browser from-b\ninclude c.conf\n")
	write(filepath.Join(sub, "c.conf"), "chafa from-c\n")

	res, err := parseConfig(filepath.Join(dir, "a.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.options["browser"]; got != "from-b" {
		t.Errorf(`options["browser"] = %q, want "from-b" (later include wins)`, got)
	}
	if got := res.options["chafa"]; got != "from-c" {
		t.Errorf(`options["chafa"] = %q, want "from-c"`, got)
	}

	// Quoted include path.
	write(filepath.Join(dir, "quoted.conf"), `include "space name.conf"`)
	write(filepath.Join(dir, "space name.conf"), "browser quoted\n")
	res, err = parseConfig(filepath.Join(dir, "quoted.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.options["browser"]; got != "quoted" {
		t.Errorf(`options["browser"] = %q, want "quoted"`, got)
	}
}

func TestParseIncludeTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "extra.conf"), []byte("browser tilde\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := parseContent2(t, "include ~/extra.conf\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.options["browser"]; got != "tilde" {
		t.Errorf(`options["browser"] = %q, want "tilde"`, got)
	}
}

// parseContent2 is parseContent that also returns the error, for includes
// exercising absolute-path resolution.
func parseContent2(t *testing.T, content string) (*parseResult, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return parseConfig(path)
}

func TestParseIncludeDiamondIsNotACycle(t *testing.T) {
	// The same file included twice in sibling branches is fine.
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.conf", "include b.conf\ninclude c.conf\n")
	write("b.conf", "include d.conf\n")
	write("c.conf", "include d.conf\n")
	write("d.conf", "browser d\n")

	res, err := parseConfig(filepath.Join(dir, "a.conf"))
	if err != nil {
		t.Fatalf("diamond include reported as error: %v", err)
	}
	if got := res.options["browser"]; got != "d" {
		t.Errorf(`options["browser"] = %q`, got)
	}
}

func TestParseIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.conf"), []byte("include b.conf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.conf"), []byte("include a.conf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseConfig(filepath.Join(dir, "a.conf"))
	if err == nil || !strings.Contains(err.Error(), "include cycle") {
		t.Fatalf("want include cycle error, got %v", err)
	}

	// Self-include is a cycle too.
	self := filepath.Join(dir, "self.conf")
	if err := os.WriteFile(self, []byte("include self.conf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = parseConfig(self)
	if err == nil || !strings.Contains(err.Error(), "include cycle") {
		t.Fatalf("want include cycle error, got %v", err)
	}
}

func TestParseIncludeMissing(t *testing.T) {
	msg := parseContentErr(t, "include does-not-exist.conf\n")
	if !strings.Contains(msg, "does-not-exist.conf") {
		t.Errorf("missing include error lacks the path: %s", msg)
	}
}

func TestParseUnterminatedQuote(t *testing.T) {
	msg := parseContentErr(t, `browser "chawan`)
	if !strings.Contains(msg, ":1:") {
		t.Errorf("unterminated quote should be a line-numbered error: %s", msg)
	}
}

func TestParseLineNumbers(t *testing.T) {
	msg := parseContentErr(t, "browser chawan\n\n# comment\n\ncolor list default\n")
	if !strings.Contains(msg, ":5:") {
		t.Errorf("error should point at line 5: %s", msg)
	}
}

func TestTokenize(t *testing.T) {
	toks, err := tokenize(`  browser  "firefox --new-tab %u"  extra `)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tok := range toks {
		got = append(got, tok.val)
	}
	want := []string{"browser", "firefox --new-tab %u", "extra"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}

	// Escape handling inside quotes.
	toks, err = tokenize(`set browser "say \"hi\" \\ done"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := `say "hi" \ done`; toks[2].val != want {
		t.Errorf("escaped token = %q, want %q", toks[2].val, want)
	}

	// Empty quoted token is a token.
	toks, err = tokenize(`browser ""`)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 2 || toks[1].val != "" {
		t.Errorf("empty quoted token lost: %+v", toks)
	}

	if _, err := tokenize(`"unterminated`); err == nil {
		t.Error("unterminated quote should error")
	}
}
