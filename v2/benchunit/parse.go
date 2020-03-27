// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchunit works with benchmark units.
//
// It provides functions for parsing and interpreting units and for
// printing numbers in those units.
package benchunit

import (
	"fmt"
	"unicode"
)

// UnitClass distinguishes units that should be scaled differently.
type UnitClass int

const (
	// UnitClassSI indicates values of a given unit should be
	// scaled by powers of 1000 and use the International System
	// of Units SI prefixes.
	UnitClassSI UnitClass = iota
	// UnitClassIEC indicates values of a given unit should be
	// scaled by powers of 1024 and use the International
	// Electrotechnical Commission binary prefixes.
	UnitClassIEC
)

func (c UnitClass) String() string {
	switch c {
	case UnitClassSI:
		return "UnitClassSI"
	case UnitClassIEC:
		return "UnitClassIEC"
	}
	return fmt.Sprintf("UnitClass(%d)", int(c))
}

// UnitClassOf returns the UnitClass of unit. If unit contains some
// measure of bytes in the numerator, this is UnitClassIEC. Otherwise,
// it is UnitClassSI.
func UnitClassOf(unit string) UnitClass {
	p := newParser(unit)
	for p.next() {
		if (p.tok == "B" || p.tok == "MB" || p.tok == "bytes") && !p.denom {
			return UnitClassIEC
		}
	}
	return UnitClassSI
}

type parser struct {
	rest string // unparsed unit
	rpos int    // byte consumed from original unit

	// Current token
	tok   string
	pos   int  // byte offset of tok in original unit
	denom bool // current token is in denominator
}

func newParser(unit string) *parser {
	return &parser{rest: unit}
}

func (p *parser) next() bool {
	// Consume separators.
	for i, r := range p.rest {
		if r == '*' {
			p.denom = false
		} else if r == '/' {
			p.denom = true
		} else if !(r == '-' || unicode.IsSpace(r)) {
			p.rpos += i
			p.rest = p.rest[i:]
			goto tok
		}
	}
	// End of string.
	p.rest = ""
	return false

tok:
	// Consume until separator.
	end := len(p.rest)
	for i, r := range p.rest {
		if r == '*' || r == '/' || r == '-' || unicode.IsSpace(r) {
			end = i
			break
		}
	}
	p.tok = p.rest[:end]
	p.pos = p.rpos
	p.rpos += end
	p.rest = p.rest[end:]
	return true
}
