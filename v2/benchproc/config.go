// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import "strings"

// A Config is either a key/value pair or a tuple of Configs.
//
// A Config is immutable and is constructed via a ConfigSet, which
// ensures arbitrarily complex Configs (from the same ConfigSet) can
// be compared using just pointer equality.
//
// Nesting of Config tuples allows constructing complex
// configurations. And tuple Configs can be efficiently extended to
// form new Configs.
type Config struct {
	configKV
	configTuple

	// For a tuple config, the total length of the tuple
	// terminating in this Config.
	tupleLen int
}

type configKV struct {
	key, val string
}

type configTuple struct {
	// A tuple config is represented as a chain of pairs, where
	// each pair adds an element to the end. This is like a
	// reversed singly-linked list.

	// prefix is nil or a tuple config for all but the last element.
	prefix *Config
	// elt is the last element in this tuple.
	elt *Config
}

// IsKeyVal returns whether c is a key/value Config.
func (c *Config) IsKeyVal() bool {
	return c != nil && c.elt == nil
}

// KeyVal returns the key and value of a key/value Config.
func (c *Config) KeyVal() (key, val string) {
	if !c.IsKeyVal() {
		return "", ""
	}
	return c.key, c.val
}

// Val returns the value of a key/value Config. This is useful when
// the key is already known.
func (c *Config) Val() string {
	return c.val
}

// Tuple returns the elements of a tuple Config.
func (c *Config) Tuple() []*Config {
	if c == nil {
		return []*Config{}
	} else if c.IsKeyVal() {
		return nil
	}
	// Collect the elements.
	out := make([]*Config, c.tupleLen)
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = c.elt
		c = c.prefix
	}
	return out
}

// TupleLen returns the length of a tuple Config.
func (c *Config) TupleLen() int {
	if c == nil {
		return 0
	}
	return c.tupleLen
}

// PrefixLast returns the tuple containing all but the last element
// and the last element of a tuple Config.
func (c *Config) PrefixLast() (*Config, *Config) {
	if c == nil || c.IsKeyVal() {
		return nil, nil
	}
	return c.prefix, c.elt
}

// String returns a string representation of c.
func (c *Config) String() string {
	if c.IsKeyVal() {
		return c.key + ":" + c.val
	}
	buf := new(strings.Builder)
	var walk func(c *Config)
	walk = func(c *Config) {
		if c.IsKeyVal() {
			buf.WriteString(c.key)
			buf.WriteByte(':')
			buf.WriteString(c.val)
		} else {
			buf.WriteByte('(')
			t := c.Tuple()
			for i, cfg := range t {
				if i > 0 {
					buf.WriteString(", ")
				}
				walk(cfg)
			}
			buf.WriteByte(')')
		}
	}
	walk(c)
	return buf.String()

}

// A ConfigSet is a collection of Configs. Configs within a single
// ConfigSet can be compared for equality using pointer comparison.
type ConfigSet struct {
	kvs    map[configKV]*Config
	tuples map[configTuple]*Config

	strings map[string]string
}

// Bytes interns str. This is useful for strings that are going to be
// retained by key/value Configs anyway.
func (s *ConfigSet) Bytes(bytes []byte) string {
	if s.strings == nil {
		s.strings = make(map[string]string)
	}
	if str, ok := s.strings[string(bytes)]; ok {
		return str
	}
	str := string(bytes)
	s.strings[str] = str
	return str
}

// KeyVal constructs a key/value Config.
func (s *ConfigSet) KeyVal(key, val string) *Config {
	if s.kvs == nil {
		s.kvs = make(map[configKV]*Config)
	}
	kv := configKV{key, val}
	c := s.kvs[kv]
	if c == nil {
		c = &Config{configKV: kv}
		s.kvs[kv] = c
	}
	return c
}

// Tuples constructs a tuple Config.
func (s *ConfigSet) Tuple(elts ...*Config) *Config {
	return s.Append(nil, elts...)
}

// Append constructs a tuple Config that appends elts to the end of
// base.
func (s *ConfigSet) Append(base *Config, elts ...*Config) *Config {
	if s.tuples == nil {
		s.tuples = make(map[configTuple]*Config)
	}

	baseLen := 0
	if base != nil {
		baseLen = base.tupleLen
	}
	prefix := base
	found := true
	for i, elt := range elts {
		key := configTuple{prefix, elt}
		if found {
			// Everything up to this point has been
			// interned, so this might be, too.
			if c := s.tuples[key]; c != nil {
				prefix = c
				continue
			}
			// Not interned, so no remaining pairs will be
			// either.
			found = false
		}
		c := &Config{configTuple: key, tupleLen: baseLen + i + 1}
		s.tuples[key] = c
		prefix = c
	}
	return prefix
}
