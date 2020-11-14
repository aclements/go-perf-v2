// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import (
	"fmt"
	"reflect"
	"testing"
)

func TestResultSetFileConfig(t *testing.T) {
	r := &Result{}
	check := func(want ...string) {
		t.Helper()
		var kv []string
		for _, cfg := range r.FileConfig {
			kv = append(kv, fmt.Sprintf("%s: %s", cfg.Key, cfg.Value))
		}
		if !reflect.DeepEqual(want, kv) {
			t.Errorf("want %q, got %q", want, kv)
		}

		// Check the index.
		for i, cfg := range r.FileConfig {
			gotI, ok := r.FileConfigIndex(string(cfg.Key))
			if !ok {
				t.Errorf("key %s missing from index", cfg.Key)
			} else if i != gotI {
				t.Errorf("key %s index: want %d, got %d", cfg.Key, i, gotI)
			}
		}
		if len(r.FileConfig) != len(r.configPos) {
			t.Errorf("index size mismatch: %d file configs, %d index length", len(r.FileConfig), len(r.configPos))
		}
	}

	// Basic key additions.
	check()
	r.SetFileConfig("a", "b")
	check("a: b")
	r.SetFileConfig("x", "y")
	check("a: b", "x: y")
	r.SetFileConfig("z", "w")
	check("a: b", "x: y", "z: w")

	// Update value.
	r.SetFileConfig("x", "z")
	check("a: b", "x: z", "z: w")

	// Delete key.
	r.SetFileConfig("a", "") // Check swapping
	check("z: w", "x: z")
	r.SetFileConfig("x", "") // Last key
	check("z: w")
	r.SetFileConfig("c", "") // Non-existent
	check("z: w")

	// Add key after deletion.
	r.SetFileConfig("c", "d")
	check("z: w", "c: d")
}

func TestResultGetFileConfig(t *testing.T) {
	r := &Result{}
	check := func(key, want string) {
		t.Helper()
		got := r.GetFileConfig(key)
		if want != got {
			t.Errorf("for key %s: want %s, got %s", key, want, got)
		}
	}
	check("x", "")
	r.SetFileConfig("x", "y")
	check("x", "y")
	r.SetFileConfig("a", "b")
	check("a", "b")
	check("x", "y")
	r.SetFileConfig("a", "")
	check("a", "")
	check("x", "y")

	// Test a literal.
	r = &Result{
		FileConfig: []Config{{"a", []byte("b")}},
	}
	check("a", "b")
	check("x", "")
}

func TestResultValue(t *testing.T) {
	r := &Result{
		Values: []Value{{42, "ns/op"}, {24, "B/op"}},
	}
	check := func(unit string, want float64) {
		t.Helper()
		got, ok := r.Value(unit)
		if !ok {
			t.Errorf("missing unit %s", unit)
		} else if want != got {
			t.Errorf("for unit %s: want %v, got %v", unit, want, got)
		}
	}
	check("ns/op", 42)
	check("B/op", 24)
	_, ok := r.Value("B/sec")
	if ok {
		t.Errorf("unexpectedly found unit %s", "B/sec")
	}
}

func TestBaseName(t *testing.T) {
	check := func(fullName string, want string) {
		t.Helper()
		got := string(BaseName([]byte(fullName)))
		if got != want {
			t.Errorf("BaseName(%q) = %q, want %q", fullName, got, want)
		}
	}
	check("Test", "Test")
	check("Test-42", "Test")
	check("Test/foo", "Test")
	check("Test/foo-42", "Test")
}

func TestNameParts(t *testing.T) {
	check := func(fullName string, base string, parts ...string) {
		t.Helper()
		got, gotParts := NameParts([]byte(fullName))
		fail := string(got) != string(base)
		if len(gotParts) != len(parts) {
			fail = true
		} else {
			for i := range parts {
				if parts[i] != string(gotParts[i]) {
					fail = true
				}
			}
		}
		if fail {
			t.Errorf("FullName(%q) = %q, %q, want %q, %q", fullName, got, gotParts, base, parts)
		}
	}
	check("Test", "Test")
	// Gomaxprocs
	check("Test-42", "Test", "-42")
	// Subtests
	check("Test/foo", "Test", "/foo")
	check("Test/foo=42/bar=24", "Test", "/foo=42", "/bar=24")
	// Both
	check("Test/foo=123-42", "Test", "/foo=123", "-42")
	// Looks like gomaxprocs, but isn't
	check("Test/foo-bar", "Test", "/foo-bar")
	check("Test/foo-1/bar", "Test", "/foo-1", "/bar")
	// Trailing slash
	check("Test/foo/", "Test", "/foo", "/")
	// Empty name
	check("", "")
	check("/a/b", "", "/a", "/b")
}
