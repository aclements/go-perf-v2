// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchproc provides tools for processing benchmark results.
//
// This package provides a processing model based around applying any
// subset of the following steps to a stream of benchmark results:
//
// 1. Transform each benchmark.Result to add or modify keys according
// to a particular tool's needs. In particular, benchmark.Result is
// used directly as the mutable representation of a result.
//
// 2. Filter each benchmark.Result according to a user predicate. See
// NewFilter for creating filters. Filters can exclude entire Results,
// or just particular values from a Result.
//
// 3. Project components of a benchmark.Result according to a user
// projection expression. See ProjectionParser. Projecting a Result
// produces a Config, which is an immutable tuple whose structure is
// described by a Schema. Identical Configs compare == and can be used
// as map keys. Generally, tools will want to group Results by Config
// and perform some processing on these groups.
//
// 4. Sort the observed Configs once all Results have been collected.
// A projection expression also describes a sort order for Configs
// produced by that projection.
package benchproc

import (
	"fmt"
	"hash/maphash"
	"strconv"
	"strings"

	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc/internal/kvql"
)

// TODO: the "key:(val1 val2)" syntax looks like a filter expression,
// but in filter expressions the values are regexps, while here
// they're literals. We could make them regexps here, too, but then we
// would need a secondary sort for unequal keys that mapped to the
// same regexp order. We could just say this is always observation
// order, or we could allow a more general key:(val1 val2)@order
// syntax.

// TODO: If we support comparison operators in filter expressions,
// does it make sense to unify the orders understood by projections
// with the comparison orders supported in filters? One danger is that
// the default order for projections is observation order, but if you
// filter on key<val, you probably want that to be numeric by default
// (it's not clear you ever want a comparison on observation order).

// A ProjectionParser parses projection expressions, which describe
// how to extract components of a benchfmt.Result into a Config and
// how to order the resulting Configs.
//
// A projection expression specifies a tuple as a comma-separated
// list. Each component of the tuple specifies a key and optionally a
// sort order and a filter using the following syntax:
//
// - "{key}[@{order}]" specifies one of the built-in sort orders. If
// order is omitted, it uses the default first-observation order.
//
// - "{key}:({val} {val}...)" specifies a fixed value order for key.
// It also specifies a filter: if key has a value that isn't any of
// the specified values, the benchfmt.Result is filtered out.
//
// The key can be any key accepted by benchfmt.NewExtractor, or
// ".config", which is a group key for all file configuration keys.
//
// Multiple projections can be parsed by one ProjectionParser, and
// they form a mutually-exclusive group of projections in which
// specific keys in any projection are excluded from group keys in any
// projection. ".config" and ".fullname" are group keys for the file
// and per-benchmark configuration, respectively. For example, given
// two projections ".config" and "commit,date", the specific file
// configuration keys "commit" and "date" are excluded from the group
// key ".config".
type ProjectionParser struct {
	configKeys   map[string]bool // Specific .config keys (excluded from .config)
	fullnameKeys []string        // Specific name keys (excluded from .fullname)
	haveConfig   bool            // .config was projected
	haveFullname bool            // .fullname was projected

	// Fields below here are constructed when the first Result is
	// processed.

	fullExtractor benchfmt.Extractor
}

// Parse parses a single projection expression.
func (p *ProjectionParser) Parse(proj string) (*Schema, error) {
	if p.configKeys == nil {
		p.configKeys = make(map[string]bool)
	}

	s := newSchema()

	// Parse the projection. This syntax is non-recursive, so we
	// don't need anything fancy.
	toks, err := kvql.Tokenize(proj)
	if err != nil {
		return nil, err
	}
	for len(toks) > 0 {
		// Process the key.
		if !(toks[0].Kind == 'w' || toks[0].Kind == 'q') {
			return nil, &kvql.SyntaxError{proj, toks[0].Off, "expected key"}
		}
		key := toks[0]
		toks = toks[1:]
		// Process the sort order.
		order := "first"
		var exact []string
		if toks[0].Kind == '@' {
			if !(toks[1].Kind == 'w' || toks[1].Kind == 'q') {
				return nil, &kvql.SyntaxError{proj, toks[1].Off, "expected sort order"}
			}
			order = toks[1].Tok
			toks = toks[2:]
		} else if toks[0].Kind == ':' {
			// TODO: For similarity with the filter
			// syntax, should we accept a bare word here?
			if toks[1].Kind != '(' {
				return nil, &kvql.SyntaxError{proj, toks[1].Off, "expected ("}
			}
			start := toks[1].Off
			toks = toks[2:]
			for toks[0].Kind == 'w' || toks[0].Kind == 'q' {
				exact = append(exact, toks[0].Tok)
				toks = toks[1:]
			}
			if toks[0].Kind != ')' {
				return nil, &kvql.SyntaxError{proj, toks[0].Off, "expected )"}
			}
			if len(exact) == 0 {
				return nil, &kvql.SyntaxError{proj, start, "nothing to match"}
			}
		}

		if err := p.makeProjection(s, key.Tok, order, exact); err != nil {
			return nil, &kvql.SyntaxError{proj, key.Off, err.Error()}
		}

		if !(toks[0].Kind == ',' || toks[0].Kind == 0) {
			return nil, &kvql.SyntaxError{proj, toks[0].Off, "expected ,"}
		}
		toks = toks[1:]
	}

	return s, nil
}

// Remainder returns a projection for any keys not yet projected by
// any parsed projection. The resulting Schema does not have a
// meaningful order.
func (p *ProjectionParser) Remainder() *Schema {
	s := newSchema()

	// The .config and .fullname groups together cover the
	// projection space. If they haven't already been specified,
	// then these groups (with any specific keys excluded) exactly
	// form the remainder.
	if !p.haveConfig {
		p.makeProjection(s, ".config", "first", nil)
	}
	if !p.haveFullname {
		p.makeProjection(s, ".fullname", "first", nil)
	}

	return s
}

func (p *ProjectionParser) makeProjection(s *Schema, key string, order string, exact []string) error {
	// Construct the order function.
	var initField func(node *schemaNode)
	var match func(a []byte) bool
	if exact != nil {
		exactMap := make(map[string]int, len(exact))
		for i, s := range exact {
			exactMap[s] = i
		}
		initField = func(node *schemaNode) {
			node.less = func(a, b string) bool {
				return exactMap[a] < exactMap[b]
			}
		}
		match = func(a []byte) bool {
			_, ok := exactMap[string(a)]
			return ok
		}
	} else if order == "first" {
		initField = func(node *schemaNode) {
			node.order = make(map[string]int)
		}
	} else if less, ok := builtinOrders[order]; ok {
		initField = func(node *schemaNode) {
			node.less = less
		}
	} else {
		return fmt.Errorf("unknown order %q", order)
	}

	var project func(*benchfmt.Result, *[]string) bool
	switch key {
	case ".config":
		// File configuration, excluding any more
		// specific file keys.
		if match != nil {
			// Exact orders don't make sense for a whole tuple.
			return fmt.Errorf("exact order not allowed for .config")
		}
		p.haveConfig = true
		group := s.addGroup(nil, ".config")
		seen := make(map[string]*schemaNode)
		project = func(r *benchfmt.Result, row *[]string) bool {
			for _, cfg := range r.FileConfig {
				field, ok := seen[cfg.Key]
				if !ok {
					if p.configKeys[cfg.Key] {
						continue
					}
					field = s.addField(group, cfg.Key)
					initField(field)
					seen[cfg.Key] = field
				}

				(*row)[field.idx] = s.intern(cfg.Value)
			}
			return true
		}

	case ".fullname":
		// Full benchmark name, including name config.
		// We want to exclude any more specific keys,
		// including keys from later projections, so
		// we delay constructing the extractor until
		// we process the first Result.
		//
		// TODO: Does this handle excluding empty keys vs
		// missing keys from the fullname correctly?
		p.haveFullname = true
		field := s.addField(nil, ".fullname")
		initField(field)
		project = func(r *benchfmt.Result, row *[]string) bool {
			if p.fullExtractor == nil {
				p.fullExtractor = benchfmt.NewExtractorFullName(p.fullnameKeys)
			}
			val := p.fullExtractor(r)
			if match != nil && !match(val) {
				return false
			}
			(*row)[field.idx] = s.intern(val)
			return true
		}

	default:
		// This is a specific name or file key. Add it
		// to the excludes.
		if key == ".name" || strings.HasPrefix(key, "/") {
			p.fullnameKeys = append(p.fullnameKeys, key)
		} else {
			p.configKeys[key] = true
		}
		ext, err := benchfmt.NewExtractor(key)
		if err != nil {
			return err
		}
		field := s.addField(nil, key)
		initField(field)
		project = func(r *benchfmt.Result, row *[]string) bool {
			val := ext(r)
			if match != nil && !match(val) {
				return false
			}
			(*row)[field.idx] = s.intern(val)
			return true
		}
	}
	s.project = append(s.project, project)
	return nil
}

// builtinOrders is the built-in comparison functions.
var builtinOrders = map[string]func(a, b string) bool{
	"alpha": func(a, b string) bool {
		return a < b
	},
	"numeric": func(a, b string) bool {
		aa, erra := strconv.ParseFloat(a, 64)
		bb, errb := strconv.ParseFloat(b, 64)
		if erra == nil && errb == nil {
			return aa < bb
		} else if erra != nil && errb != nil {
			// Fall back to string order.
			return a < b
		} else {
			// Put floats before non-floats.
			return erra == nil
		}
	},
}

// A Schema projects some subset of the components in a
// benchmark.Result into a Config. All Configs produced by a Schema
// have the same structure. Configs produced by a Schema will be == if
// they have the same values (notably, this means Configs can be used
// as map keys). A Schema also implies a sort order, which is
// lexicographic based on the order of fields in the Schema, with the
// order of each individual field determined by the projection.
type Schema struct {
	root    schemaNode
	nFields int

	// unitNode, if non-nil, is the ".unit" field used to project
	// the values of a benchmark result.
	unitNode *schemaNode

	// flatCache, if non-nil, contains the flattened schema.
	flatCache []*schemaNode

	// project is a set of functions that project a Result into
	// row.
	//
	// These take a pointer to row because these functions may
	// grow the schema, so the row slice may grow.
	project []func(r *benchfmt.Result, row *[]string) bool

	// row is the buffer used to construct a projection.
	row []string

	// interns is used to intern []byte to string. These are
	// always referenced in Configs, so this doesn't cause any
	// over-retention.
	interns map[string]string

	// configs are the interned Configs of this Schema.
	configs map[uint64][]*configNode
}

func newSchema() *Schema {
	var s Schema
	s.root.idx = -1
	s.interns = make(map[string]string)
	s.configs = make(map[uint64][]*configNode)
	return &s
}

// A schemaNode is a field or group in a Schema.
type schemaNode struct {
	name string

	// idx gives the index of this field's values in a configNode.
	// Indexes are assigned sequentially as fields are added,
	// regardless of the order of those fields in the Schema. This
	// allows new fields to be added to a schema without
	// invalidating existing Configs.
	//
	// idx is -1 for group nodes.
	idx int
	sub []*schemaNode // sub-nodes for groups

	// less is the comparison function for this field. If nil, use
	// the observation order.
	less func(a, b string) bool

	// order, if non-nil, records the observation order of this
	// field.
	order map[string]int
}

func (s *Schema) addField(group *schemaNode, name string) *schemaNode {
	if group == nil {
		group = &s.root
	}
	if group.idx != -1 {
		panic("field's parent is not a group")
	}

	// Assign this field an index.
	node := &schemaNode{name: name, idx: s.nFields}
	s.nFields++
	group.sub = append(group.sub, node)
	// Add to the row buffer.
	s.row = append(s.row, "")
	// Clear the current flattening.
	s.flatCache = nil
	return node
}

func (s *Schema) addGroup(group *schemaNode, name string) *schemaNode {
	if group == nil {
		group = &s.root
	}
	node := &schemaNode{name: name, idx: -1}
	group.sub = append(group.sub, node)
	return node
}

// AddValues appends a field to this Schema called ".unit" used to
// project out each distinct benchfmt.Value in a benchfmt.Result.
//
// For Schemas that have a .unit field, callers should use
// ProjectValues instead of Project.
//
// Typically, callers need to break out individual benchmark values on
// some dimension of a set of Schemas. Adding a .unit field makes this
// easy.
func (s *Schema) AddValues() Field {
	if s.unitNode != nil {
		panic("Schema already has a .unit field")
	}
	s.unitNode = s.addField(nil, ".unit")
	return Field{s.unitNode.name, s, s.unitNode}
}

// flat returns the flattened schema.
func (s *Schema) flat() []*schemaNode {
	if s.flatCache != nil {
		return s.flatCache
	}

	s.flatCache = make([]*schemaNode, 0, s.nFields)
	var walk func(n *schemaNode)
	walk = func(n *schemaNode) {
		if n.idx != -1 {
			s.flatCache = append(s.flatCache, n)
		} else {
			for _, sub := range n.sub {
				walk(sub)
			}
		}
	}
	walk(&s.root)
	return s.flatCache
}

// Fields returns the fields of s in the order determined by the
// Schema's projection expression. Group projections can result in
// zero or more fields. Calling s.Project can cause more fields to be
// added to s (for example, if the Result has a new file configuration
// key).
func (s *Schema) Fields() []Field {
	nodes := s.flat()
	fields := make([]Field, len(nodes))
	for i, node := range nodes {
		fields[i] = Field{node.name, s, node}
	}
	return fields
}

// A Field is a single dimension of a Schema.
type Field struct {
	Name   string
	schema *Schema
	node   *schemaNode
}

var configSeed = maphash.MakeSeed()

// Project extracts components from benchmark Result r according to
// Schema s and returns them as an immutable Config. If the projection
// filters this result, it returns a zero Config and false.
//
// If this Schema includes a .units field, it will be left as "" in
// the resulting Config. The caller should use ProjectValues instead.
func (s *Schema) Project(r *benchfmt.Result) (Config, bool) {
	if !s.populateRow(r) {
		return Config{}, false
	}
	return s.internRow(), true
}

// ProjectValues is like Project, but for each benchmark value of
// r.Values individually. The returned slice corresponds to the
// r.Values slice.
//
// If this Schema includes a .units field, it will differ between
// these Configs. If not, then all of the Configs will be identical
// because the benchmark values vary only on .unit.
func (s *Schema) ProjectValues(r *benchfmt.Result) ([]Config, bool) {
	if !s.populateRow(r) {
		return nil, false
	}
	out := make([]Config, len(r.Values))
	if s.unitNode == nil {
		// There's no .unit, so the Configs will all be the same.
		cfg := s.internRow()
		for i := range out {
			out[i] = cfg
		}
		return out, true
	}
	// Vary the .unit field.
	for i, val := range r.Values {
		s.row[s.unitNode.idx] = val.Unit
		out[i] = s.internRow()
	}
	return out, true
}

func (s *Schema) populateRow(r *benchfmt.Result) bool {
	// Clear the row buffer.
	for i := range s.row {
		s.row[i] = ""
	}

	// Run the projection functions to fill in row.
	for _, proj := range s.project {
		// proj may add fields and grow row.
		if !proj(r, &s.row) {
			return false
		}
	}
	return true
}

func (s *Schema) internRow() Config {
	// Hash the configuration. This must be invariant to unused
	// trailing fields: the schema can grow, and if those new
	// fields are later cleared, we want configurations from
	// before the growth to equal configurations from after the
	// growth.
	row := s.row
	for len(row) > 0 && row[len(row)-1] == "" {
		row = row[:len(row)-1]
	}
	var h maphash.Hash
	h.SetSeed(configSeed)
	for _, val := range row {
		h.WriteString(val)
	}
	hash := h.Sum64()

	// Check if we already have this configuration.
	configs := s.configs[hash]
	for _, config := range configs {
		if config.equalRow(row) {
			return Config{config}
		}
	}

	// Update observation orders.
	for _, node := range s.flat() {
		if node.order == nil {
			// Not tracking observation order for this field.
			continue
		}
		var val string
		if node.idx < len(row) {
			val = row[node.idx]
		}
		if _, ok := node.order[val]; !ok {
			node.order[val] = len(node.order)
		}
	}

	// Save the config.
	config := &configNode{s, append([]string(nil), row...)}
	s.configs[hash] = append(s.configs[hash], config)
	return Config{config}
}

func (s *Schema) intern(b []byte) string {
	if str, ok := s.interns[string(b)]; ok {
		return str
	}
	str := string(b)
	s.interns[str] = str
	return str
}

// A Config is an immutable tuple mapping from Fields to strings whose
// structure is given by a Schema. Two Configs are == if they come
// from the same Schema and have identical values.
type Config struct {
	c *configNode
}

// IsZero returns true if c is a zeroed Config with no schema and no
// fields.
func (c Config) IsZero() bool {
	return c.c == nil
}

// Get returns the value of Field f in this Config.
//
// It panics if Field f does not come from the same Schema as the
// Config.
func (c Config) Get(f Field) string {
	if c.IsZero() {
		panic("zero Config has no fields")
	}
	if c.c.schema != f.schema {
		panic("Config and Field have different Schemas")
	}
	idx := f.node.idx
	if idx >= len(c.c.vals) {
		return ""
	}
	return c.c.vals[idx]
}

// Schema returns the Schema describing Config c.
func (c Config) Schema() *Schema {
	if c.IsZero() {
		return nil
	}
	return c.c.schema
}

// String returns Config as a space-separated sequence of key:value
// pairs.
func (c Config) String() string {
	if c.IsZero() {
		return "<zero>"
	}
	buf := new(strings.Builder)
	for _, node := range c.c.schema.flat() {
		if node.idx >= len(c.c.vals) {
			continue
		}
		val := c.c.vals[node.idx]
		if val == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(node.name)
		buf.WriteByte(':')
		buf.WriteString(val)
	}
	return buf.String()
}

// commonSchema returns the Schema that all configs have, or panics if
// any Config has a different Schema. It returns nil if len(configs)
// == 0.
func commonSchema(configs []Config) *Schema {
	if len(configs) == 0 {
		return nil
	}
	s := configs[0].Schema()
	for _, c := range configs[1:] {
		if c.Schema() != s {
			panic("Configs must all have the same Schema")
		}
	}
	return s
}

// configNode is the internal heap-allocated object backing a Config.
// This allows Config itself to be a value type whose equality is
// determined by the pointer equality of the underlying configNode.
type configNode struct {
	schema *Schema
	// vals are the values in this Config, indexed by
	// schemaNode.idx. Trailing ""s are always trimmed.
	//
	// Notably, this is *not* in the order of the flattened
	// schema. This is because fields can be added in the middle
	// of a schema on-the-fly, and we need to not invalidate
	// existing Configs.
	vals []string
}

func (n *configNode) equalRow(row []string) bool {
	if len(n.vals) != len(row) {
		return false
	}
	for i, v := range n.vals {
		if row[i] != v {
			return false
		}
	}
	return true
}
