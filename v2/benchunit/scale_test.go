// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchunit

import (
	"math"
	"testing"
)

func TestScale(t *testing.T) {
	var cls UnitClass
	test := func(num float64, want, wantPred string) {
		t.Helper()

		got := Scale(num, cls)
		if got != want {
			t.Errorf("for %v, got %s, want %s", num, got, want)
		}

		// Check what happens when this number is exactly on
		// the crux between two scale factors.
		pred := math.Nextafter(num, 0)
		got = Scale(pred, cls)
		if got != wantPred {
			dir := "-ε"
			if num < 0 {
				dir = "+ε"
			}
			t.Errorf("for %v%s, got %s, want %s", num, dir, got, wantPred)
		}
	}

	cls = UnitClassSI
	// Smoke tests
	test(0, "0.00", "0.00")
	test(1, "1.00", "1.00")
	test(-1, "-1.00", "-1.00")
	// Full range
	test(9995000000000000, "9995T", "9995T")
	test(999500000000000, "1000T", "999T")
	test(99950000000000, "100T", "99.9T")
	test(9995000000000, "10.0T", "9.99T")
	test(999500000000, "1.00T", "999G")
	test(99950000000, "100G", "99.9G")
	test(9995000000, "10.0G", "9.99G")
	test(999500000, "1.00G", "999M")
	test(99950000, "100M", "99.9M")
	test(9995000, "10.0M", "9.99M")
	test(999500, "1.00M", "999k")
	test(99950, "100k", "99.9k")
	test(9995, "10.0k", "9.99k")
	test(999.5, "1.00k", "999")
	test(99.95, "100", "99.9")
	test(9.995, "10.0", "9.99")
	test(.9995, "1.00", "999m")
	test(.09995, "100m", "99.9m")
	test(.009995, "10.0m", "9.99m")
	test(.0009995, "1.00m", "999µ")
	test(.00009995, "100µ", "99.9µ")
	test(.000009995, "10.0µ", "9.99µ")
	test(.0000009995, "1.00µ", "999n")
	test(.00000009995, "100n", "99.9n")
	test(.000000009995, "10.0n", "9.99n")
	test(.0000000009995, "1.00n", "1.00n") // First pred we won't up-scale
	// These are below the smallest scale unit. Rounding gets funnier here.
	test(math.Nextafter(.000000000995, 1), "1.00n", "0.99n")
	test(math.Nextafter(.000000000095, 1), "0.10n", "0.09n")
	test(math.Nextafter(.000000000005, 1), "0.01n", "0.00n")
	// Misc
	test(-99950000000000, "-100T", "-99.9T")
	test(-.000000009995, "-10.0n", "-9.99n")

	cls = UnitClassIEC
	// Smoke tests
	test(0, "0.00", "0.00")
	test(1, "1.00", "1.00")
	test(-1, "-1.00", "-1.00")
	// Full range
	test(.9995*(1<<50), "1023Ti", "1023Ti")
	test(99.95*(1<<40), "100Ti", "99.9Ti")
	test(9.995*(1<<40), "10.0Ti", "9.99Ti")
	test(.9995*(1<<40), "1.00Ti", "1023Gi")
	test(99.95*(1<<30), "100Gi", "99.9Gi")
	test(9.995*(1<<30), "10.0Gi", "9.99Gi")
	test(.9995*(1<<30), "1.00Gi", "1023Mi")
	test(99.95*(1<<20), "100Mi", "99.9Mi")
	test(9.995*(1<<20), "10.0Mi", "9.99Mi")
	test(.9995*(1<<20), "1.00Mi", "1023Ki")
	test(99.95*(1<<10), "100Ki", "99.9Ki")
	test(9.995*(1<<10), "10.0Ki", "9.99Ki")
	test(.9995*(1<<10), "1.00Ki", "1023")
	test(99.95*(1<<0), "100", "99.9")
	test(9.995*(1<<0), "10.0", "9.99")
	test(.9995*(1<<0), "1.00", "1023/Ki")
	test(99.95/(1<<10), "100/Ki", "99.9/Ki")
	test(9.995/(1<<10), "10.0/Ki", "9.99/Ki")
	test(.9995/(1<<10), "1.00/Ki", "1023/Mi")
	test(99.95/(1<<20), "100/Mi", "99.9/Mi")
	test(9.995/(1<<20), "10.0/Mi", "9.99/Mi")
	test(.9995/(1<<20), "1.00/Mi", "1023/Gi")
	test(99.95/(1<<30), "100/Gi", "99.9/Gi")
	test(9.995/(1<<30), "10.0/Gi", "9.99/Gi")
	test(.9995/(1<<30), "1.00/Gi", "1023/Ti")
	test(99.95/(1<<40), "100/Ti", "99.9/Ti")
	test(9.995/(1<<40), "10.0/Ti", "9.99/Ti")
	test(.9995/(1<<40), "1.00/Ti", "1.00/Ti")
}

func TestNoOpScaler(t *testing.T) {
	test := func(val float64, want string) {
		t.Helper()
		got := NoOpScaler.Format(val)
		if got != want {
			t.Errorf("for %v, got %s, want %s", val, got, want)
		}
	}

	test(1, "1")
	test(123456789, "123456789")
	test(123.456789, "123.456789")
}
