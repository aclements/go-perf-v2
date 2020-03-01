// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"fmt"
	"strings"
	"testing"
)

func TestConfigTreeMerging(t *testing.T) {
	cs := new(ConfigSet)

	// Test basic merging.
	t.Run("basic", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"), cs.KeyVal("b", "b1"))
		c2 := cs.Tuple(cs.KeyVal("a", "a1"), cs.KeyVal("b", "b2"))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (b1) (b2))")
	})

	// Test merging at a tuple boundary.
	t.Run("tupleBoundary", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b1")))
		c2 := cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b2")))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (b1) (b2))")
	})

	// Test merging mid-tuple.
	t.Run("midTuple", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b1"), cs.KeyVal("c", "c1")))
		c2 := cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b1"), cs.KeyVal("c", "c2")))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (b1 (c1) (c2)))")
	})

	// Test that higher level differences prevent lower levels
	// from being merged, even if the lower levels match.
	t.Run("noMerge", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"), cs.KeyVal("b", "b1"))
		c2 := cs.Tuple(cs.KeyVal("a", "a2"), cs.KeyVal("b", "b1"))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (b1)) (a2 (b1))")
	})

	// Test the same thing, but with equal tuples at lower levels.
	t.Run("noMergeTuple", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b1")))
		c2 := cs.Tuple(cs.KeyVal("a", "a2"), cs.Tuple(cs.KeyVal("b", "b1")))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (b1)) (a2 (b1))")
	})

	// Test mismatched tuple lengths.
	t.Run("missingValues", func(t *testing.T) {
		c1 := cs.Tuple(cs.KeyVal("a", "a1"))
		c2 := cs.Tuple(cs.KeyVal("a", "a2"), cs.KeyVal("b", "b1"))
		c3 := cs.Tuple(cs.KeyVal("a", "a2"), cs.KeyVal("b", "b1"), cs.KeyVal("c", "c1"))
		tree, _ := NewConfigTree([]*Config{c1, c2, c3})
		checkTree(t, tree, "(a1 (nil (nil))) (a2 (b1 (nil) (c1)))")
	})

	// Test mismatched tuples lengths with whole missing
	// sub-tuples.
	t.Run("missingTuples", func(t *testing.T) {
		c1 := cs.Tuple(cs.Tuple(cs.KeyVal("a", "a1")), cs.KeyVal("d", "d1"))
		c2 := cs.Tuple(cs.Tuple(cs.KeyVal("a", "a1"), cs.Tuple(cs.KeyVal("b", "b1"), cs.KeyVal("c", "c1"))), cs.KeyVal("d", "d1"))
		tree, _ := NewConfigTree([]*Config{c1, c2})
		checkTree(t, tree, "(a1 (nil (nil (d1))) (b1 (c1 (d1))))")
	})
}

func checkTree(t *testing.T, tree []*ConfigTree, want string) {
	t.Helper()
	got := renderTree(tree)
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}

	// Check width consistency.
	var walk func(node *ConfigTree) int
	walk = func(node *ConfigTree) int {
		if len(node.Children) == 0 {
			if node.Width != 1 {
				t.Errorf("leaf node %s: want width 1, got %d", node.Config, node.Width)
			}
			return 1
		}
		w := 0
		for _, child := range node.Children {
			w += walk(child)
		}
		if node.Width != w {
			t.Errorf("node %s: want width %d, got %d", node.Config, w, node.Width)
		}
		return w
	}
	for _, root := range tree {
		walk(root)
	}
}

func renderTree(tree []*ConfigTree) string {
	buf := new(strings.Builder)
	var walk func(node *ConfigTree)
	walk = func(node *ConfigTree) {
		if node.Config == nil {
			buf.WriteString("(nil")
		} else {
			fmt.Fprintf(buf, "(%s", node.Config.Value())
		}
		for _, child := range node.Children {
			buf.WriteByte(' ')
			walk(child)
		}
		buf.WriteByte(')')
	}
	for i, root := range tree {
		if i > 0 {
			buf.WriteByte(' ')
		}
		walk(root)
	}
	return buf.String()
}
