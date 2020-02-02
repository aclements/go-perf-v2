// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchproc implements processing pipelines for benchmark
// results.
//
// A processing pipeline is driven by a Pipeline object and described
// by a tree of Operators. An Operator itself is an abstract operation
// that gets instantiated into one or more Processors that are bound
// to a particular pipeline.
package benchproc

import "golang.org/x/perf/v2/benchfmt"

// A Pipeline is a collection of Projections and Operators for
// processing a stream of benchmark results.
type Pipeline struct {
	// Root is the entry-point of this Pipeline.
	Root Processor

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

func NewPipeline(rootOp Operator) *Pipeline {
	pipeline := &Pipeline{ConfigSet: new(ConfigSet)}
	pipeline.Root = rootOp.Start(pipeline)
	return pipeline
}

func (p *Pipeline) Process(result *benchfmt.Result) {
	// Invalidate projection cache.
	p.projGen++
	p.projCur = result

	// Process the result.
	p.Root.Process(result)
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

// An Operator describes an operation on benchmark results. Many
// Operators preform some observation of the result and pass it on to
// another Operator.
type Operator interface {
	// Start returns a new Processor bound to a Pipeline with a
	// fresh state that will apply the operation described by this
	// Operator. An Operator may be instantiated multiple times on
	// the same Pipeline, for example for if results are being
	// demultiplexed into different groups.
	Start(*Pipeline) Processor
}

// A Processor processes a stream of benchmark results.
type Processor interface {
	// Process processes one result.
	Process(result *benchfmt.Result)
}
