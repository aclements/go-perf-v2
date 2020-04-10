// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "golang.org/x/perf/v2/benchproc"

// globalOrder takes a list of locally ordered config sequences from
// lowest to highest priority and returns a global order that combines
// the local orders.
func globalOrder(local [][]*benchproc.Config) []*benchproc.Config {
	// Make a graph that combines the orders.
	type node struct {
		succs   []*benchproc.Config // Successors in priority order
		set     map[*benchproc.Config]struct{}
		visited bool
	}
	nodes := make(map[*benchproc.Config]*node)
	for i := len(local) - 1; i >= 0; i-- {
		cfgs := local[i]
		var succ *benchproc.Config
		for i := len(cfgs) - 1; i >= 0; i-- {
			cfg := cfgs[i]

			// Create node for config.
			cfgNode := nodes[cfg]
			if cfgNode == nil {
				cfgNode = &node{set: make(map[*benchproc.Config]struct{})}
				nodes[cfg] = cfgNode
			}
			if succ != nil {
				// Add a cfg -> succ edge.
				if _, ok := cfgNode.set[succ]; !ok {
					cfgNode.succs = append(cfgNode.succs, succ)
					cfgNode.set[succ] = struct{}{}
				}
			}

			succ = cfg
		}
	}

	// Topologically sort the graph, using the first configuration
	// in each sequence as a root and biasing by edge priority.
	var order []*benchproc.Config
	var dfs func(cfg *benchproc.Config)
	dfs = func(cfg *benchproc.Config) {
		node := nodes[cfg]
		if node.visited {
			return
		}
		node.visited = true
		for _, succ := range node.succs {
			dfs(succ)
		}
		order = append(order, cfg)
	}
	for i := len(local) - 1; i >= 0; i-- {
		if len(local[i]) == 0 {
			continue
		}
		root := local[i][0]
		dfs(root)
	}
	// Order is backwards. Fix it.
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}
