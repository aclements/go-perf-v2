// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"fmt"

	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc/internal/kvql"
)

// A Filter filters benchmarks and benchmark observations.
type Filter struct {
	// query is the filter query.
	query kvql.Query

	// extractors records functions for extracting keys for
	// QueryMatch nodes.
	extractors map[string]func(*benchfmt.Result) string

	// usesUnits indicates that the results of this filter may be
	// different for different units.
	usesUnits bool
}

// NewFilter constructs a result filter from a boolean query.
func NewFilter(query string) (*Filter, error) {
	q, err := kvql.Parse(query)
	if err != nil {
		return nil, err
	}

	// Collect extractors for different keys.
	f := &Filter{
		query:      q,
		extractors: make(map[string]func(*benchfmt.Result) string),
	}
	var walk func(q kvql.Query) error
	walk = func(q kvql.Query) error {
		switch q := q.(type) {
		default:
			panic(fmt.Sprintf("unknown query node type %T", q))
		case *kvql.QueryOp:
			for _, sub := range q.Exprs {
				if err := walk(sub); err != nil {
					return err
				}
			}
		case *kvql.QueryMatch:
			if _, ok := f.extractors[q.Key]; ok {
				break
			}
			if q.Key == ".unit" {
				f.usesUnits = true
			} else {
				ext, err := benchfmt.NewExtractor(q.Key)
				if err != nil {
					return &kvql.SyntaxError{query, q.Off, err.Error()}
				}
				f.extractors[q.Key] = ext
			}
		}
		return nil
	}
	if err := walk(q); err != nil {
		return nil, err
	}

	return f, nil
}

// Match returns the set of res.Values that match f.
func (f *Filter) Match(res *benchfmt.Result) Match {
	// TODO: Most of the time file keys don't change. If Result
	// can have some generation indicator (a pair of a pointer
	// nonce and a counter?), I can use partial evaluation to
	// avoid even processing results if the outcome is going to be
	// false because of the file keys. If any input to an AND is
	// false, the AND is false. If any input to an OR is true, the
	// OR is true. I can pre-compute whether a change in some file
	// key is necessary to change the result if it's currently
	// true or currently false and cache the previous result.
	//
	// Actually, it would be far simpler if I just took advantage
	// of short-circuit evaluation and reordered the expression to
	// put "easy" things like file keys first and name keys last.
	// Short-circuiting would require that the intermediate
	// matchBuilder be able to answer "any" and "all" questions.
	// (For that, it might be better to just track a weight.)

	m := f.match(res, f.query)
	return m.finish(!f.usesUnits, len(res.Values))
}

func (f *Filter) match(res *benchfmt.Result, node kvql.Query) (m matchBuilder) {
	switch node := node.(type) {
	case *kvql.QueryOp:
		if len(node.Exprs) == 0 {
			if f.usesUnits {
				m = newMatchBuilder(len(res.Values))
			}
			switch node.Op {
			case kvql.OpAnd:
				m.setAll()
				return
			case kvql.OpOr:
				return
			}
		}

		m = f.match(res, node.Exprs[0])
		switch node.Op {
		case kvql.OpNot:
			m.head = ^m.head
			for i := range m.rest {
				m.rest[i] = ^m.rest[i]
			}
		case kvql.OpAnd:
			for _, sub := range node.Exprs[1:] {
				m2 := f.match(res, sub)
				m.head &= m2.head
				for i := range m.rest {
					m.rest[i] &= m2.rest[i]
				}
			}
		case kvql.OpOr:
			for _, sub := range node.Exprs[1:] {
				m2 := f.match(res, sub)
				m.head |= m2.head
				for i := range m.rest {
					m.rest[i] |= m2.rest[i]
				}
			}
		}

	case *kvql.QueryMatch:
		if f.usesUnits {
			m = newMatchBuilder(len(res.Values))
		}
		// If we're not tracking units, we only use bit 0 of
		// the match.

		if f.usesUnits && node.Key == ".unit" {
			// Find the units this matches.
			for i := range res.Values {
				if node.Match(res.Values[i].Unit) {
					m.set(i)
				}
			}
			return
		}
		ext := f.extractors[node.Key]
		if node.Match(ext(res)) {
			m.setAll()
		}
	}
	return
}

type matchBuilder struct {
	head uint64
	rest []uint64
}

func newMatchBuilder(n int) matchBuilder {
	if n <= 64 {
		return matchBuilder{}
	}
	return matchBuilder{rest: make([]uint64, (n+63)/64-1)}
}

func (m *matchBuilder) set(i int) {
	if i < 64 {
		m.head |= 1 << i
	} else {
		m.rest[i/64-1] |= 1 << (i % 64)
	}
}

func (m *matchBuilder) setAll() {
	m.head = ^uint64(0)
	for i := range m.rest {
		m.rest[i] = ^uint64(0)
	}
}

func (m *matchBuilder) finish(broadcast bool, n int) Match {
	out := Match{n: n, head: m.head, rest: m.rest}
	if broadcast {
		// Broadcast bit 0 to all bits.
		out.allEqual = true
	} else {
		// Compute allEqual.
		b0 := m.head&1 != 0
		for i := 1; i < n; i++ {
			if out.Test(i) != b0 {
				return out
			}
		}
		out.allEqual = true
		out.head &= 1
		out.rest = nil
	}
	return out
}

// A Match records the set of result values that matched a filter
// query.
type Match struct {
	// n is the number of bits in this match.
	n int

	// allEqual means bit 0 holds the state for all bits.
	allEqual bool

	head uint64
	rest []uint64
}

// All returns true if all values in a result matched the query.
func (m *Match) All() bool {
	return m.allEqual && m.head&1 != 0
}

// Any return true if any values in a result matched the query.
func (m *Match) Any() bool {
	return !m.allEqual || m.head&1 != 0
}

// Test tests whether value i matched the query.
func (m *Match) Test(i int) bool {
	if i < 0 || i >= m.n {
		return false
	} else if m.allEqual {
		return m.head&1 != 0
	} else if i < 64 {
		return m.head&(1<<i) != 0
	}
	return m.rest[i/64-1]&(1<<(i%64)) != 0
}

// Apply removes values from res that don't match m and returns
// whether any values matched.
func (m *Match) Apply(res *benchfmt.Result) bool {
	if m.All() {
		return true
	}
	if !m.Any() {
		res.Values = res.Values[:0]
		return false
	}

	j := 0
	for i, val := range res.Values {
		if m.Test(i) {
			res.Values[j] = val
			j++
		}
	}
	res.Values = res.Values[:j]
	return j > 0
}
