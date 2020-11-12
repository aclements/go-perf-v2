// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package benchstat

import (
	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc"
)

// XXX How do I sort dynamic tuples? For the static part, clearly I
// want to sort in the order given by the user, but there the keys
// will also line up. For the dynamic part, the keys won't necessarily
// line up when I'm comparing tuples. For dynamic tuples, maybe I
// actually want some sort of map compare, with an order first imposed
// on the keys? Does it help if the file config projection is stateful
// and only ever appends, so configs that line up always have the same
// key (though keep the state in the config, not the Reader)?
//
// File config could keep the key observation order. Deleted keys
// would have to be key in the config, and any deleted keys at the end
// need to be trimmed.

// XXX For user sorts, it makes sense to specify them as their own
// argument because they could be buried in a dynamic projection, and
// it would be annoying to not be able to use the dynamic projection
// just because you want to sort a particular key.

type Collection struct {
	cs *benchproc.ConfigSet

	groupBy, rowBy, colBy benchproc.Projection

	// groups maps from (groupBy, unit) to group.
	groups map[*benchproc.Config]*group

	// order records the global observation order of each key in
	// the group, row, and col configs. We track the order of each
	// key individually, rather than the whole projection because,
	//
	// 1. it's nicer for the user to see keys always presented in
	// the same order (especially when dealing with just a bag of
	// key/value pairs, like the file-level config), and
	//
	// 2. this lets users override the sort order on individual
	// keys (e.g., sort numerically).
	order map[string]*benchproc.ConfigTracker

	// orderHave is a set of *Configs that have been added to
	// order.
	orderHave map[*benchproc.Config]struct{}
}

type group struct {
	// Observed row and col configs within this group. Within the
	// group, we show only the row and col labels for the data in
	// the group, but we sort them according to the global
	// observation order for consistency across groups.
	rows map[*benchproc.Config]struct{}
	cols map[*benchproc.Config]struct{}

	// cells maps from (row, col) to each cell.
	cells map[cellKey]*cell
}

type cellKey struct {
	row *benchproc.Config // rowBy tuple
	col *benchproc.Config // colBy tuple
}

type cell struct {
	values []float64
}

func NewCollection(groupBy, rowBy, colBy benchproc.Projection) *Collection {
	// TODO: Custom key sorts
	cs := new(benchproc.ConfigSet)
	return &Collection{
		cs:      cs,
		groupBy: groupBy, rowBy: rowBy, colBy: colBy,
		groups:    make(map[*benchproc.Config]*group),
		order:     make(map[string]*benchproc.ConfigTracker),
		orderHave: make(map[*benchproc.Config]struct{}),
	}
}

func (c *Collection) Add(result *benchfmt.Result) {
	groupCfg1 := c.groupBy.Project(c.cs, result)
	cellCfg := cellKey{
		c.rowBy.Project(c.cs, result),
		c.colBy.Project(c.cs, result),
	}

	// Update the order trackers.
	c.addOrder(groupCfg1)
	c.addOrder(cellCfg.row)
	c.addOrder(cellCfg.col)

	// Map to a group.
	for _, val := range result.Values {
		unitCfg := c.cs.KeyVal(".unit", val.Unit)
		c.addOrder(unitCfg)
		groupCfg := c.cs.Tuple(groupCfg1, unitCfg)
		group := c.groups[groupCfg]
		if group == nil {
			group = c.newGroup()
			c.groups[groupCfg] = group
		}

		// XXX Grouping results and then putting them into
		// cells to make distributions is a common operation.
		// Can I export more of this mechanism?

		// Map to a cell.
		ccell := group.cells[cellCfg]
		if ccell == nil {
			ccell = new(cell)
			group.cells[cellCfg] = ccell

			// Add to the row and col sets of this group.
			group.rows[cellCfg.row] = struct{}{}
			group.cols[cellCfg.col] = struct{}{}
		}

		// Add to this cell.
		ccell.values = append(ccell.values, val.Value)
	}
}

func (c *Collection) addOrder(cfg *benchproc.Config) {
	if _, ok := c.orderHave[cfg]; ok {
		return
	}
	c.orderHave[cfg] = struct{}{}

	if cfg.IsKeyVal() {
		key, _ := cfg.KeyVal()
		tracker := c.order[key]
		if tracker == nil {
			tracker = new(benchproc.ConfigTracker)
			c.order[key] = tracker
		}
		tracker.Add(cfg)
		return
	}

	// Walk into the tuple.
	prefix, elem := cfg.PrefixLast()
	c.addOrder(prefix)
	c.addOrder(elem)
}

func (c *Collection) newGroup() *group {
	return &group{
		rows:  make(map[*benchproc.Config]struct{}),
		cols:  make(map[*benchproc.Config]struct{}),
		cells: make(map[cellKey]*cell),
	}
}

/*
func (c *Collection) ToTables() []*Table {
	// Create a tuple sorter driven by observation order of each key.
	valCmp := func(a, b *benchproc.Config) int {
		key1, _ := a.KeyVal()
		key2, _ := b.KeyVal()
		if key1 != key2 {
			panic(fmt.Sprintf("cannot compare configs: key %q != key %q", key1, key2))
		}

		order := c.order[key1]
		return order.Order[b] - order.Order[a]
	}
	keys := func(m map[*benchproc.Config]struct{}) []*benchproc.Config {
		cfgs := make([]*benchproc.Config, 0, len(m))
		for k := range m {
			cfgs = append(cfgs, k)
		}
		return cfgs
	}

	// Sort the groups.
	groupCfgs := make([]*benchproc.Config, 0, len(c.groups))
	for k := range c.groups {
		groupCfgs = append(groupCfgs, k)
	}
	sort.Slice(groupCfgs, func(i, j int) bool {
		return benchproc.ConfigCmp(groupCfgs[i], groupCfgs[j], valCmp) < 0
	})

	// Create a table for each group.
	var tables []*Table
	for _, groupCfg := range groupCfgs {
		group := c.groups[groupCfg]

		// Sort rows and cols.
		rowCfgs := keys(group.rows)
		colCfgs := keys(group.cols)

		for _, row := range rowCfgs {
			for _, col := range colCfgs {

			}
		}
	}
}
*/
