// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchunit

import (
	"fmt"
	"math"
	"strconv"
)

// Scaler represents a scaling factor for a number and its scientific
// representation.
type Scaler struct {
	Prec   int     // Digits after the decimal point
	Factor float64 // Unscaled value of 1 Prefix (e.g., 1 k => 1000)
	Prefix string  // Unit prefix (SI or binary)
}

// Format formats val and appends the unit prefix according to the
// given scale.
func (s Scaler) Format(val float64) string {
	buf := make([]byte, 0, 20)
	buf = strconv.AppendFloat(buf, val/s.Factor, 'f', s.Prec, 64)
	buf = append(buf, s.Prefix...)
	return string(buf)
}

// NoOpScaler is a Scaler that formats numbers with the smallest
// number of digits necessary to capture the exact value, and no
// prefix. This is intended for when the output will be consumed by
// another program, such as when producing CSV format.
var NoOpScaler = Scaler{-1, 1, ""}

type factor struct {
	factor float64
	prefix string
	// Thresholds for 100, 10.0, 1.00.
	t100, t10, t1 float64
}

var siFactors = mkSIFactors()
var iecFactors = mkIECFactors()

func mkSIFactors() []factor {
	// To ensure that the thresholds for printing values with
	// various factors exactly match how printing itself will
	// round, we construct the thresholds by parsing the printed
	// representation.
	var factors []factor
	exp := 12
	for _, p := range []string{"T", "G", "M", "k", "", "m", "µ", "n"} {
		t100, _ := strconv.ParseFloat(fmt.Sprintf("99.95e%d", exp), 64)
		t10, _ := strconv.ParseFloat(fmt.Sprintf("9.995e%d", exp), 64)
		t1, _ := strconv.ParseFloat(fmt.Sprintf(".9995e%d", exp), 64)
		factors = append(factors, factor{math.Pow(10, float64(exp)), p, t100, t10, t1})
		exp -= 3
	}
	return factors
}

func mkIECFactors() []factor {
	var factors []factor
	exp := 40
	// ISO/IEC 80000 doesn't specify fractional prefixes, but
	// they're still meaningful for rates like B/sec. Hence, we
	// use the convention of adding a slash as in "X per unit".
	//
	// Maybe we should instead stop at 1 and format with more
	// precision?
	for _, p := range []string{"Ti", "Gi", "Mi", "Ki", "", "/Ki", "/Mi", "/Gi", "/Ti"} {
		t100, _ := strconv.ParseFloat(fmt.Sprintf("0x1.8fccccccccccdp%d", 6+exp), 64) // 99.95
		t10, _ := strconv.ParseFloat(fmt.Sprintf("0x1.3fd70a3d70a3dp%d", 3+exp), 64)  // 9.995
		t1, _ := strconv.ParseFloat(fmt.Sprintf("0x1.ffbe76c8b4396p%d", -1+exp), 64)  // .9995
		factors = append(factors, factor{math.Pow(2, float64(exp)), p, t100, t10, t1})
		exp -= 10
	}
	return factors
}

// Scale formats val using at least three significant digits,
// appending an SI or binary prefix.
func Scale(val float64, cls UnitClass) string {
	return CommonScale([]float64{val}, cls).Format(val)
}

// CommonScale returns a common Scaler to apply to all values in vals.
// This scale will show at least three significant digits for every
// value.
func CommonScale(vals []float64, cls UnitClass) Scaler {
	// The common scale is determined by the non-zero value
	// closest to zero.
	var min float64
	for _, v := range vals {
		v = math.Abs(v)
		if v != 0 && (min == 0 || v < min) {
			min = v
		}
	}
	if min == 0 {
		return Scaler{2, 1, ""}
	}

	var factors []factor
	switch cls {
	default:
		panic(fmt.Sprintf("bad UnitClass %v", cls))
	case UnitClassSI:
		factors = siFactors
	case UnitClassIEC:
		factors = iecFactors
	}

	for i, factor := range factors {
		last := i == len(factors)-1
		switch {
		case min >= factor.t100:
			return Scaler{0, factor.factor, factor.prefix}
		case min >= factor.t10:
			return Scaler{1, factor.factor, factor.prefix}
		case min >= factor.t1 || last:
			return Scaler{2, factor.factor, factor.prefix}
		}
	}
	panic("not reachable")
}
