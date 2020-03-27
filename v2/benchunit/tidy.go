// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchunit

import (
	"strings"
	"sync"
)

// A Tidier rewrites units and values to normalize to base units,
// specifically normalizing common pre-scaled units like "ns" to "sec"
// and "MB" to "B". This is important to do before then applying a
// scaler to values so the scaler doesn't result in nonsense units
// like "megananoseconds".
//
// The zero value of this type is an initialized Tidier. This type
// provides a type-safe cache of tidied units for performance.
type Tidier struct {
	cache sync.Map // unit string -> *tidyCache
}

type tidyCache struct {
	tidied string
	factor float64
}

// Tidy returns the tidied version of unit and the multiplicative
// factor to convert a value in unit "unit" to a value in unit
// "tidied".
func (t *Tidier) Tidy(unit string) (tidied string, factor float64) {
	// Fast path for units from testing package.
	switch unit {
	case "ns/op":
		return "sec/op", 1e-9
	case "MB/s":
		return "B/s", 1e6
	case "B/op", "allocs/op":
		return unit, 1
	}
	// Fast path for units with no normalization.
	if !(strings.Contains(unit, "ns") || strings.Contains(unit, "MB")) {
		return unit, 1
	}

	// Check the cache.
	if tc, ok := t.cache.Load(unit); ok {
		tc := tc.(*tidyCache)
		return tc.tidied, tc.factor
	}

	// Do the hard work and cache it.
	tidied, factor = tidy(unit)
	t.cache.Store(unit, &tidyCache{tidied, factor})
	return
}

type edit struct {
	pos, len int
	replace  string
}

func tidy(unit string) (tidied string, factor float64) {
	// The caller has handled the fast paths. Parse the unit.
	factor = 1
	p := newParser(unit)
	edits := make([]edit, 0, 4)
	for p.next() {
		if p.denom {
			// Don't edit in the denominator.
			continue
		}
		switch p.tok {
		case "ns":
			edits = append(edits, edit{p.pos, len("ns"), "sec"})
			factor /= 1e9
		case "MB":
			edits = append(edits, edit{p.pos, len("MB"), "B"})
			factor *= 1e6
		}
	}
	// Apply edits.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		unit = unit[:e.pos] + e.replace + unit[e.pos+e.len:]
	}
	return unit, factor
}
