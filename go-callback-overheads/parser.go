package callbacks

import (
	"fmt"
	"strconv"
)

// Parser turns a statement into a callExpr. The grammar is a single
// return statement:
//
//	stmt := "return" ident "(" [ arg { "," arg } ] ")" [ ";" ]
//	arg  := string | number | ident
//
// Strings are single- or double-quoted. Numbers map to int64 when they
// have no decimal point and float64 when they do; no other numeric
// types exist. A bare ident is a variable reference resolved against
// the stack at execution time.
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
)

// arg is one parsed argument: a literal value or a variable name.
type arg struct {
	kind argKind
	str  string // argString value or argVar name
	i    int64
	f    float64
}

// callExpr is the parsed form of the return statement. args slices
// into argv while the call has at most four arguments, so parsing a
// typical statement allocates only the callExpr itself. Only used by
// pointer: copying the struct would leave args aliasing the source.
type callExpr struct {
	name string
	args []arg
	argv [4]arg
}

// Parse parses one statement.
func (p *Parser) Parse(src string) (*callExpr, error) {
	p.src, p.pos = src, 0

	if !p.keyword("return") {
		return nil, fmt.Errorf("parse: expected 'return' at offset %d", p.pos)
	}
	name := p.ident()
	if name == "" {
		return nil, fmt.Errorf("parse: expected function name at offset %d", p.pos)
	}
	if !p.consume('(') {
		return nil, fmt.Errorf("parse: expected '(' at offset %d", p.pos)
	}

	call := &callExpr{name: name}
	call.args = call.argv[:0]
	for {
		p.skipSpace()
		if p.consume(')') {
			break
		}
		if len(call.args) > 0 && !p.consume(',') {
			return nil, fmt.Errorf("parse: expected ',' or ')' at offset %d", p.pos)
		}
		a, err := p.arg()
		if err != nil {
			return nil, err
		}
		call.args = append(call.args, a)
	}

	p.consume(';')
	p.skipSpace()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("parse: trailing input at offset %d", p.pos)
	}
	return call, nil
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
		name := p.ident()
		if name == "" {
			return arg{}, fmt.Errorf("parse: unexpected character %q at offset %d", c, p.pos)
		}
		return arg{kind: argVar, str: name}, nil
	}
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
	return c == '_' || c == '.' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
