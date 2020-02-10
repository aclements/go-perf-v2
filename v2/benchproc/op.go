// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package benchproc

import "golang.org/x/perf/v2/benchfmt"

// A GroupBy is a Processor that sub-divides results by different
// values of a Projection.
type GroupBy struct {
	pipeline *Pipeline
	project  Projection
	next     Processor
}

var _ Processor = (*GroupBy)(nil)

func NewGroupBy(pipeline *Pipeline, project Projection, next Processor) *GroupBy {
	return &GroupBy{pipeline, project, next}
}

func (g *GroupBy) Process(result *benchfmt.Result, groupKey *Config) {
	groupKey2 := g.pipeline.ConfigSet.Append(groupKey, g.pipeline.Project(result, g.project))
	g.next.Process(result, groupKey2)
}

// XXX CollectConfigs? To parallel CollectValues.

// A Tracker is a leaf Processor that collects the unique values of a
// Projection in order of first observation.
type Tracker struct {
	Tracked map[*Config]*Tracked

	pipeline *Pipeline
	project  Projection
}

var _ Processor = (*Tracker)(nil)

// A Tracked stores the results of a Tracker for a single group.
type Tracked struct {
	// Configs is the set of distinct Configs resulting from the
	// Projection of all elements in this group, in order of first
	// observation.
	Configs []*Config

	// Order is the index of each Config in Configs.
	Order map[*Config]int
}

func NewTracker(pipeline *Pipeline, project Projection) *Tracker {
	return &Tracker{
		Tracked:  make(map[*Config]*Tracked),
		pipeline: pipeline,
		project:  project,
	}
}

func (t *Tracker) Process(result *benchfmt.Result, groupKey *Config) {
	tracked := t.Tracked[groupKey]
	if tracked == nil {
		tracked = &Tracked{Order: make(map[*Config]int)}
		t.Tracked[groupKey] = tracked
	}

	key := t.pipeline.Project(result, t.project)
	if _, ok := tracked.Order[key]; !ok {
		tracked.Order[key] = len(tracked.Configs)
		tracked.Configs = append(tracked.Configs, key)
	}
}

// A Tee is a Processor that passes results to one or more other
// Processors.
type Tee struct {
	subs []Processor
}

var _ Processor = (*Tee)(nil)

func NewTee(pipeline *Pipeline, subs ...Processor) *Tee {
	return &Tee{subs}
}

func (t *Tee) Process(result *benchfmt.Result, groupKey *Config) {
	for _, sub := range t.subs {
		sub.Process(result, groupKey)
	}
}
