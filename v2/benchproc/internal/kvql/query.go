// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kvql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Query is a node in the query tree. It can either be a QueryOp or a
// QueryMatch.
type Query interface {
	isQuery()
	String() string
}

// QueryMatch is a leaf in a Query tree that tests a specific key for
// a match.
type QueryMatch struct {
	Off   int // Byte offset of the key in the original query.
	Key   string
	match string
}

func (q *QueryMatch) isQuery() {}
func (q *QueryMatch) String() string {
	quote := func(s string) string {
		for _, r := range s {
			if unicode.IsSpace(r) {
				r = ' '
			}
			switch r {
			case '"', ' ', '(', ')', ':':
				return strconv.Quote(s)
			}
		}
		// No quoting necessary.
		return s
	}
	return quote(q.Key) + ":" + quote(q.match)
}

// Match returns whether q matches the given value of q.Key.
func (q *QueryMatch) Match(value string) bool {
	return value == q.match
}

// QueryOp is a boolean operator in the Query tree. OpNot must have
// exactly one child node. OpAnd and OpOr may have zero or more child
// nodes.
type QueryOp struct {
	Op    Op
	Exprs []Query
}

func (q *QueryOp) isQuery() {}
func (q *QueryOp) String() string {
	var op string
	switch q.Op {
	case OpNot:
		return fmt.Sprintf("-%s", q.Exprs[0])
	case OpAnd:
		op = " AND "
	case OpOr:
		op = " OR "
	}
	var buf strings.Builder
	buf.WriteByte('(')
	for i, e := range q.Exprs {
		if i > 0 {
			buf.WriteString(op)
		}
		buf.WriteString(e.String())
	}
	buf.WriteByte(')')
	return buf.String()
}

// Op specifies a type of boolean operator.
type Op int

const (
	OpAnd Op = 1 + iota
	OpOr
	OpNot
)
