// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import "sort"

// Less returns true if c comes before o in the sort order implied by
// their schema. It panics if c and o have different schemas.
func (c Config) Less(o Config) bool {
	if c.c.schema != o.c.schema {
		panic("cannot compare Configs from different Schemas")
	}
	return less(c.c.schema.Fields(), c.c.vals, o.c.vals)
}

func less(flat []Field, a, b []string) bool {
	// Walk the tuples in schema order.
	for _, node := range flat {
		var aa, bb string
		if node.idx < len(a) {
			aa = a[node.idx]
		}
		if node.idx < len(b) {
			bb = b[node.idx]
		}
		if aa != bb {
			if node.less == nil {
				// Sort by observation order.
				return node.order[aa] < node.order[bb]
			}
			return node.less(aa, bb)
		}
	}

	// Tuples are equal.
	return false
}

// SortConfigs sorts a slice of Configs using Config.Less. All configs
// must have the same Schema.
//
// This is equivalent to using Config.Less with the sort package, but
// is more efficient.
func SortConfigs(configs []Config) {
	// Check all the schemas so we don't have to do this on every
	// comparison.
	if len(configs) == 0 {
		return
	}
	s := commonSchema(configs)
	flat := s.Fields()

	sort.Slice(configs, func(i, j int) bool {
		return less(flat, configs[i].c.vals, configs[j].c.vals)
	})
}
