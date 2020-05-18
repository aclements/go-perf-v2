// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import (
	"bytes"
	"fmt"
	"io"
)

// A Writer writes the Go benchmark format.
type Writer struct {
	w   io.Writer
	buf bytes.Buffer

	first      bool
	fileConfig []Config
}

// NewWriter returns a writer that writes Go benchmark results to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w, first: true}
}

// Write writes benchmark result res to w. If res's file configuration
// differs from the current file configuration in w, it first emits
// the appropriate file configuration lines.
func (w *Writer) Write(res *Result) error {
	// Do we need to change file config?
	if len(w.fileConfig) != len(res.FileConfig) {
		w.writeFileConfig(res)
	} else {
		for i, fc := range res.FileConfig {
			if fc != w.fileConfig[i] {
				w.writeFileConfig(res)
				break
			}
		}
	}

	// Print the benchmark line.
	fmt.Fprintf(&w.buf, "Benchmark%s %d", res.FullName, res.Iters)
	for _, val := range res.Values {
		fmt.Fprintf(&w.buf, " %v %s", val.Value, val.Unit)
	}
	w.buf.WriteByte('\n')

	w.first = false

	_, err := w.w.Write(w.buf.Bytes())
	w.buf.Reset()
	return err
}

func (w *Writer) writeFileConfig(res *Result) {
	if !w.first {
		w.buf.WriteByte('\n')
	}

	for i, fc := range res.FileConfig {
		if i >= len(w.fileConfig) {
			w.fileConfig = append(w.fileConfig, fc)
		} else if w.fileConfig[i] != fc {
			w.fileConfig[i] = fc
		} else {
			continue
		}
		fmt.Fprintf(&w.buf, "%s: %s\n", fc.Key, fc.Value)
	}

	w.buf.WriteByte('\n')
}
