// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kvql

import (
	"unicode"
	"unicode/utf8"
)

// Tok is a single token in the kvql lexical syntax.
type Tok struct {
	// Kind specifies the category of this token. It is either 'w'
	// or 'q' for an unquoted or quoted word, respectively, an
	// operator character, or 0 for the end-of-string token.
	Kind byte
	Off  int    // Byte offset of the beginning of this token
	Tok  string // Literal token contents; quoted words are unescaped
}

func isOp(ch rune) bool {
	return ch == '(' || ch == ')' || ch == ':' || ch == '@' || ch == ','
}

// Tokenize splits q into a stream of tokens. Each token is either a
// quoted or unquoted word, or a single character operator. Quoted
// words are enclosed in double-quotes.
func Tokenize(q string) ([]Tok, error) {
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
			if unicode.IsSpace(r) || isOp(r) {
				return q[i:], q[:i], false, nil
			}
		}
		return "", q, false, nil
	}

	var toks []Tok
	for len(q) > 0 {
		off := len(qOrig) - len(q)
		// At the beginning of a word, we accept "-" and "*"
		// as operators, but in the middle of words we treat
		// them as part of the word.
		if isOp(rune(q[0])) || q[0] == '-' || q[0] == '*' {
			toks = append(toks, Tok{q[0], off, q[:1]})
			q = q[1:]
		} else if n := isSpace(q); n > 0 {
			q = q[n:]
		} else if q2, word, quoted, err := tokWord(q); err == nil {
			q = q2
			if quoted {
				toks = append(toks, Tok{'q', off, word})
			} else {
				toks = append(toks, Tok{'w', off, word})
			}
		} else {
			return nil, err
		}
	}
	// Add an EOF token. This eliminates the need for lots of
	// bounds checks in the parer and gives the EOF a position.
	toks = append(toks, Tok{0, len(qOrig), ""})
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
