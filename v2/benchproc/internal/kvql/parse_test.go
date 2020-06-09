// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kvql

import "testing"

func TestParse(t *testing.T) {
	check := func(query string, want string) {
		t.Helper()
		q, err := Parse(query)
		if err != nil {
			t.Errorf("%s: unexpected error %s", query, err)
		} else if got := q.String(); got != want {
			t.Errorf("%s: got %s, want %s", query, got, want)
		}
	}
	checkErr := func(query, error string, pos int) {
		t.Helper()
		_, err := Parse(query)
		if se, _ := err.(*SyntaxError); se == nil || se.Msg != error || se.Off != pos {
			t.Errorf("%s: want error %s at %d; got %s", query, error, pos, err)
		}
	}
	check(`*`, `*`)
	check(`a:b`, `a:b`)
	checkErr(`a`, "expected key:value", 0)
	checkErr(`a :`, "missing key", 2)
	checkErr(`a:`, "expected key:value or subexpression", 0)
	checkErr(``, "nothing to match", 0)
	checkErr(`()`, "nothing to match", 1)
	checkErr(`AND`, "nothing to match", 0)
	check(`"a":"b c"`, `a:"b c"`)
	checkErr(`a "b`, "missing end quote", 2)
	check(`(a:b)`, `a:b`)
	checkErr(`(a:b`, "missing \")\"", 4)
	checkErr(`(a:b))`, "unexpected \")\"", 5)
	check(`a:b c:d e:f`, `(a:b AND c:d AND e:f)`)
	check(`-a:b`, `-a:b`)
	check(`-*`, `-*`)
	check(`a:b AND c:d`, `(a:b AND c:d)`)
	check(`-a:b AND c:d`, `(-a:b AND c:d)`)
	check(`-(a:b AND c:d)`, `-(a:b AND c:d)`)
	check(`a:b AND * AND c:d`, `(a:b AND * AND c:d)`)
	check(`a:b OR c:d`, `(a:b OR c:d)`)
	check(`a:b AND c:d OR e:f AND g:h`, `((a:b AND c:d) OR (e:f AND g:h))`)
	check(`a:b AND (c:d OR e:f) AND g:h`, `(a:b AND (c:d OR e:f) AND g:h)`)
	check(`a:(b c d)`, `(a:b OR a:c OR a:d)`)
	checkErr(`a:(b AND c)`, "expected value", 5)
	checkErr(`a:()`, "nothing to match", 3)
}
