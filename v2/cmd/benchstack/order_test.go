// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"golang.org/x/perf/v2/benchproc"
)

func TestGlobalOrder(t *testing.T) {
	cs := new(benchproc.ConfigSet)
	strToSeq := func(str string) []*benchproc.Config {
		var seq []*benchproc.Config
		for i := 0; i < len(str); i++ {
			seq = append(seq, cs.KeyVal("", str[i:i+1]))
		}
		return seq
	}
	seqToStr := func(seq []*benchproc.Config) string {
		str := ""
		for _, cfg := range seq {
			str += cfg.Val()
		}
		return str
	}
	test := func(local []string, want string) {
		t.Helper()
		localCfgs := make([][]*benchproc.Config, len(local))
		for i, l := range local {
			localCfgs[i] = strToSeq(l)
		}
		global := seqToStr(globalOrder(localCfgs))
		if global != want {
			t.Errorf("for local order %v, got %s, want %s", local, global, want)
		}
	}

	// Trivial cases.
	test([]string{"abcd"}, "abcd")
	test([]string{"abcd", "abcd"}, "abcd")
	test([]string{"", "abcd"}, "abcd")
	test([]string{"abcd", ""}, "abcd")
	// Simple insertion.
	test([]string{"az", "abz"}, "abz")
	// Order changes.
	test([]string{"acbd", "abcd"}, "abcd")
	// Appending and prepending.
	test([]string{"xyza", "abc", "a"}, "xyzabc")
	// Diamond.
	test([]string{"abd", "acd"}, "abcd")
	// Initially a diamond, then constrained.
	test([]string{"abcd", "abd", "acd"}, "abcd")
	test([]string{"acbd", "abd", "acd"}, "acbd")
	// Cycle.
	test([]string{"cda", "abc"}, "abcd")
}
