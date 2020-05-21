// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package kvql implements a generic key-value query language.
//
// Syntax:
//
//   expr    = andExpr {"OR" andExpr} .
//   andExpr = phrase {"AND" phrase} .
//   phrase  = match {match} .
//   match   = "(" expr ")" .
//           | "-" match
//           | word ":" (word | "(" {word} ")")
//   word    = [^ ():]* | "\"" [^"]* "\""
package kvql

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// Parse parses a query string into a Query tree.
func Parse(q string) (Query, error) {
	toks, err := tokenize(q)
	if err != nil {
		return nil, err
	}
	return parse(q, toks)
}

// SyntaxError is an error produced by parsing a malformed query
// string.
type SyntaxError struct {
	Query string // The query string
	Off   int    // Byte offset of the error in Query
	Msg   string // Error message
}

func (e *SyntaxError) Error() string {
	// Translate byte offset to a rune offset.
	pos := 0
	for i, r := range e.Query {
		if i >= e.Off {
			break
		}
		if unicode.IsGraphic(r) {
			pos++
		}
	}
	return fmt.Sprintf("syntax error: %s\n\t%s\n\t%*s^", e.Msg, e.Query, pos, "")
}

type tok struct {
	kind byte
	off  int
	tok  string
}

func tokenize(q string) ([]tok, error) {
	qOrig := q
	tokWord := func(q string) (q2 string, word string, quoted bool, err error) {
		off := len(qOrig) - len(q)
		if q[0] == '"' {
			// Consume a quoted word.
			//
			// TODO: Escape sequences.
			pos := 1
			for pos < len(q) && q[pos] != '"' {
				pos++
			}
			if pos == len(q) {
				return "", "", false, &SyntaxError{qOrig, off, "missing end quote"}
			}
			return q[pos+1:], q[1:pos], true, nil
		}
		// Consume until a space or operator. We only take "-"
		// as an operator immediately following another space
		// or operator so things like "foo-bar" work as
		// expected.
		for i, r := range q {
			if unicode.IsSpace(r) {
				r = ' '
			}
			switch r {
			case ' ', '(', ')', ':':
				return q[i:], q[:i], false, nil
			}
		}
		return "", q, false, nil
	}

	var toks []tok
	for len(q) > 0 {
		off := len(qOrig) - len(q)
		if q[0] == '(' || q[0] == ')' || q[0] == '-' {
			toks = append(toks, tok{q[0], off, q[:1]})
			q = q[1:]
		} else if n := isSpace(q); n > 0 {
			q = q[n:]
		} else if q2, word, quoted, err := tokWord(q); err != nil {
			return nil, err
		} else {
			q = q2
			if len(q) > 0 && q[0] == ':' {
				// TODO: Support other operators
				if len(word) == 0 {
					return nil, &SyntaxError{qOrig, off, "missing key"}
				}
				q = q[1:] // Skip the colon
				toks = append(toks, tok{'=', off, word})
			} else if !quoted && word == "AND" {
				toks = append(toks, tok{'A', off, word})
			} else if !quoted && word == "OR" {
				toks = append(toks, tok{'O', off, word})
			} else {
				toks = append(toks, tok{'w', off, word})
			}
		}
	}
	// Add an EOF token. This eliminates the need for lots of
	// bounds checks in the parer and gives the EOF a position.
	toks = append(toks, tok{0, len(qOrig), ""})
	return toks, nil
}

func isSpace(q string) int {
	if q[0] == ' ' {
		return 1
	}
	r, size := utf8.DecodeRuneInString(q)
	if unicode.IsSpace(r) {
		return size
	}
	return 0
}

func parse(qOrig string, toks []tok) (Query, error) {
	p := parser{qOrig, toks, nil}
	q, i := p.expr(0)
	if p.toks[i].kind != 0 {
		p.error(i, "unexpected "+strconv.Quote(p.toks[i].tok))
	}
	if p.err != nil {
		return nil, p.err
	}
	return q, nil
}

type parser struct {
	q    string
	toks []tok
	err  *SyntaxError
}

func (p *parser) error(i int, msg string) int {
	off := p.toks[i].off
	if p.err == nil || p.err.Off > off {
		p.err = &SyntaxError{p.q, off, msg}
	}
	// Move to the end token.
	return len(p.toks) - 1
}

func (p *parser) expr(i int) (Query, int) {
	return p.orExpr(i)
}

func (p *parser) orExpr(i int) (Query, int) {
	var q Query
	q, i = p.andExpr(i)
	if p.toks[i].kind != 'O' {
		return q, i
	}
	terms := []Query{q}
	for p.toks[i].kind == 'O' {
		q, i = p.andExpr(i + 1)
		terms = append(terms, q)
	}
	return &QueryOp{OpOr, terms}, i
}

func (p *parser) andExpr(i int) (Query, int) {
	var q Query
	q, i = p.phrase(i)
	if p.toks[i].kind != 'A' {
		return q, i
	}
	terms := []Query{q}
	for p.toks[i].kind == 'A' {
		q, i = p.phrase(i + 1)
		terms = append(terms, q)
	}
	return &QueryOp{OpAnd, terms}, i
}

func (p *parser) phrase(i int) (Query, int) {
	var q Query
	var terms []Query
loop:
	for {
		switch p.toks[i].kind {
		case '(', '-', '=':
			q, i = p.match(i)
			terms = append(terms, q)
		case ')', 'A', 'O', 0:
			break loop
		case 'w':
			return nil, p.error(i, "expected key:value")
		default:
			panic(fmt.Sprintf("unknown token type %#v", p.toks[i]))
		}
	}
	if len(terms) == 0 {
		return nil, p.error(i, "nothing to match")
	}
	if len(terms) == 1 {
		return terms[0], i
	}
	return &QueryOp{OpAnd, terms}, i
}

func (p *parser) match(i int) (Query, int) {
	switch p.toks[i].kind {
	case '(':
		q, i := p.expr(i + 1)
		if p.toks[i].kind != ')' {
			return nil, p.error(i, "missing \")\"")
		}
		return q, i + 1
	case '-':
		q, i := p.match(i + 1)
		q = &QueryOp{OpNot, []Query{q}}
		return q, i
	case '=':
		off := p.toks[i].off
		switch p.toks[i+1].kind {
		case 'w':
			// Simple match.
			q := &QueryMatch{off, p.toks[i].tok, p.toks[i+1].tok}
			return q, i + 2
		case '(':
			// Multi-match.
			key := p.toks[i].tok
			terms := []Query{}
			for i += 2; p.toks[i].kind == 'w'; i++ {
				q := &QueryMatch{off, key, p.toks[i].tok}
				terms = append(terms, q)
			}
			if p.toks[i].kind != ')' {
				return nil, p.error(i, "expected value")
			}
			if len(terms) == 0 {
				return nil, p.error(i, "nothing to match")
			}
			q := &QueryOp{OpOr, terms}
			return q, i + 1
		}
	}
	return nil, p.error(i, "expected key:value or subexpression")
}
