// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchunit

import "testing"

func TestUnitClassOf(t *testing.T) {
	test := func(unit string, cls UnitClass) {
		t.Helper()
		got := UnitClassOf(unit)
		if got != cls {
			t.Errorf("for %s, want %s, got %s", unit, cls, got)
		}
	}
	test("ns/op", UnitClassSI)
	test("sec/op", UnitClassSI)
	test("sec/B", UnitClassSI)
	test("sec/B/B", UnitClassSI)
	test("sec/disk-B", UnitClassSI)

	test("B/op", UnitClassIEC)
	test("bytes/op", UnitClassIEC)
	test("B/s", UnitClassIEC)
	test("B/sec", UnitClassIEC)
	test("sec/B*B", UnitClassIEC) // Discouraged
	test("disk-B/sec", UnitClassIEC)
	test("disk-B/sec", UnitClassIEC)
}
