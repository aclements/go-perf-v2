// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command benchfilter reads Go benchmark results from input files,
// filters them, and writes filtered benchmark results to stdout. If
// no inputs are provided, it reads from stdin.
//
// It supports the following query syntax:
//
// 	key:regexp    - Test if key matches regexp. Key and value can be quoted.
// 	key:(x y ...) - Test if key matches any of x, y, etc.
// 	x y ...       - Test if x, y, etc. are all true
// 	x AND y       - Same as x y
// 	x OR y        - Test if x or y are true
// 	-x            - Negate x
// 	(...)         - Subexpression
//
// Keys may be one of the following:
//
// 	.name         - The base name of a benchmark
// 	.fullname     - The full name of a benchmark (including configuration)
// 	.unit         - The name of a unit for a particular metric
// 	/name-key     - Per-benchmark name configuration key
// 	file-key      - File-level configuration key
//
// Regexp matching is anchored at the beginning and end, so a literal
// string without any regexp operators must match exactly.
//
// For example, the query
//
// 	.name:Lookup goos:linux .unit:(ns/op B/op)
//
// matches benchmarks called "Lookup" with file-level configuration
// "goos" equal to "linux" and extracts just the "ns/op" and "B/op"
// measurements.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc"
)

func main() {
	log.SetPrefix("")
	log.SetFlags(0)

	flag.Usage = func() {
		// Note: Keep this in sync with the package doc.
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s query [inputs...]

benchfilter reads Go benchmark results from input files, filters them,
and writes filtered benchmark results to stdout. If no inputs are
provided, it reads from stdin.

It supports the following query syntax:

	key:regexp    - Test if key matches regexp. Key and value can be quoted.
	key:(x y ...) - Test if key matches any of x, y, etc.
	x y ...       - Test if x, y, etc. are all true
	x AND y       - Same as x y
	x OR y        - Test if x or y are true
	-x            - Negate x
	(...)         - Subexpression

Keys may be one of the following:

	.name         - The base name of a benchmark
	.fullname     - The full name of a benchmark (including configuration)
	.unit         - The name of a unit for a particular metric
	/name-key     - Per-benchmark name configuration key
	file-key      - File-level configuration key

Regexp matching is anchored at the beginning and end, so a literal
string without any regexp operators must match exactly.

For example, the query

	.name:Lookup goos:linux .unit:(ns/op B/op)

matches benchmarks called "Lookup" with file-level configuration
"goos" equal to "linux" and extracts just the "ns/op" and "B/op"
measurements.
`, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	// TODO: Consider adding filtering on values, like "@ns/op>=100".

	filter, err := benchproc.NewFilter(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	var reader benchfmt.Reader
	writer := benchfmt.NewWriter(os.Stdout)
	files := FileArgs{Args: flag.Args()[1:]}
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

			match := filter.Match(res)
			if !match.Apply(res) {
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
