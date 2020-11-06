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
//   match   = "(" expr ")"
//           | "-" match
//           | "*"
//           | word ":" (word | "(" {word} ")") .
//   word    = [^ ():]* | "\"" [^"]* "\""
package kvql

import (
	"fmt"
	"regexp"
	"strconv"
	"unicode"
)

// Parse parses a query string into a Query tree.
func Parse(q string) (Query, error) {
	toks, err := Tokenize(q)
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

func parse(qOrig string, toks []Tok) (Query, error) {
	// Rewrite tokens to find operators.
	for i, tok := range toks {
		if tok.Kind == 'w' {
			switch tok.Tok {
			case "AND":
				toks[i].Kind = 'A'
			case "OR":
				toks[i].Kind = 'O'
			}
		} else if tok.Kind == 'q' {
			// Once we've recognized operators, we don't
			// care about quoted/unquoted.
			toks[i].Kind = 'w'
		}
	}

	p := parser{qOrig, toks, nil}
	q, i := p.expr(0)
	if p.toks[i].Kind != 0 {
		p.error(i, "unexpected "+strconv.Quote(p.toks[i].Tok))
	}
	if p.err != nil {
		return nil, p.err
	}
	return q, nil
}

type parser struct {
	q    string
	toks []Tok
	err  *SyntaxError
}

func (p *parser) error(i int, msg string) int {
	off := p.toks[i].Off
	if p.err == nil || off < p.err.Off {
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
	if p.toks[i].Kind != 'O' {
		return q, i
	}
	terms := []Query{q}
	for p.toks[i].Kind == 'O' {
		q, i = p.andExpr(i + 1)
		terms = append(terms, q)
	}
	return &QueryOp{OpOr, terms}, i
}

func (p *parser) andExpr(i int) (Query, int) {
	var q Query
	q, i = p.phrase(i)
	if p.toks[i].Kind != 'A' {
		return q, i
	}
	terms := []Query{q}
	for p.toks[i].Kind == 'A' {
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
		switch p.toks[i].Kind {
		case '(', '-', 'w', '*':
			q, i = p.match(i)
			terms = append(terms, q)
		case ')', 'A', 'O', 0:
			break loop
		default:
			return nil, p.error(i, "unexpected "+strconv.Quote(p.toks[i].Tok))
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
	switch p.toks[i].Kind {
	case '(':
		q, i := p.expr(i + 1)
		if p.toks[i].Kind != ')' {
			return nil, p.error(i, "missing \")\"")
		}
		return q, i + 1
	case '-':
		q, i := p.match(i + 1)
		q = &QueryOp{OpNot, []Query{q}}
		return q, i
	case '*':
		q := &QueryOp{OpAnd, nil}
		return q, i + 1
	case 'w':
		off := p.toks[i].Off
		key := p.toks[i].Tok
		if p.toks[i+1].Kind != ':' {
			// TODO: Support other operators
			return nil, p.error(i, "expected key:value")
		}
		switch p.toks[i+2].Kind {
		default:
			return nil, p.error(i, "expected key:value")
		case 'w':
			// Simple match.
			return p.matchWord(i+2, off, key)
		case '(':
			// Multi-match.
			terms := []Query{}
			for i += 3; p.toks[i].Kind == 'w'; {
				var q Query
				q, i = p.matchWord(i, off, key)
				terms = append(terms, q)
			}
			if p.toks[i].Kind != ')' {
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

func (p *parser) matchWord(i int, keyOff int, key string) (Query, int) {
	if p.toks[i].Kind != 'w' {
		panic("matchWord called on non-word token")
	}
	// Make sure the regexp is well-formed before we manipulate
	// the string.
	_, err := regexp.Compile(p.toks[i].Tok)
	if err != nil {
		return nil, p.error(i, err.Error())
	}

	// Now make the regexp we'll actually use.
	re := regexp.MustCompile("^(?:" + p.toks[i].Tok + ")$")
	return &QueryMatch{keyOff, key, re, p.toks[i].Tok}, i + 1
}
