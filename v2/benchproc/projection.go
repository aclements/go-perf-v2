// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import "golang.org/x/perf/v2/benchfmt"

// A Projection extracts some aspect of a benchmark result into a
// Config.
type Projection interface {
	Project(*Pipeline, *benchfmt.Result) *Config
}

// A ProjectionProduct combines the results of one or more other
// projections into a tuple.
type ProjectionProduct struct {
	projs []Projection
}

func NewProjectionProduct(projs []Projection) *ProjectionProduct {
	return &ProjectionProduct{projs}
}

func (p *ProjectionProduct) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	// Invoke each child projection.
	subs := make([]*Config, 0, 16)
	for _, proj := range p.projs {
		subs = append(subs, pipeline.Project(res, proj))
	}
	return pipeline.ConfigSet.Tuple(subs...)
}

// TODO

// func NewProjectionFileKey(key string) XXX {}
// func NewProjectionFileConfig() XXX        {}
// func NewProjectionBaseName() XXX          {}
// func NewProjectionFullName() XXX          {}
// func NewProjectionNameKey(key string) XXX {}
