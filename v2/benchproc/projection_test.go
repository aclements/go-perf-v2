// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"testing"

	"golang.org/x/perf/v2/benchfmt"
)

func TestProjectFullName(t *testing.T) {
	cs := new(ConfigSet)
	check := func(t *testing.T, p Projection, fullName string, want string) {
		t.Helper()
		r := &benchfmt.Result{FullName: []byte(fullName)}
		cfg := p.Project(cs, r).String()
		if cfg != want {
			t.Errorf("got %s, want %s", cfg, want)
		}
	}

	t.Run("basic", func(t *testing.T) {
		p := new(ProjectFullName)
		check(t, p, "Test", ".name:Test")
		check(t, p, "Test/a=123", ".name:Test/a=123")
		check(t, p, "Test-2", ".name:Test-2")
	})

	t.Run("excludeA", func(t *testing.T) {
		p := NewProjectFullName([]string{"/a"})
		check(t, p, "Test", ".name:Test")
		check(t, p, "Test/a=123", ".name:Test/a=*")
		check(t, p, "Test/b=123/a=123", ".name:Test/b=123/a=*")
		check(t, p, "Test/a=123/b=123", ".name:Test/a=*/b=123")
		check(t, p, "Test/a=123/a=123", ".name:Test/a=*/a=*")
		check(t, p, "Test/a=123-2", ".name:Test/a=*-2")
	})

	t.Run("excludeGomaxprocs", func(t *testing.T) {
		p := NewProjectFullName([]string{"/gomaxprocs"})
		check(t, p, "Test", ".name:Test")
		check(t, p, "Test/a=123", ".name:Test/a=123")
		check(t, p, "Test/a=123-2", ".name:Test/a=123-*")
		check(t, p, "Test/gomaxprocs=123", ".name:Test/gomaxprocs=*")
	})
}

func TestProjectNameKey(t *testing.T) {
	cs := new(ConfigSet)
	check := func(t *testing.T, p Projection, fullName string, want string) {
		t.Helper()
		r := &benchfmt.Result{FullName: []byte(fullName)}
		cfg := p.Project(cs, r).String()
		if cfg != want {
			t.Errorf("got %s, want %s", cfg, want)
		}
	}

	t.Run("basic", func(t *testing.T) {
		p := &ProjectNameKey{Key: "/a"}
		check(t, p, "Test", "/a:")
		check(t, p, "Test/a=1", "/a:1")
		check(t, p, "Test/a=1/b=2", "/a:1")
		check(t, p, "Test/b=1/a=2", "/a:2")
		check(t, p, "Test/b=1/a=2-4", "/a:2")
	})

	t.Run("gomaxprocs", func(t *testing.T) {
		p := &ProjectNameKey{Key: "/gomaxprocs"}
		check(t, p, "Test", "/gomaxprocs:")
		check(t, p, "Test/gomaxprocs=4", "/gomaxprocs:4")
		check(t, p, "Test-4", "/gomaxprocs:4")
		check(t, p, "Test/a-4", "/gomaxprocs:4")
	})

	t.Run("badKey", func(t *testing.T) {
		defer func() {
			err := recover()
			if err, ok := err.(string); ok && err == "name key must being with /" {
				return
			}
			t.Fatalf("wanted panic, got %v", err)
		}()
		p := &ProjectNameKey{Key: "badKey"}
		check(t, p, "Test", "")
	})
}

func TestProjectFileConfig(t *testing.T) {
	cs := new(ConfigSet)
	check := func(t *testing.T, p Projection, want string, fileConfig ...string) {
		t.Helper()
		var r benchfmt.Result
		for i := 0; i < len(fileConfig); i += 2 {
			r.FileConfig = append(r.FileConfig, benchfmt.Config{fileConfig[i], fileConfig[i+1]})
		}
		cfg := p.Project(cs, &r).String()
		if cfg != want {
			t.Errorf("got %s, want %s", cfg, want)
		}
	}

	t.Run("basic", func(t *testing.T) {
		p := new(ProjectFileConfig)
		check(t, p, "()")
		check(t, p, "(a:1)", "a", "1")
		check(t, p, "(a:1, b:2)", "a", "1", "b", "2")
	})

	t.Run("deletion", func(t *testing.T) {
		p := new(ProjectFileConfig)
		check(t, p, "(a:1)", "a", "1")
		check(t, p, "(a:1, b:2)", "a", "1", "b", "2")
		check(t, p, "(a:1)", "a", "1", "b", "")
		// Tuples must remain schema-compatible, so the
		// deleted key reappears if there's a new key.
		check(t, p, "(a:1, b:, c:3)", "a", "1", "b", "", "c", "3")
		check(t, p, "(a:1, b:4, c:3)", "a", "1", "b", "4", "c", "3")
	})

	t.Run("reordering", func(t *testing.T) {
		p := new(ProjectFileConfig)
		check(t, p, "(a:1, b:2)", "a", "1", "b", "2")
		check(t, p, "(a:, b:3)", "b", "3")
		check(t, p, "(a:4, b:3)", "b", "3", "a", "4")
	})

	t.Run("exclude", func(t *testing.T) {
		p := NewProjectFileConfig([]string{"exc"})
		check(t, p, "(a:1)", "a", "1")
		check(t, p, "(a:1, exc:*)", "a", "1", "exc", "2")
		check(t, p, "(a:1, exc:*, b:3)", "a", "1", "exc", "2", "b", "3")
		check(t, p, "(a:1)", "a", "1", "exc", "")
		// Even if an excluded key is deleted, it needs to
		// appear in normalized form.
		check(t, p, "(a:1, exc:*, b:4)", "a", "1", "exc", "", "b", "4")
	})
}
