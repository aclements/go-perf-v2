// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package benchproc

import "golang.org/x/perf/v2/benchfmt"

// XXX There's a bad cycle here since I need to provide the
// CollectValues to the GroupByUnit and the GroupByUnit to the
// CollectValues. GroupByUnit either needs to be given a target it can
// push the current unit to, or I need a mutator on CollectValues to
// link it up after the fact.

type GroupByUnit struct {
	CurrentUnit string

	pipeline *Pipeline
	next     Processor
}

var _ Processor = (*GroupByUnit)(nil)

func NewGroupByUnit(pipeline *Pipeline, next Processor) *GroupByUnit {
	return &GroupByUnit{"", pipeline, next}
}

func (g *GroupByUnit) Process(result *benchfmt.Result, groupKey *Config) {
	cs := g.pipeline.ConfigSet
	for _, val := range result.Values {
		g.CurrentUnit = val.Unit
		groupKey2 := cs.Append(groupKey, cs.KeyValue(".unit", val.Unit))
		g.next.Process(result, groupKey2)
	}
	g.CurrentUnit = ""
}

type CollectValues struct {
	unit *GroupByUnit

	Values map[*Config][]float64
}

var _ Processor = (*CollectValues)(nil)

func NewCollectValues(pipeline *Pipeline) *CollectValues {
	return &CollectValues{nil, make(map[*Config][]float64)}
}

func (c *CollectValues) BindUnit(g *GroupByUnit) {
	c.unit = g
}

func (c *CollectValues) Process(result *benchfmt.Result, groupKey *Config) {
	unit := c.unit.CurrentUnit
	for _, val := range result.Values {
		if val.Unit == unit {
			c.Values[groupKey] = append(c.Values[groupKey], val.Value)
			return
		}
	}
}
