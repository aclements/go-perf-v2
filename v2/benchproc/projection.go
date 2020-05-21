// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"golang.org/x/perf/v2/benchfmt"
)

// A Projection extracts some aspect of a benchmark result into a
// Config.
//
// Projections can either have a static schema or a dynamic schema.
// Dynamic projections are necessarily stateful in order to produce
// Configs that are schema compatible.
//
// Projections that may overlap with other more specific projections
// support "excludes" that allow the keys captured by more specific
// projections to be excluded from the output of the broader
// projection.
type Projection interface {
	Project(*ConfigSet, *benchfmt.Result) *Config

	// AppendStaticKeys appends the static keys produced by this
	// projection to keys.
	AppendStaticKeys(keys []string) []string
}

// A ProjectProduct combines the results of one or more other
// projections into a tuple.
type ProjectProduct []Projection

func (p *ProjectProduct) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	// Invoke each child projection.
	subs := make([]*Config, 0, 16)
	for _, proj := range *p {
		subs = append(subs, proj.Project(cs, r))
	}
	return cs.Tuple(subs...)
}

func (p *ProjectProduct) AppendStaticKeys(keys []string) []string {
	for _, proj := range *p {
		keys = proj.AppendStaticKeys(keys)
	}
	return keys
}

// A projectExtractor projects a simple key/value pair from a result
// using a benchfmt.Extractor.
type projectExtractor struct {
	key string
	ext benchfmt.Extractor
}

// NewProjectKey returns a Projection for the given extractor key. See
// benchfmt.NewExtractor for supported keys.
func NewProjectKey(key string) (Projection, error) {
	ext, err := benchfmt.NewExtractor(key)
	if err != nil {
		return nil, err
	}
	return &projectExtractor{key, ext}, nil
}

// NewProjectFullName returns a new projection for the full name of a
// benchmark, but with any name keys in exclude normalized in the
// projected Config.
func NewProjectFullName(exclude []string) (Projection, error) {
	ext := benchfmt.NewExtractorFullName(exclude)
	return &projectExtractor{".full", ext}, nil
}

func (p *projectExtractor) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	return cs.KeyVal(p.key, p.ext(r))
}

func (p *projectExtractor) AppendStaticKeys(keys []string) []string {
	return append(keys, p.key)
}

// ProjectFileConfig projects the full file configuration as a tuple
// Config. The zero value of ProjectFileConfig is a valid projection
// that excludes no keys.
//
// This projection is stateful because its schema is dynamic.
type ProjectFileConfig struct {
	order   []string
	known   map[string]bool
	exclude map[string]bool
}

// NewProjectFileConfig returns a new projection for the file
// configuration, but with any keys in exclude normalized in the
// projected Config
func NewProjectFileConfig(exclude []string) *ProjectFileConfig {
	if len(exclude) == 0 {
		return new(ProjectFileConfig)
	}

	excludeMap := make(map[string]bool)
	for _, k := range exclude {
		excludeMap[k] = true
	}
	return &ProjectFileConfig{exclude: excludeMap}
}

func (p *ProjectFileConfig) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	var config *Config
	pendingUnset := make([]string, 0, 16)
	addElem := func(fCfg *benchfmt.Config) {
		if fCfg.Value == "" {
			// Unset value. We defer adding these to the
			// tuple until we set a set value and discard
			// unset values at the end of the tuple. This
			// way, tuples after a key is deleted appear
			// the same as tuples before that key existed.
			pendingUnset = append(pendingUnset, fCfg.Key)
			return
		}

		// Flush any pending unset values to fill in the
		// schema.
		for _, k := range pendingUnset {
			val := ""
			if p.exclude[k] {
				// Excluded values always appear
				// normalized, even if they've been
				// deleted, because we want this to be
				// equivalent to the key being removed
				// from tuples.
				val = "*"
			}
			config = cs.Append(config, cs.KeyVal(k, val))
		}
		pendingUnset = pendingUnset[:0]

		val := fCfg.Value
		if p.exclude[fCfg.Key] {
			val = "*"
		}
		config = cs.Append(config, cs.KeyVal(fCfg.Key, val))
	}

	// Collect known keys.
	found := 0
	for _, k := range p.order {
		if pos, ok := r.FileConfigIndex(k); !ok {
			addElem(&benchfmt.Config{k, ""})
		} else {
			addElem(&r.FileConfig[pos])
			found++
		}
	}

	// If we didn't get all of the file config, find the keys we
	// missed.
	if found != len(r.FileConfig) {
		if p.known == nil {
			p.known = make(map[string]bool)
		}
		for i := range r.FileConfig {
			fCfg := &r.FileConfig[i]
			if _, ok := p.known[fCfg.Key]; ok {
				continue
			}
			p.order = append(p.order, fCfg.Key)
			p.known[fCfg.Key] = true
			addElem(fCfg)
		}
	}

	return config
}

func (p *ProjectFileConfig) AppendStaticKeys(keys []string) []string {
	return keys
}
