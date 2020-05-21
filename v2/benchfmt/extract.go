// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

// An Extractor returns some component of a benchmark result.
type Extractor func(*Result) string

// NewExtractor returns a function that extracts some component of a
// benchmark result.
//
// The key must be one of the following:
//
// - ".name" for the benchmark name (excluding per-benchmark
// configuration).
//
// - ".full" for the full benchmark name (including configuration).
//
// - "/{key}" for a benchmark name key. This may be "/gomaxprocs" and
// the extractor will normalize the name as needed.
//
// - Any other string is a file configuration key.
func NewExtractor(key string) (Extractor, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("key must not be empty")
	}

	switch key[0] {
	case '.':
		if key == ".name" {
			return extractName, nil
		} else if key == ".full" {
			return extractFull, nil
		}
		return nil, fmt.Errorf("unknown special key: %s", key)

	case '/':
		// Construct the byte prefix to search for.
		prefix := make([]byte, len(key)+1)
		copy(prefix, key)
		prefix[len(prefix)-1] = '='
		isGomaxprocs := key == "/gomaxprocs"
		return func(res *Result) string {
			return extractNamePart(res, prefix, isGomaxprocs)
		}, nil

	default:
		if isFileKey(key) {
			return func(res *Result) string {
				return extractFileKey(res, key)
			}, nil
		}
	}

	return nil, fmt.Errorf("expected .name, .full, /key, or file key: %s", key)
}

func isFileKey(key string) bool {
	// See parseKeyValueLine.
	if len(key) == 0 {
		return false
	}
	for i, r := range key {
		if i == 0 && !unicode.IsLower(r) {
			return false
		}
		if unicode.IsSpace(r) || unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// NewExtractorFullName returns an extractor for the full name of a
// benchmark, but optionally with name configuration keys excluded.
// Any excluded keys will be normalized to "/key=*" (or "-*" for
// gomaxprocs). This will ignore anything in the exclude list that
// isn't in the form of a /-prefixed name configuration key.
func NewExtractorFullName(exclude []string) Extractor {
	// Extract the name keys, turn them into substrings and
	// construct their normalized replacement.
	var replace [][]byte
	excGomaxprocs := false
	for _, k := range exclude {
		if !strings.HasPrefix(k, "/") {
			continue
		}
		replace = append(replace, append([]byte(k), '='))
		if k == "/gomaxprocs" {
			excGomaxprocs = true
		}
	}
	if len(replace) == 0 && !excGomaxprocs {
		return extractFull
	}
	return func(res *Result) string {
		return extractFullExcluded(res, replace, excGomaxprocs)
	}
}

func extractName(res *Result) string {
	return string(res.BaseName())
}

func extractFull(res *Result) string {
	return string(res.FullName)
}

func extractFullExcluded(res *Result, replace [][]byte, excGomaxprocs bool) string {
	name := res.FullName
	found := false
	for _, k := range replace {
		if bytes.Contains(name, k) {
			found = true
			break
		}
	}
	if excGomaxprocs && bytes.IndexByte(name, '-') >= 0 {
		found = true
	}
	if !found {
		// No need to transform name.
		return string(name)
	}

	// Normalize excluded keys from the name.
	base, parts := res.NameParts()
	var newName strings.Builder
	newName.Grow(len(name))
	newName.Write(base)
outer:
	for _, part := range parts {
		for _, k := range replace {
			if bytes.HasPrefix(part, k) {
				newName.Write(k)
				newName.WriteByte('*')
				continue outer
			}
		}
		if excGomaxprocs && part[0] == '-' {
			newName.WriteString("-*")
			continue outer
		}
		newName.Write(part)
	}
	return newName.String()
}

func extractNamePart(res *Result, prefix []byte, isGomaxprocs bool) string {
	_, parts := res.NameParts()
	if isGomaxprocs && len(parts) > 0 {
		last := parts[len(parts)-1]
		if last[0] == '-' {
			// GOMAXPROCS specified as "-N" suffix.
			return string(last[1:])
		}
	}
	// Search for the prefix.
	for _, part := range parts {
		if bytes.HasPrefix(part, prefix) {
			return string((part[len(prefix):]))
		}
	}
	// Not found.
	return ""
}

func extractFileKey(res *Result, key string) string {
	pos, ok := res.FileConfigIndex(key)
	val := ""
	if ok {
		val = res.FileConfig[pos].Value
	}
	return val
}
