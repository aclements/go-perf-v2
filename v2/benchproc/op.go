// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import "golang.org/x/perf/v2/benchfmt"

// A Grouper is an Operator that produces a *Grouped, which groups
// results by distinct values of a Projection.
type Grouper struct {
	project Projection
	next    Operator
}

// A Grouped is a Processor that collects results grouped by distinct
// values of a Projection.
type Grouped struct {
	Groups map[*Config]Processor

	pipeline *Pipeline
	op       Grouper
}

var _ Operator = (*Grouper)(nil)

func NewGrouper(p Projection, next Operator) *Grouper {
	return &Grouper{p, next}
}

func (g *Grouper) Start(pipeline *Pipeline) Processor {
	return &Grouped{make(map[*Config]Processor), pipeline, *g}
}

func (g *Grouped) Process(result *benchfmt.Result) {
	key := g.pipeline.Project(result, g.op.project)
	sub, ok := g.Groups[key]
	if !ok {
		sub = g.op.next.Start(g.pipeline)
		g.Groups[key] = sub
	}
	sub.Process(result)
}

// A Tracker is an Operator that produces a *Tracked.
type Tracker struct {
	project Projection
}

// A Tracked is a Processor that collects the unique values of a
// Projection in order of first observation.
type Tracked struct {
	// Configs is the set of distinct Configs resulting from the
	// projection of all elements in Next, in order of first
	// observation.
	Configs []*Config

	// Order is the index of each Config in Configs.
	Order map[*Config]int

	pipeline *Pipeline
	op       Tracker
}

var _ Operator = (*Tracker)(nil)

func NewTracker(p Projection) *Tracker {
	return &Tracker{p}
}

func (t *Tracker) Start(pipeline *Pipeline) Processor {
	return &Tracked{Order: make(map[*Config]int), pipeline: pipeline, op: *t}
}

func (t *Tracked) Process(result *benchfmt.Result) {
	key := t.pipeline.Project(result, t.op.project)
	if _, ok := t.Order[key]; !ok {
		t.Order[key] = len(t.Configs)
		t.Configs = append(t.Configs, key)
	}
}

// A Tee is an Operator that produces a *Teed.
type Tee struct {
	subs []Operator
}

type Teed struct {
	Subs []Processor
}

var _ Operator = (*Tee)(nil)

func NewTee(subs []Operator) *Tee {
	return &Tee{subs}
}

func (t *Tee) Start(pipeline *Pipeline) Processor {
	subs := make([]Processor, len(t.subs))
	for i := range subs {
		subs[i] = t.subs[i].Start(pipeline)
	}
	return &Teed{subs}
}

func (t *Teed) Process(result *benchfmt.Result) {
	for _, sub := range t.Subs {
		sub.Process(result)
	}
}
