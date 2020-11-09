// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

// A ConfigHeader is a node in a Config header tree. It represents a
// subslice of a slice of Configs that are all equal up to some
// prefix.
//
// Given a Config slice configs and ConfigHeader node n,
// configs[n.Start:n.Start+n.Len] are equal for all fields from 0 to
// n.Field-1.
type ConfigHeader struct {
	// Field is the index of the Schema field represented by this
	// node.
	Field int

	// Start is the index of the first Config covered by this
	// node.
	Start int
	// Len is the number of Configs in the sequence represented by
	// this node. Visually, this is also the cell span of this
	// node.
	Len int

	// Value is the value that all Configs have in common for
	// Field.
	Value string
}

// NewConfigHeader combines a sequence of Configs by common prefixes.
//
// This is intended to visually present a sequence of Configs in a
// compact form; for example, as a header over a table where each
// column is keyed by a Config.
//
// All Configs must have the same Schema. In the result, level[i]
// corresponds to field i of this Schema. The ConfigHeader nodes in
// level[i] form a disjoint subslicing of configs. For each
// ConfigHeader node, all Configs in the subslice represented by the
// node are identical for fields 0 through i-1. Hence, the
// ConfigHeaders also logically form a tree because each level
// subdivides the level above it.
func NewConfigHeader(configs []Config) (levels [][]*ConfigHeader) {
	if len(configs) == 0 {
		return nil
	}

	fields := commonSchema(configs).Fields()

	levels = make([][]*ConfigHeader, len(fields))
	prevLevel := []*ConfigHeader{&ConfigHeader{-1, 0, len(configs), ""}}
	// Walk through the levels of the tree, subdividing the nodes
	// from the previous level.
	for i, field := range fields {
		for _, parent := range prevLevel {
			var node *ConfigHeader
			for j, config := range configs[parent.Start : parent.Start+parent.Len] {
				val := config.Get(field)
				if node != nil && val == node.Value {
					node.Len++
				} else {
					node = &ConfigHeader{i, parent.Start + j, 1, val}
					levels[i] = append(levels[i], node)
				}
			}
		}
		prevLevel = levels[i]
	}
	return levels
}
