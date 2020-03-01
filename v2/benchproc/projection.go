// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// XXX The Projector is not turning out to be useful, and is really
// annoying. Move back to uncached projections.

package benchproc

import (
	"golang.org/x/perf/v2/benchfmt"
)

// A Projection extracts some aspect of a benchmark result into a
// Config.
type Projection interface {
	Project(*Projector) *Config
}

// A Projector applies Projections to a benchmark result. This stores
// the state necessary to apply a Projection and caches their results.
type Projector struct {
	configSet *ConfigSet
	result    *benchfmt.Result
	cache     map[Projection]projCache
	gen       uint64
}

type projCache struct {
	gen uint64
	val *Config
}

func NewProjector(cs *ConfigSet) *Projector {
	return &Projector{cs, nil, make(map[Projection]projCache), 0}
}

func (p *Projector) Reset(res *benchfmt.Result) {
	p.gen++
	p.result = res
}

func (p *Projector) Project(projection Projection) *Config {
	if cached, ok := p.cache[projection]; ok && cached.gen == p.gen {
		return cached.val
	}
	val := projection.Project(p)
	p.cache[projection] = projCache{p.gen, val}
	return val
}

func (p *Projector) ConfigSet() *ConfigSet {
	return p.configSet
}

func (p *Projector) Cur() *benchfmt.Result {
	return p.result
}

// A ProjectProduct combines the results of one or more other
// projections into a tuple.
type ProjectProduct []Projection

func (p *ProjectProduct) Project(pr *Projector) *Config {
	// Invoke each child projection.
	subs := make([]*Config, 0, 16)
	for _, proj := range *p {
		subs = append(subs, pr.Project(proj))
	}
	return pr.ConfigSet().Tuple(subs...)
}

// ProjectFileKey projects a file configuration key.
type ProjectFileKey struct {
	Key string
}

// Project returns a key/value Config with the key p.Key and a value
// of the file configuration key p.Key, or "" if the key is not
// present.
func (p *ProjectFileKey) Project(pr *Projector) *Config {
	res := pr.Cur()
	pos, ok := res.FileConfigIndex(p.Key)
	val := ""
	if ok {
		val = res.FileConfig[pos].Value
	}
	return pr.ConfigSet().KeyVal(p.Key, val)
}

// TODO: If any name keys are extracted, perhaps ProjectFullName needs
// to be able to exclude them. It could rewrite the name with a *
// there or something.

// ProjectFullName projects the full name of a benchmark.
type ProjectFullName struct{}

// Project returns a key/value Config with the key ".name" and a value
// of the full name of the benchmark.
func (p *ProjectFullName) Project(pr *Projector) *Config {
	cs := pr.ConfigSet()
	return cs.KeyVal(".name", cs.Bytes(pr.Cur().FullName))
}

// ProjectFileConfig projects the full file configuration as a tuple
// Config.
//
// This projection is stateful because it produces a dynamic tuple.
type ProjectFileConfig struct {
	order map[string]int
}

func (p *ProjectFileConfig) Project(pr *Projector) *Config {
	// TODO: Collect keys in consistent order. If the file config
	// was a map, I could just iterate over the order and collect
	// keys and check if I got all of them and, if not, iterate
	// over the file config map for missed keys. As a slice, I
	// could have a fast path if the order is the same and a slow
	// path if not; its easy to find the new keys, but I'm stuck
	// in the slow path if the order changes (unless I cache
	// position hints?). Trim empty keys from the end so tuples
	// after a key is deleted appear the same as tuples before
	// that key existed.

	// TODO: What if some file key was specified in another
	// projection? I want to include all but that key. Yuck. I
	// suppose the same could apply to benchmark name keys, which
	// is even messier.

	panic("not implemented")
}

/*

type ProjectBaseName struct{}

func (p *ProjectBaseName) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	cs := pipeline.ConfigSet
	baseName, _ := res.NameParts()
	return cs.KeyValue(".base", cs.Bytes(baseName))
}

type ProjectNameKey struct {
	key        string
	prefix     []byte
	gomaxprocs bool
}

func NewProjectNameKey(key string) *ProjectNameKey {
	if !strings.HasPrefix(key, "/") {
		panic("name key must being with /")
	}

	// Construct the byte prefix to search for.
	prefix := make([]byte, len(key)+1)
	copy(prefix, key)
	prefix[len(prefix)-1] = '='
	return &ProjectNameKey{key, prefix, key == "/gomaxprocs"}
}

func (p *ProjectNameKey) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	cs := pipeline.ConfigSet
	_, parts := res.NameParts()
	if p.gomaxprocs && len(parts) > 0 {
		last := parts[len(parts)-1]
		if last[0] == '-' {
			// GOMAXPROCS specified as "-N" suffix.
			return cs.KeyValue(p.key, cs.Bytes(last[1:]))
		}
	}
	// Search for the prefix.
	for _, part := range parts {
		if bytes.HasPrefix(part, p.prefix) {
			return cs.KeyValue(p.key, cs.Bytes(part[len(p.prefix):]))
		}
	}
	// Not found.
	return cs.KeyValue(p.key, "")
}
*/
