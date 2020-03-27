// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"sort"

	"golang.org/x/perf/v2/benchproc"
)

// OMap is an insertion-ordered map keyed by *Config.
//
// The zero value of OMap is a usable map.
type OMap struct {
	// New is called to create new values for LoadOrNew.
	New func(key *benchproc.Config) interface{}

	// Keys is the keys of this map in insertion order.
	Keys []*benchproc.Config

	// KeyPos is a map from key to that key's position in the map.
	KeyPos map[*benchproc.Config]int

	vals map[*benchproc.Config]interface{}
}

// Load returns the value associated with key, or nil if key is not in
// the map.
func (m *OMap) Load(key *benchproc.Config) interface{} {
	return m.vals[key]
}

// LoadOK returns the value associated with key and whether or not it
// is in the map.
func (m *OMap) LoadOK(key *benchproc.Config) (interface{}, bool) {
	val, ok := m.vals[key]
	if !ok {
		_, ok = m.KeyPos[key]
	}
	return val, ok
}

// LoadOrNew is like Load, but if key doesn't exist, it first invokes
// m.New and stores the returned value under key.
func (m *OMap) LoadOrNew(key *benchproc.Config) interface{} {
	val, ok := m.LoadOK(key)
	if !ok {
		val = m.New(key)
		m.Store(key, val)
	}
	return val
}

// Store sets key's value to value. If this is the first time key has
// been stored, it adds key to the insertion order.
func (m *OMap) Store(key *benchproc.Config, value interface{}) {
	if value != nil {
		if m.vals == nil {
			m.vals = make(map[*benchproc.Config]interface{})
		}
		m.vals[key] = value
	} else if m.vals != nil {
		delete(m.vals, key)
	}

	if _, ok := m.KeyPos[key]; !ok {
		if m.KeyPos == nil {
			m.KeyPos = make(map[*benchproc.Config]int)
		}
		m.KeyPos[key] = len(m.Keys)
		m.Keys = append(m.Keys, key)
	}
}

type OMap2D struct {
	Rows, Cols []*benchproc.Config

	RowPos, ColPos map[*benchproc.Config]int

	cells map[oMap2DKey]interface{}

	New func(row, col *benchproc.Config) interface{}
}

type oMap2DKey struct {
	a, b *benchproc.Config
}

func (m *OMap2D) Load(row, col *benchproc.Config) interface{} {
	return m.cells[oMap2DKey{row, col}]
}

func (m *OMap2D) LoadOK(row, col *benchproc.Config) (interface{}, bool) {
	cell, ok := m.cells[oMap2DKey{row, col}]
	return cell, ok
}

func (m *OMap2D) LoadOrNew(row, col *benchproc.Config) interface{} {
	cell, ok := m.cells[oMap2DKey{row, col}]
	if !ok {
		cell = m.New(row, col)
		m.Store(row, col, cell)
	}
	return cell
}

func (m *OMap2D) Store(row, col *benchproc.Config, value interface{}) {
	if m.cells == nil {
		m.cells = make(map[oMap2DKey]interface{})
		m.RowPos = make(map[*benchproc.Config]int)
		m.ColPos = make(map[*benchproc.Config]int)
	}

	m.cells[oMap2DKey{row, col}] = value
	if _, ok := m.RowPos[row]; !ok {
		m.RowPos[row] = len(m.Rows)
		m.Rows = append(m.Rows, row)
	}
	if _, ok := m.ColPos[col]; !ok {
		m.ColPos[col] = len(m.Cols)
		m.Cols = append(m.Cols, col)
	}
}

func (m *OMap2D) Map(f func(row, col *benchproc.Config, val interface{}) interface{}) {
	for k, v := range m.cells {
		m.cells[k] = f(k.a, k.b, v)
	}
}

func (m *OMap2D) Sort(rowLess, colLess func(a, b *benchproc.Config) bool) {
	if len(m.Rows) == 0 {
		return
	}
	sort1 := func(less func(a, b *benchproc.Config) bool, sl []*benchproc.Config, m map[*benchproc.Config]int) {
		// Sort the slice.
		sort.Slice(sl, func(i, j int) bool {
			return less(sl[i], sl[j])
		})
		// Reconstruct the position map.
		for i, cfg := range sl {
			m[cfg] = i
		}
	}
	if rowLess != nil {
		sort1(rowLess, m.Rows, m.RowPos)
	}
	if colLess != nil {
		sort1(colLess, m.Cols, m.ColPos)
	}
}
