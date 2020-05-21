// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseOpts gives options to ParseProjection.
type ParseOpts struct {
	// Exclude is a list of keys to exclude from projections that
	// support excludes.
	Exclude []string

	// Special is an optional map of projection constructors for
	// special keys.
	Special map[string]func() Projection
}

// ParseProjection parses a projection from a specification.
//
// The projection syntax is as follows:
//
//   top         = tuple .
//   tuple       = [ projection { "," projection } ] .
//   projection  = "(" tuple ")" | special_key | name_key | file_key
//   special_key = ".name" | ".full" | ".config"
//   name_key    = "/" key
//   file_key    = key
//   key         = { [^,()[:space:]] }
func ParseProjection(proj string, opts ParseOpts) (Projection, error) {
	parser := parser{proj: proj, opts: opts}
	return parser.top()
}

// ParseProjectionBundle parses a list of mutually-exclusive
// projections. All static keys in any of these projections are added
// to the exclude sets of all dynamic projections.
func ParseProjectionBundle(projs []string, opts ParseOpts) ([]Projection, error) {
	// Parse all of the projections without any excludes and build
	// up the exclude list.
	exclude := append([]string(nil), opts.Exclude...)
	for _, proj := range projs {
		p, err := ParseProjection(proj, opts)
		if err != nil {
			return nil, err
		}
		exclude = p.AppendStaticKeys(exclude)
	}

	// Parse all of the projections again, supplying the exclude
	// list.
	opts.Exclude = exclude
	res := make([]Projection, len(projs))
	for i, proj := range projs {
		var err error
		res[i], err = ParseProjection(proj, opts)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

type parser struct {
	proj string
	pos  int
	opts ParseOpts
	err  error
}

func (p *parser) top() (Projection, error) {
	p.ws()
	proj := p.tuple()
	if p.pos != len(p.proj) {
		p.syntaxError("expected end")
	}
	if p.err != nil {
		return nil, p.err
	}
	return proj, nil
}

func (p *parser) syntaxError(msg string) {
	if p.err == nil {
		p.err = fmt.Errorf("%s in projection at position %d: %s>>>%s", msg, p.pos, p.proj[:p.pos], p.proj[p.pos:])
	}
}

func (p *parser) ws() {
	for i, r := range p.proj[p.pos:] {
		if !unicode.IsSpace(r) {
			p.pos += i
			return
		}
	}
	p.pos = len(p.proj)
}

func (p *parser) tuple() *ProjectProduct {
	product := new(ProjectProduct)
	for i := 0; ; i++ {
		s := p.proj[p.pos:]
		if i == 0 {
			if s == "" || strings.HasPrefix(s, ")") {
				break
			}
		} else if i > 0 {
			if !strings.HasPrefix(p.proj[p.pos:], ",") {
				break
			}
			p.pos++
			p.ws()
		}
		sub := p.projection()
		if sub == nil {
			return nil
		}
		*product = append(*product, sub)
	}
	return product
}

func (p *parser) projection() Projection {
	if strings.HasPrefix(p.proj[p.pos:], "(") {
		p.pos++
		p.ws()
		proj := p.tuple()
		if !strings.HasPrefix(p.proj[p.pos:], ")") {
			p.syntaxError("expected `,` or `)'")
			return nil
		}
		p.pos++
		p.ws()
		return proj
	}

	// Consume a key and then figure out what it is.
	start, end := p.pos, len(p.proj)
	for i, r := range p.proj[p.pos:] {
		if r == ',' || r == '(' || r == ')' || unicode.IsSpace(r) {
			end = start + i
			break
		}
	}
	if start == end {
		p.syntaxError("expected tuple or key")
	}
	key := p.proj[start:end]
	p.pos = end
	p.ws()

	// Distinguish key type.
	var proj Projection
	var err error
	if ctor, ok := p.opts.Special[key]; ok {
		proj = ctor()
	} else if key == ".name" {
		proj, err = NewProjectFullName(p.opts.Exclude)
	} else if key == ".config" {
		proj = NewProjectFileConfig(p.opts.Exclude)
	} else {
		proj, err = NewProjectKey(key)
		if err != nil {
			p.syntaxError(err.Error())
			return nil
		}
	}
	return proj
}
