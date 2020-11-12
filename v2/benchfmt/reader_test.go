// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func parseAll(t *testing.T, data string, setup ...func(r *Reader)) []*Result {
	sr := strings.NewReader(data)
	r := NewReader(sr, "test")
	for _, f := range setup {
		f(r)
	}
	var out []*Result
	for r.Scan() {
		res, err := r.Result()
		if err == nil {
			out = append(out, res.Clone())
		} else {
			out = append(out, errResult(err.Error()))
		}
	}
	if err := r.Err(); err != nil {
		t.Fatal("parsing failed: ", err)
	}
	return out
}

func printResult(w io.Writer, r *Result) {
	for _, fc := range r.FileConfig {
		fmt.Fprintf(w, "{%s: %s} ", fc.Key, fc.Value)
	}
	fmt.Fprintf(w, "%s %d", r.FullName, r.Iters)
	for _, val := range r.Values {
		fmt.Fprintf(w, " %v %s", val.Value, val.Unit)
	}
	fmt.Fprintf(w, "\n")
}

// errResult returns a result that captures an error message. This is
// just a convenience for testing.
func errResult(msg string) *Result {
	return &Result{FullName: []byte("error: " + msg)}
}

func r(cfg []Config, fullName string, iters int, vals []Value) *Result {
	return &Result{
		FileConfig: cfg,
		FullName:   []byte(fullName),
		Iters:      iters,
		Values:     vals,
	}
}

func TestReader(t *testing.T) {
	type testCase struct {
		name, input string
		want        []*Result
	}
	for _, test := range []testCase{
		{
			"basic",
			`key: value
BenchmarkOne 100 1 ns/op 2 B/op
BenchmarkTwo 300 4.5 ns/op
`,
			[]*Result{r(
				[]Config{{"key", []byte("value")}},
				"One",
				100,
				[]Value{{1, "ns/op"}, {2, "B/op"}},
			), r(
				[]Config{{"key", []byte("value")}},
				"Two",
				300,
				[]Value{{4.5, "ns/op"}},
			)},
		},
		{
			"weird",
			`
BenchmarkSpaces    1   1   ns/op
BenchmarkHugeVal 1 9999999999999999999999999999999 ns/op
BenchmarkEmSpace  1  1  ns/op
`,
			[]*Result{r(
				[]Config{},
				"Spaces",
				1,
				[]Value{{1, "ns/op"}},
			), r(
				[]Config{},
				"HugeVal",
				1,
				[]Value{{9999999999999999999999999999999, "ns/op"}},
			), r(
				[]Config{},
				"EmSpace",
				1,
				[]Value{{1, "ns/op"}},
			)},
		},
		{
			"basic file keys",
			`key1:    	 value
: not a key
ab:not a key
a b: also not a key
key2: value

BenchmarkOne 100 1 ns/op
`,
			[]*Result{r(
				[]Config{{"key1", []byte("value")}, {"key2", []byte("value")}},
				"One",
				100,
				[]Value{{1, "ns/op"}},
			)},
		},
		{
			"bad lines",
			`not a benchmark
BenchmarkMissingIter
BenchmarkBadIter abc
BenchmarkHugeIter 9999999999999999999999999999999
BenchmarkMissingVal 100
BenchmarkBadVal 100 abc
BenchmarkMissingUnit 100 1
BenchmarkMissingUnit2 100 1 ns/op 2
also not a benchmark
`,
			[]*Result{
				errResult("test:2: missing iteration count"),
				errResult("test:3: parsing iteration count: invalid syntax"),
				errResult("test:4: parsing iteration count: value out of range"),
				errResult("test:5: missing measurements"),
				errResult("test:6: parsing measurement: invalid syntax"),
				errResult("test:7: missing units"),
				errResult("test:8: missing units"),
			},
		},
		{
			"remove existing label",
			`key: value
key:
BenchmarkOne 100 1 ns/op
`,
			[]*Result{r(
				[]Config{},
				"One",
				100,
				[]Value{{1, "ns/op"}},
			)},
		},
		{
			"overwrite exiting label",
			`key1: first
key2: second
key1: third
BenchmarkOne 100 1 ns/op
`,
			[]*Result{r(
				[]Config{{"key1", []byte("third")}, {"key2", []byte("second")}},
				"One",
				100,
				[]Value{{1, "ns/op"}},
			)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := parseAll(t, test.input)
			want := test.want
			var diff bytes.Buffer
			for i := 0; i < len(got) || i < len(want); i++ {
				if i >= len(got) {
					fmt.Fprintf(&diff, "[%d] got: none, want:\n", i)
					printResult(&diff, want[i])
				} else if i >= len(want) {
					fmt.Fprintf(&diff, "[%d] want: none, got:\n", i)
					printResult(&diff, got[i])
				} else if !reflect.DeepEqual(got[i], want[i]) {
					fmt.Fprintf(&diff, "[%d] got:\n", i)
					printResult(&diff, got[i])
					fmt.Fprintf(&diff, "[%d] want:\n", i)
					printResult(&diff, want[i])
				}
			}
			if diff.Len() != 0 {
				t.Error(diff.String())
			}
		})
	}
}

func BenchmarkReader(b *testing.B) {
	path := "testdata/bent"
	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		b.Fatal("reading test data directory: ", err)
	}

	var files []*os.File
	for _, info := range fileInfos {
		f, err := os.Open(filepath.Join(path, info.Name()))
		if err != nil {
			b.Fatal(err)
		}
		defer f.Close()
		files = append(files, f)
	}

	b.ResetTimer()

	start := time.Now()
	var n int
	for i := 0; i < b.N; i++ {
		r := new(Reader)
		for _, f := range files {
			if _, err := f.Seek(0, 0); err != nil {
				b.Fatal("seeking to 0: ", err)
			}
			r.Reset(f, f.Name())
			for r.Scan() {
				n++
				if _, err := r.Result(); err != nil {
					b.Fatal("malformed record: ", err)
				}
			}
			if err := r.Err(); err != nil {
				b.Fatal(err)
			}
		}
	}
	dur := time.Since(start)
	b.Logf("read %d records", n)

	b.StopTimer()
	b.ReportMetric(float64(n/b.N), "records/op")
	b.ReportMetric(float64(n)*float64(time.Second)/float64(dur), "records/sec")
}
