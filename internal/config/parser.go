package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// directiveKind identifies which binding-mutating directive a parsed line
// produced; directives are replayed in file order by Bindings.apply.
type directiveKind int

const (
	bindKey   directiveKind = iota // bind-key <key> <op> [<context>]
	unbindKey                      // unbind-key <key> [<context>]
	unbindAll                      // unbind-key (bare)
)

type bindingDirective struct {
	kind             directiveKind
	key, op, context string // op: bindKey only; context: "" for unbindAll
}

// parseResult is everything extracted from a config file (and its includes).
type parseResult struct {
	options  map[string]string  // plain `name value` options; last assignment wins
	bindings []bindingDirective // in file order (bind-key/unbind-key interleaved)
	macros   map[string]Macro   // key -> macro; redefinition wins
	colors   []ColorRule        // in file order
	warnings []string           // non-fatal issues, with file:line prefixes
}

func (r *parseResult) warn(format string, args ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

// parseConfig parses the newsboat-style config at path, following include
// directives recursively.
func parseConfig(path string) (*parseResult, error) {
	res := &parseResult{
		options: map[string]string{},
		macros:  map[string]Macro{},
	}
	if err := parseFile(path, res, nil); err != nil {
		return nil, err
	}
	return res, nil
}

// parseFile parses one file. stack holds the canonical paths of the files
// currently being parsed (the include chain) for cycle detection; a file
// may be included twice in sibling branches. A missing file is an error —
// including a missing include.
func parseFile(path string, res *parseResult, stack []string) error {
	canon, err := filepath.Abs(path)
	if err != nil {
		canon = filepath.Clean(path)
	}
	for _, p := range stack {
		if p == canon {
			return fmt.Errorf("include cycle: %s", path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	stack = append(stack, canon)
	for i, line := range strings.Split(string(data), "\n") {
		if err := parseLine(path, i+1, line, res, stack); err != nil {
			return err
		}
	}
	return nil
}

// parseLine handles one line of a config file. path/lineno are used for
// error and warning attribution.
func parseLine(path string, lineno int, line string, res *parseResult, stack []string) error {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	if strings.HasPrefix(line, `\#`) {
		// newsboat escape for a line that must start with a literal #.
		line = strings.TrimSpace(line[1:])
	}
	toks, err := tokenize(line)
	if err != nil {
		return fmt.Errorf("%s:%d: %v", path, lineno, err)
	}
	if len(toks) == 0 {
		return nil
	}

	switch toks[0].val {
	case "include":
		if len(toks) != 2 {
			return fmt.Errorf("%s:%d: include: expected `include <path>`", path, lineno)
		}
		return parseFile(resolveInclude(path, toks[1].val), res, stack)

	case "bind-key":
		if len(toks) < 3 || len(toks) > 4 {
			return fmt.Errorf("%s:%d: bind-key: expected `bind-key <key> <operation> [<context>]`", path, lineno)
		}
		key, op := toks[1].val, toks[2].val
		ctx := CtxAll // newsboat: no context given binds for all contexts
		if len(toks) == 4 {
			ctx = toks[3].val
		}
		if !knownContexts[ctx] {
			res.warn("%s:%d: bind-key: unknown context %q", path, lineno, ctx)
		}
		if canon, ok := Operations[op]; !ok {
			res.warn("%s:%d: bind-key: unknown operation %q", path, lineno, op)
		} else if canon != op {
			op = canon // alias (e.g. newsboat's macro-prefix) -> canonical name
		}
		res.bindings = append(res.bindings, bindingDirective{kind: bindKey, key: key, op: op, context: ctx})

	case "unbind-key":
		switch {
		case len(toks) == 1:
			res.bindings = append(res.bindings, bindingDirective{kind: unbindAll})
		case len(toks) == 2:
			res.bindings = append(res.bindings, bindingDirective{kind: unbindKey, key: toks[1].val, context: CtxAll})
		case len(toks) == 3:
			ctx := toks[2].val
			if !knownContexts[ctx] {
				res.warn("%s:%d: unbind-key: unknown context %q", path, lineno, ctx)
			}
			res.bindings = append(res.bindings, bindingDirective{kind: unbindKey, key: toks[1].val, context: ctx})
		default:
			return fmt.Errorf("%s:%d: unbind-key: expected `unbind-key [<key> [<context>]]`", path, lineno)
		}

	case "macro":
		if len(toks) < 2 {
			return fmt.Errorf("%s:%d: macro: expected `macro <key> <operations...>`", path, lineno)
		}
		key := toks[1].val
		ops, err := parseMacroOps(line[toks[1].end:])
		if err != nil {
			return fmt.Errorf("%s:%d: macro %q: %v", path, lineno, key, err)
		}
		if len(ops) == 0 {
			return fmt.Errorf("%s:%d: macro %q: no operations", path, lineno, key)
		}
		for _, op := range ops {
			if canon, ok := Operations[op.Op]; !ok {
				res.warn("%s:%d: macro %q: unknown operation %q", path, lineno, key, op.Op)
			} else if canon != op.Op {
				op.Op = canon // alias -> canonical name
			}
		}
		res.macros[key] = Macro{Ops: ops} // redefinition wins

	case "color":
		if len(toks) < 4 {
			return fmt.Errorf("%s:%d: color: expected `color <element> <fg> <bg> [<attr>...]`", path, lineno)
		}
		rule := ColorRule{Element: toks[1].val, Fg: toks[2].val, Bg: toks[3].val}
		for _, t := range toks[4:] {
			rule.Attrs = append(rule.Attrs, t.val)
			if !knownAttrs[t.val] {
				res.warn("%s:%d: color: unknown attribute %q", path, lineno, t.val)
			}
		}
		res.colors = append(res.colors, rule)

	default:
		// Plain `name value` option. The value is the rest of the line as
		// one token sequence (quotes already stripped); last assignment
		// wins, like newsboat re-assignment.
		name := toks[0].val
		parts := make([]string, 0, len(toks)-1)
		for _, t := range toks[1:] {
			parts = append(parts, t.val)
		}
		res.options[name] = strings.Join(parts, " ")
		if !knownOptions[name] {
			res.warn("%s:%d: unknown option %q", path, lineno, name)
		}
	}
	return nil
}

// resolveInclude expands ~ and resolves target relative to the directory
// of the including file (not the working directory).
func resolveInclude(includingFile, target string) string {
	t := expandTilde(target)
	if filepath.IsAbs(t) {
		return t
	}
	return filepath.Join(filepath.Dir(includingFile), t)
}

// expandTilde expands a leading ~ or ~/ using os.UserHomeDir.
func expandTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// token is one whitespace-separated word of a config line. end is the byte
// offset just past the token in the line it was tokenized from, so callers
// can take the raw remainder.
type token struct {
	val string
	end int
}

// tokenize splits a config line into whitespace-separated tokens with
// newsboat-style double quoting: a token wrapped in "..." may contain
// spaces and semicolons; inside quotes, \" and \\ are escapes. Token values
// have the quotes stripped.
func tokenize(line string) ([]token, error) {
	var toks []token
	i, n := 0, len(line)
	for i < n {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '"':
			i++
			var sb strings.Builder
			closed := false
			for i < n {
				ch := line[i]
				if ch == '\\' && i+1 < n && (line[i+1] == '"' || line[i+1] == '\\') {
					sb.WriteByte(line[i+1])
					i += 2
					continue
				}
				if ch == '"' {
					i++
					closed = true
					break
				}
				sb.WriteByte(ch)
				i++
			}
			if !closed {
				return nil, errors.New("unterminated quoted string")
			}
			toks = append(toks, token{val: sb.String(), end: i})
		default:
			start := i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			toks = append(toks, token{val: line[start:i], end: i})
		}
	}
	return toks, nil
}
