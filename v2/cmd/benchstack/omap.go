// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"golang.org/x/perf/v2/benchproc"
)

// OMap is an insertion-ordered map keyed by Config.
//
// The zero value of OMap is a usable map.
type OMap struct {
	// New is called to create new values for LoadOrNew.
	New func(key benchproc.Config) interface{}

	// Keys is the keys of this map in insertion order.
	Keys []benchproc.Config

	// KeyPos is a map from key to that key's position in the map.
	KeyPos map[benchproc.Config]int

	vals map[benchproc.Config]interface{}
}

// Load returns the value associated with key, or nil if key is not in
// the map.
func (m *OMap) Load(key benchproc.Config) interface{} {
	return m.vals[key]
}

// LoadOK returns the value associated with key and whether or not it
// is in the map.
func (m *OMap) LoadOK(key benchproc.Config) (interface{}, bool) {
	val, ok := m.vals[key]
	if !ok {
		_, ok = m.KeyPos[key]
	}
	return val, ok
}

// LoadOrNew is like Load, but if key doesn't exist, it first invokes
// m.New and stores the returned value under key.
func (m *OMap) LoadOrNew(key benchproc.Config) interface{} {
	val, ok := m.LoadOK(key)
	if !ok {
		val = m.New(key)
		m.Store(key, val)
	}
	return val
}

// Store sets key's value to value. If this is the first time key has
// been stored, it adds key to the insertion order.
func (m *OMap) Store(key benchproc.Config, value interface{}) {
	if value != nil {
		if m.vals == nil {
			m.vals = make(map[benchproc.Config]interface{})
		}
		m.vals[key] = value
	} else if m.vals != nil {
		delete(m.vals, key)
	}

	if _, ok := m.KeyPos[key]; !ok {
		if m.KeyPos == nil {
			m.KeyPos = make(map[benchproc.Config]int)
		}
		m.KeyPos[key] = len(m.Keys)
		m.Keys = append(m.Keys, key)
	}
}

// Map applies f to every value in m and returns an OMap of its results.
func (m *OMap) Map(f func(key benchproc.Config, val interface{}) interface{}) *OMap {
	copyPos := func(x map[benchproc.Config]int) map[benchproc.Config]int {
		out := make(map[benchproc.Config]int, len(x))
		for k, v := range x {
			out[k] = v
		}
		return out
	}
	out := &OMap{
		Keys:   append([]benchproc.Config(nil), m.Keys...),
		KeyPos: copyPos(m.KeyPos),
		vals:   make(map[benchproc.Config]interface{}),
	}
	for k, v := range m.vals {
		out.vals[k] = f(k, v)
	}
	return out
}
