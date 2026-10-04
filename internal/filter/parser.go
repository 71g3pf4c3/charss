package filter

import (
	"fmt"
	"strings"
)

// tokenKind identifies a lexical token.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokLParen
	tokRParen
	tokOp
)

// token is one lexical token. pos is the 1-based byte offset of the token's
// first byte in the source.
type token struct {
	kind tokenKind
	text string
	pos  int
}

// displayToken renders a token for use in syntax errors. EOF becomes
// "end of input" so the message still names an offending token.
func displayToken(t token) string {
	if t.kind == tokEOF {
		return "end of input"
	}
	return t.text
}

// lexer produces tokens one byte at a time. It never backs up: the grammar
// is LL(1) with single-token lookahead.
type lexer struct {
	src string
	pos int // 0-based byte offset of the next unread byte
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// next scans and returns the next token, skipping leading whitespace.
func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) && isSpace(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, pos: l.pos + 1}, nil
	}
	start := l.pos
	c := l.src[l.pos]
	switch {
	case c == '(':
		l.pos++
		return token{kind: tokLParen, text: "(", pos: start + 1}, nil
	case c == ')':
		l.pos++
		return token{kind: tokRParen, text: ")", pos: start + 1}, nil
	case c == '=' || c == '#':
		l.pos++
		return token{kind: tokOp, text: string(c), pos: start + 1}, nil
	case c == '!':
		if l.pos+1 < len(l.src) && (l.src[l.pos+1] == '=' || l.src[l.pos+1] == '#') {
			op := l.src[l.pos : l.pos+2]
			l.pos += 2
			return token{kind: tokOp, text: op, pos: start + 1}, nil
		}
		return token{}, &SyntaxError{Msg: "unexpected character", Token: "!", Pos: start + 1}
	case c == '"':
		// No escape processing: the value ends at the first '"'.
		l.pos++
		vStart := l.pos
		for l.pos < len(l.src) && l.src[l.pos] != '"' {
			l.pos++
		}
		if l.pos >= len(l.src) {
			return token{}, &SyntaxError{Msg: "unterminated string", Token: l.src[vStart:], Pos: start + 1}
		}
		v := l.src[vStart:l.pos]
		l.pos++
		return token{kind: tokString, text: v, pos: start + 1}, nil
	case isIdentStart(c):
		for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
			l.pos++
		}
		return token{kind: tokIdent, text: l.src[start:l.pos], pos: start + 1}, nil
	default:
		return token{}, &SyntaxError{Msg: "unexpected character", Token: string(c), Pos: start + 1}
	}
}

// node is a compiled expression tree.
type node interface {
	eval(s Subject) bool
}

type orNode struct{ left, right node }
type andNode struct{ left, right node }
type notNode struct{ inner node }
type cmpNode struct {
	attr  attribute
	op    operator
	value string // lowercased at compile time
}

// parser is a recursive-descent parser over the grammar in the package doc.
type parser struct {
	lex lexer
	cur token
}

func (p *parser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = t
	return nil
}

func (p *parser) keyword(kw string) bool {
	return p.cur.kind == tokIdent && p.cur.text == kw
}

// parse parses a complete expression; trailing input is an error.
func parse(src string) (node, error) {
	p := &parser{lex: lexer{src: src}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.kind == tokEOF {
		return nil, &SyntaxError{Msg: "empty expression", Token: "", Pos: 1}
	}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != tokEOF {
		return nil, &SyntaxError{Msg: "unexpected token after expression", Token: displayToken(p.cur), Pos: p.cur.pos}
	}
	return n, nil
}

// parseExpr: andexpr ("or" andexpr)*
func (p *parser) parseExpr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.keyword("or") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left: left, right: right}
	}
	return left, nil
}

// parseAnd: notexpr ("and" notexpr)*
func (p *parser) parseAnd() (node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.keyword("and") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = andNode{left: left, right: right}
	}
	return left, nil
}

// parseNot: "not" notexpr | primary. Right-recursive so that "not not x"
// nests.
func (p *parser) parseNot() (node, error) {
	if p.keyword("not") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return notNode{inner: inner}, nil
	}
	return p.parsePrimary()
}

// parsePrimary: "(" expr ")" | comparison.
func (p *parser) parsePrimary() (node, error) {
	if p.cur.kind == tokLParen {
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.cur.kind != tokRParen {
			return nil, &SyntaxError{Msg: "expected ')'", Token: displayToken(p.cur), Pos: p.cur.pos}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return inner, nil
	}
	return p.parseComparison()
}

// parseComparison: attr op value.
func (p *parser) parseComparison() (node, error) {
	if p.cur.kind != tokIdent {
		return nil, &SyntaxError{Msg: "expected attribute", Token: displayToken(p.cur), Pos: p.cur.pos}
	}
	name, pos := p.cur.text, p.cur.pos
	attr, ok := attrByName[name]
	if !ok {
		return nil, &SyntaxError{Msg: fmt.Sprintf("unknown attribute %q", name), Token: name, Pos: pos}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.kind != tokOp {
		return nil, &SyntaxError{Msg: "expected operator (=, !=, #, !#)", Token: displayToken(p.cur), Pos: p.cur.pos}
	}
	op, ok := opByName(p.cur.text)
	if !ok {
		return nil, &SyntaxError{Msg: fmt.Sprintf("unknown operator %q", p.cur.text), Token: p.cur.text, Pos: p.cur.pos}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.cur.kind != tokString {
		return nil, &SyntaxError{Msg: "expected quoted string value", Token: displayToken(p.cur), Pos: p.cur.pos}
	}
	// Case-insensitive matching: fold the value once, at compile time.
	c := cmpNode{attr: attr, op: op, value: strings.ToLower(p.cur.text)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return c, nil
}
