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

// A Stack visualizes the cumulative sum of some phase metric.
//
// Each individual Stack has an independent sequence of phases, but
// the changes within a phase are visualized across the row.
type Stack struct {
	unitClass benchunit.UnitClass

	phases OMap // phase config -> stackPhase

	sum float64
}

type stackPhase struct {
	start, end float64
}

func (p stackPhase) len() float64 {
	return p.end - p.start
}

func NewStacks(dists []*OMap, unitClass benchunit.UnitClass) []Cell {
	cells := make([]Cell, len(dists))
	for i, phases := range dists {
		stack := &Stack{
			unitClass: unitClass,
		}
		var csum float64
		for _, phaseCfg := range phases.Keys {
			dist := phases.Load(phaseCfg).(*benchstat.Distribution)
			stack.phases.Store(phaseCfg, stackPhase{csum, csum + dist.Center})
			csum += dist.Center
		}
		stack.sum = csum
		cells[i] = stack
	}
	return cells
}

func (s *Stack) Extent() (xmin, xmax, ymin, ymax float64) {
	return 0, 1, 0, s.sum
}

func (s *Stack) Render(svg *SVG, x, y scale.QQ, prev Cell, prevRight float64) {
	for _, phaseCfg := range s.phases.Keys {
		phase := s.phases.Load(phaseCfg).(stackPhase)
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
			phase0, ok := prev.phases.Load(phaseCfg).(stackPhase)
			if !ok {
				continue
			}
			fmt.Fprintf(svg, `  <path d="M%f %fL%f %fV%fL%f %fz" fill="%s" fill-opacity="0.5" />`+"\n", prevRight, y.Map(phase0.start), x.Map(0), y.Map(phase.start), y.Map(phase.end), prevRight, y.Map(phase0.end), fill)
		}
	}

}

func (s *Stack) RenderKey(svg *SVG, x float64, y scale.QQ, lastRight float64) (right, bot float64) {
	const phaseFontSize = 12
	const phaseFontHeight = phaseFontSize * 5 / 4
	const phaseWidth = 150

	// Label top N phases > 1% in right-most column.
	const numPhases = 15
	const minPhaseFrac = 0.01
	var topPhases []*benchproc.Config
	for _, cfg := range s.phases.Keys {
		phase := s.phases.Load(cfg).(stackPhase)
		if phase.len() < s.sum*minPhaseFrac {
			continue
		}
		topPhases = append(topPhases, cfg)
	}
	// Get the top N phases.
	sort.Slice(topPhases, func(i, j int) bool {
		p1 := s.phases.Load(topPhases[i]).(stackPhase)
		p2 := s.phases.Load(topPhases[j]).(stackPhase)
		return p1.len() > p2.len()
	})
	if len(topPhases) > numPhases {
		topPhases = topPhases[:numPhases]
	}
	// Sort back into phase order.
	sort.Slice(topPhases, func(i, j int) bool {
		order := s.phases.KeyPos
		return order[topPhases[i]] < order[topPhases[j]]
	})
	// Create initial visual intervals.
	intervals := make([]interval, len(topPhases))
	for i := range intervals {
		phase := s.phases.Load(topPhases[i]).(stackPhase)
		mid := (y.Map(phase.start) + y.Map(phase.end)) / 2
		intervals[i] = interval{mid - phaseFontHeight/2, mid + phaseFontHeight/2}
	}
	// Slide intervals to remove overlaps.
	removeIntervalOverlaps(intervals)
	// Emit labels
	for i, phaseCfg := range topPhases {
		phase := s.phases.Load(phaseCfg).(stackPhase)
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
