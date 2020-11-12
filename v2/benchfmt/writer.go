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
	fileConfig map[string][]byte
	order      []string
}

// NewWriter returns a writer that writes Go benchmark results to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w, first: true, fileConfig: make(map[string][]byte)}
}

// Write writes benchmark result res to w. If res's file configuration
// differs from the current file configuration in w, it first emits
// the appropriate file configuration lines.
func (w *Writer) Write(res *Result) error {
	// If any file config changed, write out the changes.
	if len(w.fileConfig) != len(res.FileConfig) {
		w.writeFileConfig(res)
	} else {
		for _, cfg := range res.FileConfig {
			if val, ok := w.fileConfig[cfg.Key]; !ok || !bytes.Equal(cfg.Value, val) {
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

	// Flush the buffer out to the io.Writer. Write to the buffer
	// can't fail, so we only have to check if this fails.
	_, err := w.w.Write(w.buf.Bytes())
	w.buf.Reset()
	return err
}

func (w *Writer) writeFileConfig(res *Result) {
	if !w.first {
		// Configuration blocks after results get an extra blank.
		w.buf.WriteByte('\n')
		w.first = true
	}

	// Walk keys we know to find changes and deletions.
	for i := 0; i < len(w.order); i++ {
		key := w.order[i]
		have := w.fileConfig[key]
		idx, ok := res.FileConfigIndex(key)
		if !ok {
			// Key was deleted.
			fmt.Fprintf(&w.buf, "%s:\n", key)
			delete(w.fileConfig, key)
			copy(w.order[i:], w.order[i+1:])
			w.order = w.order[:len(w.order)-1]
			i--
			continue
		}
		if bytes.Equal(have, res.FileConfig[idx].Value) {
			// Value did not change.
			continue
		}
		// Value changed.
		cfg := &res.FileConfig[idx]
		fmt.Fprintf(&w.buf, "%s: %s\n", key, cfg.Value)
		w.fileConfig[key] = append(w.fileConfig[key][:0], cfg.Value...)
	}

	// Find new keys.
	if len(w.fileConfig) != len(res.FileConfig) {
		for _, cfg := range res.FileConfig {
			if _, ok := w.fileConfig[cfg.Key]; ok {
				continue
			}
			// New key.
			fmt.Fprintf(&w.buf, "%s: %s\n", cfg.Key, cfg.Value)
			w.fileConfig[cfg.Key] = append([]byte(nil), cfg.Value...)
			w.order = append(w.order, cfg.Key)
		}
	}

	w.buf.WriteByte('\n')
}
