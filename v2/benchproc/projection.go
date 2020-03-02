// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"bytes"
	"strings"
	"sync"

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

// ProjectFileKey projects a file configuration key.
type ProjectFileKey struct {
	Key string
}

// Project returns a key/value Config with the key p.Key and a value
// of the file configuration key p.Key, or "" if the key is not
// present.
func (p *ProjectFileKey) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	pos, ok := r.FileConfigIndex(p.Key)
	val := ""
	if ok {
		val = r.FileConfig[pos].Value
	}
	return cs.KeyVal(p.Key, val)
}

func (p *ProjectFileKey) AppendStaticKeys(keys []string) []string {
	return append(keys, p.Key)
}

// ProjectFullName projects the full name of a benchmark. The zero
// value of ProjectFullName is a valid projection that excludes no
// keys.
type ProjectFullName struct {
	replace       map[string][]byte
	excGomaxprocs bool
}

// NewProjectFullName returns a new projection for the full name of a
// benchmark, but with any name keys in exclude normalized in the
// projected Config.
func NewProjectFullName(exclude []string) *ProjectFullName {
	// Extract the name keys, turn them into substrings and
	// construct their normalized replacement.
	var replace map[string][]byte
	excGomaxprocs := false
	for _, k := range exclude {
		if !strings.HasPrefix(k, "/") {
			continue
		}
		if replace == nil {
			replace = make(map[string][]byte)
		}
		replace[k+"="] = append([]byte(k), '=', '*')
		if k == "/gomaxprocs" {
			excGomaxprocs = true
		}
	}
	return &ProjectFullName{replace, excGomaxprocs}
}

// Project returns a key/value Config with the key ".name" and a value
// of the full name of the benchmark.
func (p *ProjectFullName) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	name := cs.Bytes(r.FullName)
	found := false
	if len(p.replace) != 0 {
		for k := range p.replace {
			if strings.Contains(name, k) {
				found = true
				break
			}
		}
		if p.excGomaxprocs && strings.IndexByte(name, '-') >= 0 {
			found = true
		}
	}
	if !found {
		// No need to transform name.
		return cs.KeyVal(".name", name)
	}

	// Normalize excluded keys from the name.
	base, parts := r.NameParts()
	newName := make([]byte, 0, 100)
	newName = append(newName, base...)
	for _, part := range parts {
		eq := bytes.IndexByte(part, '=') + 1
		if eq > 0 {
			replacement, ok := p.replace[string(part[:eq])]
			if ok {
				newName = append(newName, replacement...)
				continue
			}
		}
		if p.excGomaxprocs && part[0] == '-' {
			newName = append(newName, '-', '*')
			continue
		}
		newName = append(newName, part...)
	}
	return cs.KeyVal(".name", cs.Bytes(newName))
}

func (p *ProjectFullName) AppendStaticKeys(keys []string) []string {
	return append(keys, ".name")
}

// ProjectBaseName projects the base name of a benchmark, without any
// per-benchmark configuration.
type ProjectBaseName struct{}

// Project returns a key/value Config with the key ".base" and a value
// of the base name of the benchmark.
func (p *ProjectBaseName) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	baseName, _ := r.NameParts()
	return cs.KeyVal(".base", cs.Bytes(baseName))
}

func (p *ProjectBaseName) AppendStaticKeys(keys []string) []string {
	return append(keys, ".base")
}

// ProjectNameKey projects a specific key from per-benchmark
// configuration. The key must begin with "/". The key "/gomaxprocs"
// will match the implicit GOMAXPROCS specified as "-N" at the end of
// a benchmark name.
type ProjectNameKey struct {
	Key string

	once       sync.Once
	prefix     []byte
	gomaxprocs bool
}

// Project returns a key/value Config with the key p.Key and the value
// of p.Key from the benchmark's name configuration. If p.Key isn't
// present in the name configuration, the value is "".
func (p *ProjectNameKey) Project(cs *ConfigSet, r *benchfmt.Result) *Config {
	p.once.Do(func() {
		if !strings.HasPrefix(p.Key, "/") {
			panic("name key must being with /")
		}

		// Construct the byte prefix to search for.
		prefix := make([]byte, len(p.Key)+1)
		copy(prefix, p.Key)
		prefix[len(prefix)-1] = '='
		p.prefix = prefix
		p.gomaxprocs = p.Key == "/gomaxprocs"
	})

	_, parts := r.NameParts()
	if p.gomaxprocs && len(parts) > 0 {
		last := parts[len(parts)-1]
		if last[0] == '-' {
			// GOMAXPROCS specified as "-N" suffix.
			return cs.KeyVal(p.Key, cs.Bytes(last[1:]))
		}
	}
	// Search for the prefix.
	for _, part := range parts {
		if bytes.HasPrefix(part, p.prefix) {
			return cs.KeyVal(p.Key, cs.Bytes(part[len(p.prefix):]))
		}
	}
	// Not found.
	return cs.KeyVal(p.Key, "")
}

func (p *ProjectNameKey) AppendStaticKeys(keys []string) []string {
	return append(keys, p.Key)
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
