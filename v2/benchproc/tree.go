// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import "fmt"

// A ConfigTree is a tree with a key/val Config at each node, with the
// property that all Configs at the Nth level of a ConfigTree have the
// same key (or are nil).
//
// This is intended for visually presenting a sequence of Configs in a
// compact form. For example, given a sequence of columns in a table,
// each keyed by a Config, these Configs can be collected into a
// ConfigTree to construct a compact header to show over the columns.
type ConfigTree struct {
	// Config is the Config at this node. This is a key/val Config
	// or nil. A nil Config indicates an empty cell in the tree,
	// which can happen if tuple Configs have different lengths.
	Config *Config
	// Width is the number of leaf nodes under this node. It's
	// also the sum of the widths of the children of this node,
	// unless this is a leaf, in which case it is 1. In a visual
	// representation, this is the number of cells this node
	// spans. At each level of a tree, the sum of all widths is
	// the same as at every other level.
	Width int
	// Children is the child nodes of this node. In the original
	// slice of Configs, these all share the same prefix.
	Children []*ConfigTree
}

// NewConfigTree computes the ConfigTree of a set of Configs. The
// Configs must all have a compatible schema. They will be flattened
// into that schema, and combined into a tree by their common
// prefixes. It also returns the sequence of keys corresponding to the
// levels in the returned tree. All non-nil Configs at the level i of
// the tree will have key keys[i].
func NewConfigTree(configs []*Config) (tree []*ConfigTree, keys []string) {
	if len(configs) == 0 {
		return nil, nil
	}

	schema := commonSchema(configs)
	// levels tracks the right spine of the tree as we build it.
	// levels[0] is a virtual root node to track the forest.
	levels := make([]*ConfigTree, schema.size+1)
	levels[0] = new(ConfigTree)
	var flat []*Config // buffer for config flattening
	for _, cfg := range configs {
		flat = schema.flatten(cfg, flat)
		// Iterate down the flattened representation. Merge
		// into the existing spine until we diverge, then
		// create new nodes.
		diverged := false
		for level, kv := range flat {
			node := levels[level+1]
			if diverged || node == nil || node.Config != kv {
				node = &ConfigTree{kv, 1, nil}
				levels[level+1] = node
				prev := levels[level]
				prev.Children = append(prev.Children, node)
				diverged = true
			} else {
				node.Width++
			}
		}
	}

	return levels[0].Children, schema.keys()
}

type schema struct {
	key   string
	tuple []*schema
	size  int
}

// commonSchema computes the common key structure of all of the
// Configs in the configs slice.
//
// commonSchema panics if configs don't have a common schema.
func commonSchema(configs []*Config) *schema {
	if len(configs) == 0 {
		return nil
	}

	return commonSchema1(configs, true)
}

func commonSchema1(configs []*Config, cow bool) *schema {
	if configs[0].IsKeyVal() {
		// Check that they're all key/vals and have the same
		// key.
		key, _ := configs[0].KeyVal()
		for _, cfg := range configs[1:] {
			if !cfg.IsKeyVal() {
				panic("mismatched key/val and tuple configs")
			}
			key2, _ := cfg.KeyVal()
			if key != key2 {
				panic(fmt.Sprintf("mismatched keys: %q, %q", key, key2))
			}
		}
		return &schema{key: key, size: 1}
	}

	// They must all be tuple configs. Check them recursively.
	maxLen := 0
	for _, cfg := range configs {
		if cfg.IsKeyVal() {
			panic("mismatched key/val and tuple configs")
		}
		if cfg.TupleLen() > maxLen {
			maxLen = cfg.TupleLen()
		}
	}

	// Strip off the idx'th element until we've processed all of
	// every tuple. This is going to mutate the configs slice to
	// as we get the prefixes, so copy it if necessary.
	if cow {
		configs = append([]*Config(nil), configs...)
	}
	elems := make([]*Config, 0, len(configs))
	tuple := make([]*schema, maxLen)
	size := 0
	for idx := maxLen - 1; idx >= 0; idx-- {
		elems = elems[:0]
		for i, cfg := range configs {
			if idx >= cfg.TupleLen() {
				continue
			}

			prefix, elem := cfg.PrefixElem()
			configs[i] = prefix
			if len(elems) == 0 || elem != elems[len(elems)-1] {
				elems = append(elems, elem)
			}
		}
		// We're done with elems, so the recursive call can
		// mutate it.
		sub := commonSchema1(elems, false)
		tuple[idx] = sub
		size += sub.size
	}

	return &schema{tuple: tuple, size: size}
}

// keys returns the flattened key sequence in this schema.
func (s *schema) keys() []string {
	keys := make([]string, 0, s.size)
	var walk func(s *schema)
	walk = func(s *schema) {
		if s.tuple == nil {
			keys = append(keys, s.key)
		} else {
			for _, sub := range s.tuple {
				walk(sub)
			}
		}
	}
	walk(s)
	return keys
}

// flatten extracts all key/value leafs from config according to
// schema s. The returned Configs will all be either a key/value
// Config or nil for elements that are in the schema, but not in
// config.
//
// flatten uses buf as scratch space, expanding it if necessary. The
// buffer returned by flatten can be passed to subsequent calls to
// reduce allocation.
func (s *schema) flatten(config *Config, buf []*Config) []*Config {
	if cap(buf) >= s.size {
		buf = buf[:s.size]
	} else {
		buf = make([]*Config, s.size)
	}

	var flatten1 func(s *schema, config *Config, buf []*Config) []*Config
	flatten1 = func(s *schema, config *Config, buf []*Config) []*Config {
		if s.tuple == nil {
			buf[len(buf)-1] = config
			return buf[:len(buf)-1]
		}

		tuple := s.tuple
		for len(tuple) > config.TupleLen() {
			buf = flatten1(tuple[len(tuple)-1], nil, buf)
			tuple = tuple[:len(tuple)-1]
		}
		for len(tuple) > 0 {
			prefix, elem := config.PrefixElem()
			buf = flatten1(tuple[len(tuple)-1], elem, buf)
			tuple = tuple[:len(tuple)-1]
			config = prefix
		}
		return buf
	}
	final := flatten1(s, config, buf)
	if len(final) != 0 {
		panic(fmt.Sprintf("internal error: len(final) = %d, should be 0", len(final)))
	}
	return buf
}
