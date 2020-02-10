// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchstat

import (
	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc"
)

// XXX I want to sort by the config leafs, even for dynamic keys like
// the file config. This means I can't statically determine the set of
// sorters up-front from the projection tuples (though I also don't
// have to take a product projection).

// XXX How do I sort dynamic tuples? For the static part, clearly I
// want to sort in the order given by the user, but there the keys
// will also line up. For the dynamic part, the keys won't necessarily
// line up when I'm comparing tuples. For dynamic tuples, maybe I
// actually want some sort of map compare, with an order first imposed
// on the keys? Does it help if the file config projection is stateful
// and only ever appends, so configs that line up always have the same
// key (though keep the state in the config, not the Reader)?

// XXX For user sorts, it makes sense to specify them as their own
// argument because they could be buried in a dynamic projection, and
// it would be annoying to not be able to use the dynamic projection
// just because you want to sort a particular key.

type Collection struct {
	cs        *benchproc.ConfigSet
	projector *benchproc.Projector

	groupBy, rowBy, colBy *benchproc.ProjectProduct

	// groups maps from (groupBy, unit) to group.
	groups map[groupKey]*group

	// order records the global observation order of each element
	// in the group, row, and col projections. We track the order
	// of each key individually, rather than the whole projection
	// because,
	//
	// 1. it's nicer for the user to see keys always presented in
	// the same order (especially when dealing with just a bag of
	// key/value pairs, like the file-level config), and
	//
	// 2. this lets users override the sort order on individual
	// keys (e.g., sort numerically).
	order map[benchproc.Projection]*benchproc.ConfigTracker

	// unitTracker records the observation order of units.
	unitTracker benchproc.ConfigTracker
}

type groupKey struct {
	group *benchproc.Config // group tuple
	unit  *benchproc.Config // (".unit", unit) key/value
}

type group struct {
	// Observed subsets in this group for the elements of the row
	// and column projections. Within the group, we show only the
	// row and column labels for the data in the group, but we
	// sort them according to the global observation order for
	// consistency across groups.
	subConfigs map[benchproc.Projection]*benchproc.ConfigTracker

	// cells maps from (row, col) to each cell.
	cells map[cellKey]*cell
}

type cellKey struct {
	rowKey *benchproc.Config // row tuple
	colKey *benchproc.Config // col tuple
}

type cell struct {
	values []float64
}

// XXX

// // projectUnit is a dummy projection that allows units to be tracked
// // along with other dimensions.
// type projectUnit struct{}

// func (p *projectUnit) Project() *benchproc.Config {
// 	panic("dummy unit projection called")
// }

func NewCollection(groupBy, rowBy, colBy *benchproc.ProjectProduct) *Collection {
	cs := new(benchproc.ConfigSet)

	groups := make(map[groupKey]*group)

	// Initialize the order trackers.
	order := make(map[benchproc.Projection]*benchproc.ConfigTracker)
	for _, prod := range []*benchproc.ProjectProduct{groupBy, rowBy, colBy} {
		for _, sub := range prod.Elts {
			order[sub] = new(benchproc.ConfigTracker)
		}
	}

	return &Collection{
		cs:        cs,
		projector: benchproc.NewProjector(cs),
		groupBy:   groupBy, rowBy: rowBy, colBy: colBy,
		groups: groups,
		order:  order,
	}
}

func (c *Collection) Add(result *benchfmt.Result) {
	projector := c.projector
	projector.Reset(result)

	// Add to the order trackers.
	for proj, tracker := range c.order {
		tracker.Add(projector.Project(proj))
	}

	// Map to a group.
	gkey1 := projector.Project(c.groupBy)
	for _, val := range result.Values {
		unitKey := c.cs.KeyValue(".unit", val.Unit)
		c.unitTracker.Add(unitKey)
		gkey := groupKey{gkey1, unitKey}
		group := c.groups[gkey]
		if group == nil {
			group = c.newGroup()
			c.groups[gkey] = group
		}

		// Add to the row and col sets of this group.
		for proj, tracker := range group.subConfigs {
			tracker.Add(projector.Project(proj))
		}

		// Map to a cell.
		ckey := cellKey{projector.Project(c.rowBy), projector.Project(c.colBy)}
		ccell := group.cells[ckey]
		if ccell == nil {
			ccell = new(cell)
			group.cells[ckey] = ccell
		}

		// Add to this cell.
		ccell.values = append(ccell.values, val.Value)
	}
}

func (c *Collection) newGroup() *group {
	g := &group{
		subConfigs: make(map[benchproc.Projection]*benchproc.ConfigTracker),
		cells:      make(map[cellKey]*cell),
	}
	// Populate the subConfigs map from the row and col
	// projections.
	for _, sub := range c.rowBy.Elts {
		g.subConfigs[sub] = new(benchproc.ConfigTracker)
	}
	for _, sub := range c.colBy.Elts {
		g.subConfigs[sub] = new(benchproc.ConfigTracker)
	}
	return g
}
