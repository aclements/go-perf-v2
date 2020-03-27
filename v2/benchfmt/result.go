// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchfmt provides a high-performance reader and writer for
// the Go benchmark format.
//
// The reader and writer are structured as streaming operations to
// allow incremental processing and avoid dictating a data model. This
// allows consumers of these APIs to provide their own data model best
// suited to its needs.
//
// The reader is designed to process millions of benchmark results per
// second.
//
// The format is documented at https://golang.org/design/14313-benchmark-format
package benchfmt

// Result is a single benchmark result and all of its measurements.
type Result struct {
	// FileConfig is the set of file-level key/value pairs in
	// effect for this result.
	//
	// Callers should not modify or set this directly and should
	// instead use SetFileConfig.
	//
	// This is modified in place. New keys are appended to the
	// slice. When an existing key changes value, it is updated in
	// place. When a key is deleted, its Value is set to "". As a
	// consequence, consumers can cache the indexes of keys.
	FileConfig []Config

	// FullName is the full name of this benchmark, including all
	// sub-benchmark configuration.
	FullName []byte

	// Iters is the number of iterations this benchmark's results
	// were averaged over.
	Iters int

	// Values is this benchmark's measurements and their units.
	Values []Value

	// configPos, if non-nil, maps from Config.Key to index in
	// FileConfig.
	configPos map[string]int

	// permConfig indicates that FileConfig[:permConfig] cannot be
	// overridden.
	permConfig int

	// nameParts is a cache of the split parts of FullName. Its
	// length is 0 if it has not been computed.
	nameParts [][]byte
}

// Config is a single key/value configuration pair.
type Config struct {
	Key, Value string
}

// Value is a single value/unit measurement from a benchmark result.
type Value struct {
	Value float64
	Unit  string
}

// Clone makes a copy of Result that shares no state with r.
func (r *Result) Clone() *Result {
	// All of these slices share no sub-structure.
	r2 := &Result{
		FileConfig: append([]Config(nil), r.FileConfig...),
		FullName:   append([]byte(nil), r.FullName...),
		Iters:      r.Iters,
		Values:     append([]Value(nil), r.Values...),
		permConfig: r.permConfig,
	}
	return r2
}

// setFileConfig sets file configuration key to value, overriding or
// adding the configuration as necessary. perm indicates that this is
// a permanent config value that cannot be overridden by a file.
func (r *Result) setFileConfig(key, value string, perm bool) {
	pos, ok := r.FileConfigIndex(key)
	if ok {
		if !perm && pos < r.permConfig {
			// Cannot override permanent config.
			return
		}
		r.FileConfig[pos].Value = value
		return
	}
	pos = len(r.FileConfig)
	if perm {
		if pos != r.permConfig {
			panic("setting permanent file config after reading file")
		}
		r.permConfig = pos + 1
	}
	r.FileConfig = append(r.FileConfig, Config{key, value})
	r.configPos[key] = pos
}

// FileConfigIndex returns the index in r.FileConfig of key.
func (r *Result) FileConfigIndex(key string) (pos int, ok bool) {
	if r.configPos == nil {
		r.configPos = make(map[string]int)
		for i, cfg := range r.FileConfig {
			r.configPos[cfg.Key] = i
		}
	}
	pos, ok = r.configPos[key]
	return
}

// Value returns the measurement for the given unit.
func (r *Result) Value(unit string) (float64, bool) {
	for _, v := range r.Values {
		if v.Unit == unit {
			return v.Value, true
		}
	}
	return 0, false
}

// NameParts returns the base name and sub-benchmark configuration
// parts. Each sub-benchmark configuration part is one of three forms:
//
// 1. "/<key>=<value>" indicates a key/value configuration pair.
//
// 2. "/<string>" indicates a positional configuration pair.
//
// 3. "-<gomaxprocs>" indicates the GOMAXPROCS of this benchmark. This
// component can only appear last.
//
// Concatenating the base name and the configuration parts
// reconstructs the full name.
func (r *Result) NameParts() (baseName []byte, parts [][]byte) {
	if len(r.nameParts) == 0 {
		buf := r.FullName
		// First pull off any GOMAXPROCS.
		var gomaxprocs []byte
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '-' && i < len(buf)-1 {
				gomaxprocs, buf = buf[i:], buf[:i]
				break
			} else if !('0' <= buf[i] && buf[i] <= '9') {
				// Not a digit.
				break
			}
		}
		// Split the remaining parts.
		prev := 0
		for i, c := range buf {
			if c == '/' {
				r.nameParts = append(r.nameParts, buf[prev:i])
				prev = i
			}
		}
		r.nameParts = append(r.nameParts, buf[prev:])
		if gomaxprocs != nil {
			r.nameParts = append(r.nameParts, gomaxprocs)
		}
	}
	return r.nameParts[0], r.nameParts[1:]
}
