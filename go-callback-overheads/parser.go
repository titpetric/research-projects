package callbacks

import (
	"fmt"
	"strconv"
)

// Parser turns a program into a list of statements. The grammar has no
// operators: a statement is a call, and every value is a literal, a
// name, or the result of another call.
//
//	program := { stmt }
//	stmt    := "return" [ expr ] ";"
//	         | [ name { "," name } ( ":=" | "=" ) ] expr ";"
//	expr    := path "(" [ args ] ")" { "." ident "(" [ args ] ")" }
//	path    := ident { "." ident }
//	args    := arg { "," arg }
//	arg     := string | number | name | expr
//
// A path is resolved by the compiler, not here: http.NewRequest is one
// bound name while req.Cookies is a method on the value held by req,
// and the parser cannot tell them apart without the bindings.
//
// Strings are single- or double-quoted. Numbers map to int64 when they
// have no decimal point and float64 when they do; no other numeric
// types exist.
type Parser struct {
	src string
	pos int
}

type argKind int

const (
	argString argKind = iota
	argInt
	argFloat
	argVar
	argCall
)

// arg is one parsed argument: a literal, a name, or a nested call.
type arg struct {
	kind argKind
	str  string // argString value or argVar name
	i    int64
	f    float64
	sub  *callExpr // argCall
}

// link is one ".Method(args)" step chained onto a call.
type link struct {
	name string
	args []arg
}

// callExpr is a call and the chain of method calls applied to its
// result. path holds the dotted name written in the source, split on
// the dots.
type callExpr struct {
	path  []string
	args  []arg
	chain []link
}

// stmt is one statement of a program.
type stmt struct {
	lhs    []string // names bound to the call's results, empty to discard
	define bool     // ":=" rather than "="
	ret    bool     // a return statement
	call   *callExpr
}

// program is a parsed source unit.
type program struct {
	stmts []stmt
}

// flatCall reports the single call of a one-statement program whose
// arguments are all leaves, and whether the program has that shape.
// This is the form the JIT shape table matches, and the form every
// statement had before programs grew past one line.
func (p *program) flatCall() (*callExpr, bool) {
	if len(p.stmts) != 1 {
		return nil, false
	}
	s := p.stmts[0]
	if !s.ret || s.call == nil || len(s.call.chain) != 0 || len(s.call.path) != 1 {
		return nil, false
	}
	for _, a := range s.call.args {
		if a.kind == argCall {
			return nil, false
		}
	}
	return s.call, true
}

// Parse parses a program.
func (p *Parser) Parse(src string) (*program, error) {
	p.src, p.pos = src, 0

	prog := &program{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			break
		}
		s, err := p.stmt()
		if err != nil {
			return nil, err
		}
		prog.stmts = append(prog.stmts, s)
	}
	if len(prog.stmts) == 0 {
		return nil, fmt.Errorf("parse: empty program")
	}
	return prog, nil
}

func (p *Parser) stmt() (stmt, error) {
	if p.keyword("return") {
		s := stmt{ret: true}
		p.skipSpace()
		if p.consume(';') {
			return s, nil
		}
		call, err := p.expr()
		if err != nil {
			return s, err
		}
		s.call = call
		if !p.consume(';') {
			return s, fmt.Errorf("parse: expected ';' at offset %d", p.pos)
		}
		return s, nil
	}

	// A statement is a call, optionally preceded by the names its
	// results bind to. The names are only known to be names once the
	// assignment operator is seen, so the position is saved and the
	// scan restarts as a bare call when it is not.
	save := p.pos
	lhs, define, ok := p.assignList()
	if !ok {
		p.pos = save
		lhs, define = nil, false
	}

	call, err := p.expr()
	if err != nil {
		return stmt{}, err
	}
	if !p.consume(';') {
		return stmt{}, fmt.Errorf("parse: expected ';' at offset %d", p.pos)
	}
	return stmt{lhs: lhs, define: define, call: call}, nil
}

// assignList scans "a, b :=" or "a =" and reports whether one was
// there. It never returns an error: anything that does not match is the
// caller's cue to rewind and read a bare call.
func (p *Parser) assignList() ([]string, bool, bool) {
	var lhs []string
	for {
		name := p.ident()
		if name == "" {
			return nil, false, false
		}
		lhs = append(lhs, name)
		p.skipSpace()
		if p.consume(',') {
			continue
		}
		break
	}
	if p.consumeStr(":=") {
		return lhs, true, true
	}
	// "==" does not exist in the grammar, but a lone "=" must not
	// swallow one if it ever does.
	if p.pos < len(p.src) && p.src[p.pos] == '=' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '=') {
		p.pos++
		return lhs, false, true
	}
	return nil, false, false
}

func (p *Parser) expr() (*callExpr, error) {
	path, err := p.path()
	if err != nil {
		return nil, err
	}
	if !p.consume('(') {
		return nil, fmt.Errorf("parse: expected '(' at offset %d", p.pos)
	}
	args, err := p.args()
	if err != nil {
		return nil, err
	}
	call := &callExpr{path: path, args: args}

	for {
		save := p.pos
		p.skipSpace()
		if !p.consume('.') {
			p.pos = save
			return call, nil
		}
		name := p.ident()
		if name == "" {
			return nil, fmt.Errorf("parse: expected method name at offset %d", p.pos)
		}
		if !p.consume('(') {
			return nil, fmt.Errorf("parse: expected '(' at offset %d", p.pos)
		}
		largs, err := p.args()
		if err != nil {
			return nil, err
		}
		call.chain = append(call.chain, link{name: name, args: largs})
	}
}

func (p *Parser) path() ([]string, error) {
	name := p.ident()
	if name == "" {
		return nil, fmt.Errorf("parse: expected a name at offset %d", p.pos)
	}
	path := []string{name}
	for {
		save := p.pos
		if !p.consume('.') {
			return path, nil
		}
		next := p.ident()
		if next == "" {
			p.pos = save
			return path, nil
		}
		path = append(path, next)
	}
}

// args reads the argument list up to and including the closing paren.
func (p *Parser) args() ([]arg, error) {
	var out []arg
	for {
		p.skipSpace()
		if p.consume(')') {
			return out, nil
		}
		if len(out) > 0 && !p.consume(',') {
			return nil, fmt.Errorf("parse: expected ',' or ')' at offset %d", p.pos)
		}
		a, err := p.arg()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
}

func (p *Parser) arg() (arg, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return arg{}, fmt.Errorf("parse: unexpected end of input")
	}
	c := p.src[p.pos]
	switch {
	case c == '"' || c == '\'':
		return p.stringLit(c)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.numberLit()
	default:
		save := p.pos
		path, err := p.path()
		if err != nil {
			return arg{}, err
		}
		if p.peek() == '(' {
			p.pos = save
			sub, err := p.expr()
			if err != nil {
				return arg{}, err
			}
			return arg{kind: argCall, sub: sub}, nil
		}
		if len(path) != 1 {
			return arg{}, fmt.Errorf("parse: %q is not a name at offset %d", joinPath(path), save)
		}
		return arg{kind: argVar, str: path[0]}, nil
	}
}

func joinPath(path []string) string {
	out := ""
	for i, s := range path {
		if i > 0 {
			out += "."
		}
		out += s
	}
	return out
}

func (p *Parser) stringLit(quote byte) (arg, error) {
	p.pos++ // opening quote
	start := p.pos
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case quote:
			// No escapes: the literal is a substring of the source,
			// zero-copy.
			lit := p.src[start:p.pos]
			p.pos++
			return arg{kind: argString, str: lit}, nil
		case '\\':
			return p.stringLitEscaped(quote, start)
		}
		p.pos++
	}
	return arg{}, fmt.Errorf("parse: unterminated string at offset %d", p.pos)
}

// stringLitEscaped is the slow path taken at the first backslash: the
// literal needs unescaping into a buffer.
func (p *Parser) stringLitEscaped(quote byte, start int) (arg, error) {
	buf := append([]byte(nil), p.src[start:p.pos]...)
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case quote:
			p.pos++
			return arg{kind: argString, str: string(buf)}, nil
		case '\\':
			if p.pos+1 >= len(p.src) {
				return arg{}, fmt.Errorf("parse: unterminated escape at offset %d", p.pos)
			}
			p.pos++
			buf = append(buf, p.src[p.pos])
		default:
			buf = append(buf, c)
		}
		p.pos++
	}
	return arg{}, fmt.Errorf("parse: unterminated string at offset %d", p.pos)
}

func (p *Parser) numberLit() (arg, error) {
	start := p.pos
	if p.src[p.pos] == '-' {
		p.pos++
	}
	float := false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '.' && !float {
			float = true
			p.pos++
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		p.pos++
	}
	lit := p.src[start:p.pos]
	if float {
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return arg{}, fmt.Errorf("parse: bad float %q: %w", lit, err)
		}
		return arg{kind: argFloat, f: f}, nil
	}
	i, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		return arg{}, fmt.Errorf("parse: bad int %q: %w", lit, err)
	}
	return arg{kind: argInt, i: i}, nil
}

func (p *Parser) keyword(kw string) bool {
	p.skipSpace()
	end := p.pos + len(kw)
	if end > len(p.src) || p.src[p.pos:end] != kw {
		return false
	}
	if end < len(p.src) && isIdentChar(p.src[end]) {
		return false
	}
	p.pos = end
	return true
}

func (p *Parser) ident() string {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.src) && isIdentChar(p.src[p.pos]) {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *Parser) consume(c byte) bool {
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

func (p *Parser) consumeStr(s string) bool {
	p.skipSpace()
	end := p.pos + len(s)
	if end > len(p.src) || p.src[p.pos:end] != s {
		return false
	}
	p.pos = end
	return true
}

// peek returns the next non-space byte without consuming it, or 0 at
// the end of the source.
func (p *Parser) peek() byte {
	save := p.pos
	p.skipSpace()
	c := byte(0)
	if p.pos < len(p.src) {
		c = p.src[p.pos]
	}
	p.pos = save
	return c
}

func (p *Parser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func isIdentChar(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
