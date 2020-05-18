// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/perf/v2/benchfmt"
)

func main() {
	var units IncludeExclude

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [flags] [inputs...]

Read Go benchmark results from input files, filter them, and write
filtered benchmark results to stdout. If no inputs are provided, read
from stdin.

Flags:
`, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Var(&units, "units", "comma-separated `list` of units to include or, when prefixed with '!', exclude")
	flag.Parse()

	// TODO: Filtering on file keys and benchmark names and keys.

	var reader benchfmt.Reader
	writer := benchfmt.NewWriter(os.Stdout)
	files := FileArgs{Args: flag.Args()}
	for {
		f, err := files.Next()
		if err != nil {
			log.Fatal(err)
		}
		if f == nil {
			break
		}

		reader.Reset(f, f.Name())
		for reader.Scan() {
			res, err := reader.Result()
			if err != nil {
				// Non-fatal result parse error. Warn
				// but keep going.
				fmt.Fprintln(os.Stderr, err)
				continue
			}

			if !filterUnits(res, &units) {
				continue
			}

			err = writer.Write(res)
			if err != nil {
				log.Fatal("writing output: ", err)
			}
		}
		if err := reader.Err(); err != nil {
			log.Fatal(err)
		}
	}
}

func filterUnits(res *benchfmt.Result, keep *IncludeExclude) bool {
	j := 0
	for _, val := range res.Values {
		if keep.Match(val.Unit) {
			res.Values[j] = val
			j++
		}
	}
	res.Values = res.Values[:j]
	return len(res.Values) > 0
}

type FileArgs struct {
	Args []string

	next int
	f    *os.File
}

func (fa *FileArgs) Next() (*os.File, error) {
	if fa.f != nil {
		err := fa.f.Close()
		fa.f = nil
		if err != nil {
			return nil, err
		}
	}

	if fa.next >= len(fa.Args) {
		if fa.next == 0 {
			fa.next++
			return os.Stdin, nil
		}
		return nil, nil
	}

	f, err := os.Open(fa.Args[fa.next])
	if err != nil {
		return nil, err
	}
	fa.next++
	fa.f = f
	return f, nil
}

type IncludeExclude struct {
	patterns []string
}

func (ie *IncludeExclude) Set(arg string) error {
	parts := strings.Split(arg, ",")
	// TODO: Support globs (but path.Match treats / specially)
	ie.patterns = append(ie.patterns, parts...)
	return nil
}

func (ie *IncludeExclude) String() string {
	return strings.Join(ie.patterns, ",")
}

func (ie *IncludeExclude) Match(val string) bool {
	if len(ie.patterns) == 0 {
		return true
	}
	match := strings.HasPrefix(ie.patterns[0], "!")
	for _, pat := range ie.patterns {
		if strings.HasPrefix(pat, "!") && pat[1:] == val {
			match = false
		} else if pat == val {
			match = true
		}
	}
	return match
}
