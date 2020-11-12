// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"golang.org/x/perf/v2/benchproc"
)

type ConfigGraph struct {
	edges map[benchproc.Config]map[benchproc.Config]struct{}
	nodes []benchproc.Config
}

func (g *ConfigGraph) Add(a, b benchproc.Config) {
	if g.edges == nil {
		g.edges = make(map[benchproc.Config]map[benchproc.Config]struct{})
	}
	addNode := func(c benchproc.Config) {
		if !c.IsZero() && g.edges[c] == nil {
			g.edges[c] = make(map[benchproc.Config]struct{})
			g.nodes = append(g.nodes, c)
		}
	}
	addNode(a)
	addNode(b)
	if !a.IsZero() && !b.IsZero() {
		g.edges[a][b] = struct{}{}
		g.edges[b][a] = struct{}{}
	}
}

func (g *ConfigGraph) Color(max int) map[benchproc.Config]int {
	// This is a greedy coloring algorithm, but with a twist: we
	// try to use as many colors as we can by rotating the initial
	// color selection as we visit nodes.
	type colorSet uint32
	if max >= 32 {
		panic("color count exceeds uint32 mask")
	}
	coloring := make(map[benchproc.Config]int)

	// Visit nodes.
nextNode:
	for i, node := range g.nodes {
		// Gather adjacent colors.
		var exclude colorSet
		for dst := range g.edges[node] {
			if c, ok := coloring[dst]; ok {
				exclude |= 1 << c
			}
		}
		// Pick a color that doesn't conflict with neighbors,
		// starting with an index-based color to cycle through
		// the palette.
		c := uint32(i)
		for off := uint32(0); off < uint32(max); off++ {
			if exclude&(1<<((c+off)%uint32(max))) == 0 {
				coloring[node] = int((c + off) % uint32(max))
				continue nextNode
			}
		}
		// Failed. Just use c.
		coloring[node] = int(c % uint32(max))
	}

	return coloring
}
