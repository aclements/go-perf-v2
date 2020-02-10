// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
type ProjectProduct struct {
	Elts []Projection
}

func (p *ProjectProduct) Project(pr *Projector) *Config {
	// Invoke each child projection.
	subs := make([]*Config, 0, 16)
	for _, proj := range p.Elts {
		subs = append(subs, pr.Project(proj))
	}
	return pr.ConfigSet().Tuple(subs...)
}

/*

type ProjectFileKey struct {
	key     string
	pos     int
	lastLen int
}

// XXX
//
// Note that this projection is stateful, so it should not be used in
// more than one Pipeline.
func NewProjectFileKey(key string) *ProjectFileKey {
	return &ProjectFileKey{key, -1, 0}
}

// XXX This forces me to also use the same benchfmt.Reader for the
// whole thing, versus being able to feed results from multiple Reader
// instances into one pipeline. Using the fixed file indexes may just
// be a bad idea and perhaps I should just use a map.
//
// XXX Though if I don't do this, ProjectFileConfig becomes stateful
// and more complicated, but maybe that's not so bad.

func (p *ProjectFileKey) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	cs := pipeline.ConfigSet

	// Find the index of the key.
	if p.pos == -1 {
		if len(res.FileConfig) != p.lastLen {
			p.lastLen = len(res.FileConfig)
			if pos, ok := res.FileConfigIndex(p.key); ok {
				p.pos = pos
			}
		}
		if p.pos == -1 {
			return cs.KeyValue(p.key, "")
		}
	}

	return cs.KeyValue(p.key, res.FileConfig[p.pos].Value)
}

type ProjectFileConfig struct{}

func (p *ProjectFileConfig) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	// TODO: What if some file key was specified in another
	// projection? I want to include all but that key. Yuck. I
	// suppose the same could apply to name keys, which is even
	// messier.

	// Build a tuple of present keys. Even though we drop keys
	// when deleted, because the order of keys in FileConfig never
	// changes once they're added, the tuple will be stable even
	// if a key is "deleted" and re-added.
	cs := pipeline.ConfigSet
	cfg := make([]*Config, 0, 16)
	for _, fcfg := range res.FileConfig {
		if fcfg.Value == "" {
			continue
		}
		cfg = append(cfg, cs.KeyValue(fcfg.Key, fcfg.Value))
	}
	return cs.Tuple(cfg...)
}

type ProjectFullName struct{}

func (p *ProjectFullName) Project(pipeline *Pipeline, res *benchfmt.Result) *Config {
	cs := pipeline.ConfigSet
	return cs.KeyValue(".name", cs.Bytes(res.FullName))
}

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
