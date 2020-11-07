// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"fmt"
	"testing"

	"golang.org/x/perf/v2/benchfmt"
)

func TestFilter(t *testing.T) {
	res := (&benchfmt.Result{
		FileConfig: []benchfmt.Config{{"f1", "v1"}, {"f2", "v2"}},
		FullName:   []byte("Name/n1=v3"),
		Values: []benchfmt.Value{
			{100, "ns/op"},
			{100, "B/op"},
		},
	}).Clone()
	const ALL = 0b11
	const NONE = 0

	check := func(t *testing.T, query string, want uint) {
		t.Helper()
		f, err := NewFilter(query)
		if err != nil {
			t.Fatal(err)
		}
		m := f.Match(res)
		var got uint
		for i := 0; i < 2; i++ {
			if m.Test(i) {
				got |= 1 << i
			}
		}
		if got != want {
			t.Errorf("%s: got %02b, want %02b", query, got, want)
		} else if want == ALL && !m.All() {
			t.Errorf("%s: want All", query)
		} else if want == 0 && m.Any() {
			t.Errorf("%s: want !Any", query)
		}
	}

	t.Run("basic", func(t *testing.T) {
		// File keys
		check(t, "f1:v1", ALL)
		check(t, "f1:v2", NONE)
		// Name keys
		check(t, "/n1:v3", ALL)
		// Special keys
		check(t, ".name:Name", ALL)
		check(t, ".fullname:Name/n1=v3", ALL)
	})

	t.Run("units", func(t *testing.T) {
		check(t, ".unit:ns/op", 0b01)
		check(t, ".unit:B/op", 0b10)
		check(t, ".unit:foo", 0b00)
	})

	t.Run("boolean", func(t *testing.T) {
		check(t, "*", ALL)
		check(t, "f1:v1 OR f1:v2", ALL)
		check(t, "f1:v1 AND f1:v2", NONE)
		check(t, "f1:v1 f1:v2", NONE)
		check(t, "f1:v1 f2:v2", ALL)
		check(t, "-f1:v1", NONE)
		check(t, "--f1:v1", ALL)
		check(t, ".unit:(ns/op B/op)", 0b11)
	})

	t.Run("manyUnits", func(t *testing.T) {
		res := res.Clone()
		res.Values = make([]benchfmt.Value, 100)
		for i := range res.Values {
			res.Values[i].Unit = fmt.Sprintf("u%d", i)
		}
		// Test large unit matches through all operators.
		f, err := NewFilter("f1:v1 AND --(f1:v2 OR .unit:(u0 u99))")
		if err != nil {
			t.Fatal(err)
		}
		m := f.Match(res)
		for i := 0; i < 100; i++ {
			got := m.Test(i)
			want := i == 0 || i == 99
			if got != want {
				t.Errorf("for unit u%d, got %v, want %v", i, got, want)
			}
		}
	})
}
