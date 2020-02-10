// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

// A ConfigSorter imposes a total order on Configs.
type ConfigSorter interface {
	ConfigLess(a, b *Config) bool
}

// A ConfigTracker tracks a set of *Configs in order of first
// observation. It is also a ConfigSorter that sorts by this order.
//
// The zero value of a ConfigTracker is valid.
type ConfigTracker struct {
	// Configs is the set of distinct Configs observed by this
	// ConfigTracker, in order of first observation.
	Configs []*Config

	// Order is the index of each Config in Configs.
	Order map[*Config]int
}

// Add adds cfg to the set of Configs in t, if it is not already
// present.
func (t *ConfigTracker) Add(cfg *Config) {
	if t.Order == nil {
		t.Order = make(map[*Config]int)
	}
	if _, ok := t.Order[cfg]; ok {
		return
	}
	t.Order[cfg] = len(t.Configs)
	t.Configs = append(t.Configs, cfg)
}

// ConfigLess returns whether a was observed before b. If only one of
// a or b was observed, it considers the observed one to come first.
// If neither was observed, it falls back to ConfigSortAlpha.
func (t *ConfigTracker) ConfigLess(a, b *Config) bool {
	i1, ok1 := t.Order[a]
	i2, ok2 := t.Order[b]
	switch {
	case ok1 && ok2:
		return i1 < i2
	case ok1 || ok2:
		// Ordered configs come before unordered configs.
		return ok1
	default:
		// Fall back sort.
		return ConfigSortAlpha.ConfigLess(a, b)
	}
}

type configSortAlpha struct{}

// ConfigSortAlpha is a ConfigSorter that sorts *Configs
// alphabetically by key and then value. Key/value *Configs come
// before tuple *Configs. Tuple *Configs are compared
// lexicographically, with each element compared by ConfigSortAlpha.
var ConfigSortAlpha = configSortAlpha{}

func (configSortAlpha) ConfigLess(a, b *Config) bool {
	switch {
	case a == b:
		return false
	case a.IsKeyVal() && b.IsKeyVal():
		k1, v1 := a.KeyVal()
		k2, v2 := b.KeyVal()
		if k1 != k2 {
			return k1 < k2
		}
		return v1 < v2
	case a.IsKeyVal() || b.IsKeyVal():
		// Key/value configs win.
		return a.IsKeyVal()
	default:
		return compareTuples(a, b, configSortAlpha{}) < 0
	}
}

func compareTuples(a, b *Config, sorter ConfigSorter) int {
	// The nil tuple sorts before everything else. Handling this
	// case up-front lets us access tupleLen safely.
	if a == nil || b == nil {
		if a == b {
			return 0
		}
		if a == nil {
			return -1
		}
		return 1
	}

	aLen, bLen := a.tupleLen, b.tupleLen
	// Make a always the shorter tuple.
	if aLen < bLen {
		return compareTuplesRec(a, b, sorter)
	}
	return -compareTuplesRec(b, a, sorter)
}

func compareTuplesRec(a, b *Config, sorter ConfigSorter) int {
	// If we've reached the same tuple prefix, stop.
	if a == b {
		return 0
	}
	// If b is longer, strip off an element.
	if a.tupleLen < b.tupleLen {
		sub := compareTuplesRec(a, b.prefix, sorter)
		if sub != 0 {
			return sub
		}
		// Prefixes are equal, so the shorter tuple wins.
		return -1
	}
	// Tuples are the same length and not equal. Check prefix,
	// then last element.
	if a.tupleLen == b.tupleLen {
		sub := compareTuplesRec(a.prefix, b.prefix, sorter)
		if sub != 0 {
			return sub
		}
		// Prefix are equal, so compare this element.
		//
		// XXX This requires the ConfigLess compare equal if
		// and only if a == b.
		if a.elt == b.elt {
			return 0
		} else if sorter.ConfigLess(a.elt, b.elt) {
			return -1
		} else {
			return 1
		}
	}
	panic("inconsistent tuple size")
}

// XXX Should he individual sorters have an "I don't know" (partial
// order) and be invoked recursively by a general tuple sorter? The
// tuple sorter turns a partial order into a total order.
