// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// XXX Move package doc

// Package benchproc implements processing pipelines for benchmark
// results.
//
// A processing pipeline is driven by a Pipeline object and described
// by a tree of Processors.
package benchproc

import "golang.org/x/perf/v2/benchfmt"

// A Pipeline is a tree of Processors for processing a stream of
// benchmark results.
type Pipeline struct {
	// Root is the entry-point of this Pipeline.
	root Processor

	// ConfigSet is the space of Configs used in this Pipeline.
	ConfigSet *ConfigSet

	// projCache caches evaluated Projections for the current
	// result.
	projCache map[Projection]projCache
	projCur   *benchfmt.Result
	projGen   uint64
}

type projCache struct {
	gen uint64
	val *Config
}

// NewPipeline returns an empty pipeline. The caller should construct
// a tree of Processors to use in this Pipeline, then call SetRoot to
// register the root of the tree, then call Process on each benchmark
// result.
func NewPipeline() *Pipeline {
	return &Pipeline{ConfigSet: new(ConfigSet)}
}

// SetRoot sets the root of the processing pipeline. This may only be
// called once on p.
//
// The root processor will always be called with the empty group key
// (nil).
func (p *Pipeline) SetRoot(root Processor) {
	if p.root != nil {
		panic("pipeline root already set")
	}
	p.root = root
}

// Process processes a single benchmark result.
func (p *Pipeline) Process(result *benchfmt.Result) {
	// Invalidate projection cache.
	p.projGen++
	p.projCur = result

	// Process the result, starting with the empty group tuple.
	p.root.Process(result, nil)
}

// Project returns the projection of result by proj. This adds caching
// on top of directly calling proj.Project, so that projections that
// are reused across a pipeline are only evaluated once per result.
func (p *Pipeline) Project(result *benchfmt.Result, proj Projection) *Config {
	if result != p.projCur {
		// We only cache for the current result.
		return proj.Project(p, result)
	}

	// Check the projection cache.
	cached, gen := p.projCache[proj], p.projGen
	if cached.gen == gen {
		return cached.val
	}

	// Compute the projection.
	val := proj.Project(p, result)
	p.projCache[proj] = projCache{gen, val}
	return val
}

// A Processor is a node in a benchmark result processing pipeline.
//
// A given Processor type may be an interior processor or a leaf
// processor. Interior processors typically accumulate on state of
// their own, but observe the result in some way and invoke other
// Processors to further process a result. Leaf processors typically
// gather results.
type Processor interface {
	// Process processes one result.
	//
	// The groupKey argument gives the current grouping key.
	// Grouping operations can extend groupKey to further
	// subdivide groups before calling other Processors. Leaf
	// operations should separate their results by groupKey.
	Process(result *benchfmt.Result, groupKey *Config)
}
