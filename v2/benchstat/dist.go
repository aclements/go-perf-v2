// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchstat

import "github.com/aclements/go-moremath/stats"

type Distribution struct {
	Values []float64
	Center float64
}

type DistributionOptions struct{}

func NewDistribution(values []float64, opts DistributionOptions) *Distribution {
	samp := stats.Sample{Xs: values}
	// Speed up order statistics.
	samp.Sort()
	return &Distribution{
		Values: samp.Xs,
		Center: samp.Quantile(0.5),
	}
}

type Comparison struct {
	P float64

	Delta float64

	N1, N2 int
}

func (d *Distribution) Compare(d2 *Distribution) Comparison {
}
