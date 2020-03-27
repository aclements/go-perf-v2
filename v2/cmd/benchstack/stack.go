// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"

	"github.com/aclements/go-moremath/scale"
	"golang.org/x/perf/v2/benchproc"
	"golang.org/x/perf/v2/benchstat"
	"golang.org/x/perf/v2/benchunit"
)

// A StackRow is a Row of Stack visualizations.
//
// Each individual Stack has an independent sequence of phases, but
// the changes within a phase are visualized across the row.
type StackRow struct {
	unitClass benchunit.UnitClass
	stacks    map[*benchproc.Config]*Stack // col config
}

// A Stack visualizes the cumulative sum of some phase metric and how
// it varies across columns.
type Stack struct {
	unitClass benchunit.UnitClass

	phases OMap // phase config -> []float64 measurements

	csum map[*benchproc.Config]stackPhase
	sum  float64
}

type stackPhase struct {
	start, end float64
}

func (p stackPhase) len() float64 {
	return p.end - p.start
}

func NewStackRow(unitClass benchunit.UnitClass) *StackRow {
	return &StackRow{
		unitClass: unitClass,
		stacks:    make(map[*benchproc.Config]*Stack),
	}
}

func (r *StackRow) Add(colCfg, phaseCfg *benchproc.Config, val float64) {
	stack, ok := r.stacks[colCfg]
	if !ok {
		stack = &Stack{
			unitClass: r.unitClass,
			phases:    OMap{New: func(*benchproc.Config) interface{} { return ([]float64)(nil) }},
		}
		r.stacks[colCfg] = stack
	}

	stack.phases.Store(phaseCfg, append(stack.phases.LoadOrNew(phaseCfg).([]float64), val))
}

func (r *StackRow) Finish(cols []*benchproc.Config) []Cell {
	cells := make([]Cell, len(cols))

	for colI, cfg := range cols {
		stack := r.stacks[cfg]
		if stack == nil {
			continue
		}
		cells[colI] = stack

		// Finalize distribution.
		stack.csum = make(map[*benchproc.Config]stackPhase)
		var csum float64
		for _, cfg := range stack.phases.Keys {
			dist := benchstat.NewDistribution(stack.phases.Load(cfg).([]float64), benchstat.DistributionOptions{})
			stack.csum[cfg] = stackPhase{csum, csum + dist.Center}
			csum += dist.Center
		}
		stack.sum = csum
	}

	return cells
}

func (s *Stack) Extent() (xmin, xmax, ymin, ymax float64) {
	return 0, 1, 0, s.sum
}

func (s *Stack) Render(svg *SVG, x, y scale.QQ, prev Cell, prevRight float64) {
	for _, phaseCfg := range s.phases.Keys {
		phase := s.csum[phaseCfg]
		fill := svg.PhaseColor(phaseCfg)
		title := phaseCfg.Val()

		// Draw rectangle for this phase.
		path := fmt.Sprintf("M%f %fH%fV%fH%fz", x.Map(0), y.Map(phase.start), x.Map(1), y.Map(phase.end), x.Map(0))
		fmt.Fprintf(svg, `  <path d="%s" fill="%s"><title>%s (%s)</title></path>`+"\n", path, fill, title, benchunit.Scale(phase.len(), s.unitClass))

		// Phase label.
		clipID := svg.GenID("clip")
		fmt.Fprintf(svg, `  <clipPath id="%s"><path d="%s" /></clipPath>`+"\n", clipID, path)
		fmt.Fprintf(svg, `  <text x="%f" y="%f" clip-path="url(#%s)" font-size="%d" text-anchor="middle" dy=".4em">%s (%.0f%%)</text>`+"\n", x.Map(0.5), (y.Map(phase.start)+y.Map(phase.end))/2, clipID, labelFontSize, benchunit.Scale(phase.len(), s.unitClass), 100*phase.len()/s.sum)

		// Connect to phase in previous column.
		if prev, ok := prev.(*Stack); ok {
			phase0, ok := prev.csum[phaseCfg]
			if !ok {
				continue
			}
			fmt.Fprintf(svg, `  <path d="M%f %fL%f %fV%fL%f %fz" fill="%s" fill-opacity="0.5" />`+"\n", prevRight, y.Map(phase0.start), x.Map(0), y.Map(phase.start), y.Map(phase.end), prevRight, y.Map(phase0.end), fill)
		}
	}

}

func (r *StackRow) RenderKey(svg *SVG, x float64, y scale.QQ, last Cell, lastRight float64) (right, bot float64) {
	const phaseFontSize = 12
	const phaseFontHeight = phaseFontSize * 5 / 4
	const phaseWidth = 150

	lastStack := last.(*Stack)

	// Label top N phases > 1% in right-most column.
	const numPhases = 15
	const minPhaseFrac = 0.01
	var topPhases []*benchproc.Config
	for cfg, phase := range lastStack.csum {
		if phase.len() < lastStack.sum*minPhaseFrac {
			continue
		}
		topPhases = append(topPhases, cfg)
	}
	// Get the top N phases.
	sort.Slice(topPhases, func(i, j int) bool {
		p1 := lastStack.csum[topPhases[i]]
		p2 := lastStack.csum[topPhases[j]]
		return p1.len() > p2.len()
	})
	if len(topPhases) > numPhases {
		topPhases = topPhases[:numPhases]
	}
	// Sort back into phase order.
	sort.Slice(topPhases, func(i, j int) bool {
		order := lastStack.phases.KeyPos
		return order[topPhases[i]] < order[topPhases[j]]
	})
	// Create initial visual intervals.
	intervals := make([]interval, len(topPhases))
	for i := range intervals {
		phase := lastStack.csum[topPhases[i]]
		mid := (y.Map(phase.start) + y.Map(phase.end)) / 2
		intervals[i] = interval{mid - phaseFontHeight/2, mid + phaseFontHeight/2}
	}
	// Slide intervals to remove overlaps.
	removeIntervalOverlaps(intervals)
	// Emit labels
	for i, phaseCfg := range topPhases {
		phase := lastStack.csum[phaseCfg]
		label := phaseCfg.Val()
		in := intervals[i]
		stroke := svg.PhaseColor(phaseCfg)
		fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" dominant-baseline="central">%s</text>`+"\n", x+phaseFontSize/2, in.mid(), phaseFontSize, label)
		fmt.Fprintf(svg, `  <path d="M%f %fC%f %f,%f %f,%f %f" stroke="%s" stroke-width="2px" />`+"\n",
			lastRight, mid(y.Map(phase.start), y.Map(phase.end)),
			mid(x, lastRight), mid(y.Map(phase.start), y.Map(phase.end)),
			mid(x, lastRight), in.mid(),
			x, in.mid(),
			stroke)
		if in.end > bot {
			bot = in.end
		}
	}
	right = x + phaseWidth

	return
}
