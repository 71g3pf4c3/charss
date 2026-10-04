package config

import "strings"

// MacroOp is a single operation inside a macro: the operation name plus an
// optional raw argument (e.g. `set browser "cha"` becomes
// Op "set", Arg `browser cha`). Arguments are stored raw; the executor
// interprets them.
type MacroOp struct {
	Op  string
	Arg string
}

// Macro is the parsed body of a `macro <key> <operations...>` directive.
// A macro key redefined later in the config wins.
type Macro struct {
	Ops []MacroOp
}

// parseMacroOps splits a macro body into semicolon-separated operations.
// The separator must be quoted (with "...") to appear inside an argument.
// Empty segments (e.g. a trailing `;`) are skipped.
func parseMacroOps(body string) ([]MacroOp, error) {
	var ops []MacroOp
	for _, seg := range splitOnSemicolon(body) {
		toks, err := tokenize(seg)
		if err != nil {
			return nil, err
		}
		if len(toks) == 0 {
			continue
		}
		var args []string
		for _, t := range toks[1:] {
			args = append(args, t.val)
		}
		ops = append(ops, MacroOp{Op: toks[0].val, Arg: strings.Join(args, " ")})
	}
	return ops, nil
}

// splitOnSemicolon splits s on `;`, keeping double-quoted segments intact
// (and leaving the quotes in place for tokenize to strip).
func splitOnSemicolon(s string) []string {
	var segs []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\\' && inQuote && i+1 < len(s):
			cur.WriteByte(ch)
			cur.WriteByte(s[i+1])
			i++
		case ch == '"':
			inQuote = !inQuote
			cur.WriteByte(ch)
		case ch == ';' && !inQuote:
			segs = append(segs, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	return append(segs, cur.String())
}
