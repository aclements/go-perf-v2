// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math"
	"strings"

	"golang.org/x/perf/v2/benchproc"
	"golang.org/x/perf/v2/benchstat"
	"golang.org/x/perf/v2/benchunit"
)

// A DeltaCell is a Cell that visualizes the deltas between a metric
// across a sequence of phases.
type DeltaCell struct {
	row       *deltaRow
	unitClass benchunit.UnitClass

	phases []*benchproc.Config
	info   map[*benchproc.Config]deltaInfo
	layout map[*benchproc.Config]deltaBar

	maxVal float64
}

type deltaRow struct {
	maxVal float64
}

type deltaInfo struct {
	start, end, delta float64
}

type deltaBar struct {
	t, b, l, r float64
	fill       string
	neg        bool
}

func NewDeltaCells(dists []*OMap, unitClass benchunit.UnitClass) []Cell {
	row := &deltaRow{}
	cells := make([]Cell, len(dists))
	var maxVal float64
	for i, phases := range dists {
		// Compute values and deltas.
		info := make(map[*benchproc.Config]deltaInfo)
		var prev float64
		var cellMax float64
		for _, phaseCfg := range phases.Keys {
			dist := phases.Load(phaseCfg).(*benchstat.Distribution)
			info[phaseCfg] = deltaInfo{prev, dist.Center, dist.Center - prev}
			prev = dist.Center
			cellMax = math.Max(cellMax, math.Abs(dist.Center))
		}

		cells[i] = &DeltaCell{
			row:       row,
			unitClass: unitClass,
			phases:    phases.Keys,
			info:      info,
			maxVal:    cellMax,
		}

		maxVal = math.Max(maxVal, cellMax)
	}
	row.maxVal = maxVal

	// Only show deltas that are large enough to be interesting.
	// Find phases that have any delta large enough to be
	// interesting.
	thresh := maxVal * 0.05
	keepPhases := map[*benchproc.Config]bool{}
	for _, cell := range cells {
		cell := cell.(*DeltaCell)
		for _, phaseCfg := range cell.phases {
			if math.Abs(cell.info[phaseCfg].delta) >= thresh {
				keepPhases[phaseCfg] = true
			}
		}
	}
	// Filter phases.
	for _, cell := range cells {
		cell := cell.(*DeltaCell)
		var newPhases []*benchproc.Config
		for _, phaseCfg := range cell.phases {
			if keepPhases[phaseCfg] {
				newPhases = append(newPhases, phaseCfg)
			}
		}
		cell.phases = newPhases
	}

	return cells
}

func (c *DeltaCell) Extents(ext *Extents) {
	expandScale(&ext.X, 0, float64(len(c.phases)))
	expandScale(&ext.Y, 0, c.row.maxVal)

	// Make room for labels.
	ext.Margins.Bottom = 40 + labelFontHeight

	var prev *benchproc.Config
	for _, phase := range c.phases {
		ext.TopPhases.Add(prev, phase)
		prev = phase
	}
}

func (c *DeltaCell) Render(svg *SVG, scales *Scales, prev0 Cell, prevRight float64) {
	x, y := scales.X, scales.Y
	prev, _ := prev0.(*DeltaCell)

	var cross []interval
	type crossInfo struct {
		label string
		phase *benchproc.Config
	}

	// Compute bar layout where the top and bottom are absolute
	// start and end, and phases are spaced out on the X axis.
	const hMargin = 0.1
	const negStroke = 2
	layout := make(map[*benchproc.Config]deltaBar)
	for i, phaseCfg := range c.phases {
		info := c.info[phaseCfg]
		start, end := info.start, info.end
		t, b := y.Map(start), y.Map(end)
		l := x.Map(float64(i) + hMargin/2)
		r := x.Map(float64(i+1) - hMargin/2)
		fill := svgColor(scales.Colors[phaseCfg])

		// For a negative delta, draw the outline of the bar
		// instead of a solid bar.
		neg := start > end
		if neg {
			// Bring path in so stroke is entirely inside.
			t, b = b, t
			l, r = l+negStroke/2, r-negStroke/2
			t, b = t+negStroke/2, b-negStroke/2
		}

		bar := deltaBar{t, b, l, r, fill, neg}
		layout[phaseCfg] = bar

		// Prepare cross-cell delta.
		if prev != nil {
			info0, ok := prev.info[phaseCfg]
			if !ok {
				// TODO: Show +inf, but need to figure
				// out where to place it.
				continue
			}
			bar0 := prev.layout[phaseCfg]
			label := fmt.Sprintf("%+.0f%%", 100*(info.delta/info0.delta-1))
			ly := (bar0.t + bar0.b + bar.t + bar.b) / 4
			cross = append(cross, interval{ly - labelFontSize/2, ly + labelFontSize/2, crossInfo{label, phaseCfg}})
		}
	}
	c.layout = layout

	// Show cross-cell deltas.
	squiggle := func(bar deltaBar, x, y float64, west bool) {
		var sidestep = (bar.r - bar.l) / 3
		var path string
		switch {
		case y < bar.t:
			// Connect to top.
			path = fmt.Sprintf("M%f %fV%fH%f", mid(bar.l, bar.r), bar.t, y, x)
		case y > bar.b:
			// Connect to bottom.
			path = fmt.Sprintf("M%f %fV%fH%f", mid(bar.l, bar.r), bar.b, y, x)
		case west:
			// Connect to west.
			path = fmt.Sprintf("M%f %fH%fV%fH%f",
				bar.l, mid(bar.t, bar.b),
				bar.l-sidestep,
				y,
				x)
		default:
			// Connect to east.
			path = fmt.Sprintf("M%f %fH%fV%fH%f",
				bar.r, mid(bar.t, bar.b),
				bar.r+sidestep,
				y,
				x)
		}
		fmt.Fprintf(svg, `  <path d="%s" fill="none" stroke="%s" stroke-width="2px" stroke-opacity="0.5" />`+"\n", path, bar.fill)
	}
	if len(cross) != 0 {
		removeIntervalOverlaps(cross)
		x := mid(prevRight, scales.Outer.Left)
		for _, int := range cross {
			info := int.data.(crossInfo)
			lbar, rbar := prev.layout[info.phase], layout[info.phase]
			y := int.mid()
			squiggle(lbar, prevRight, y, false)
			squiggle(rbar, scales.Outer.Left, y, true)
			fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" text-anchor="middle" fill="%s" dy=".4em">%s</text>`+"\n", x, y, labelFontSize, rbar.fill, info.label)
		}
	}

	// Draw bars.
	for _, phaseCfg := range c.phases {
		info := c.info[phaseCfg]
		bar := layout[phaseCfg]

		deltaLabel := benchunit.Scale(info.delta, c.unitClass)
		if !strings.HasPrefix(deltaLabel, "-") {
			// Make it clearer this is a delta by always
			// putting a + or -.
			deltaLabel = "+" + deltaLabel
		}
		barLabel := fmt.Sprintf("%s (%s)", phaseCfg.Val(), deltaLabel)

		path := svgPathRect(bar.l, bar.t, bar.r, bar.b)
		if bar.neg {
			fmt.Fprintf(svg, `  <path d="%s" fill="none" stroke="%s" stroke-width="%d"><title>%s</title></path>`+"\n", path, bar.fill, negStroke, barLabel)
		} else {
			fmt.Fprintf(svg, `  <path d="%s" fill="%s"><title>%s</title></path>`+"\n", path, bar.fill, barLabel)
		}

		// Show delta at the end of the bar
		ly, anchor := bar.b+2, "end"
		if bar.neg {
			ly, anchor = bar.t-2, "start"
		}
		fmt.Fprintf(svg, `  <text transform="translate(%f %f) rotate(-90)" font-size="%d" text-anchor="%s" dominant-baseline="mathematical">%s</text>`+"\n", mid(bar.l, bar.r), ly, labelFontSize, anchor, deltaLabel)
	}

	// Show the peak at the very bottom.
	label := benchunit.Scale(c.maxVal, c.unitClass)
	totalY := scales.Outer.Bottom - labelFontHeight + labelFontSize
	fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" text-anchor="middle">%s</text>`+"\n", mid(scales.Outer.Left, scales.Outer.Right), totalY, labelFontSize, label)
	if prev != nil {
		// Show the delta in the peak between cells.
		fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" text-anchor="middle">%+.0f%%</text>`+"\n", mid(prevRight, scales.Outer.Left), totalY, labelFontSize, 100*(c.maxVal/prev.maxVal-1))
	}
}

func (c *DeltaCell) RenderKey(svg *SVG, x float64, lastScales *Scales) (right, bot float64) {
	y := lastScales.Y
	lastRight := lastScales.Outer.Right

	// Create initial visual intervals.
	var intervals []interval
	for _, phaseCfg := range c.phases {
		info := c.info[phaseCfg]
		inY := mid(y.Map(info.start), y.Map(info.end))
		in := interval{inY - keyFontHeight/2, inY + keyFontHeight/2, phaseCfg}
		intervals = append(intervals, in)
	}
	removeIntervalOverlaps(intervals)

	// Emit labels.
	for _, in := range intervals {
		phaseCfg := in.data.(*benchproc.Config)
		info := c.info[phaseCfg]
		label := phaseCfg.Val()
		stroke := svgColor(lastScales.Colors[phaseCfg])
		fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" dominant-baseline="central">%s</text>`+"\n", x+keyFontSize/2, in.mid(), keyFontSize, label)
		fmt.Fprintf(svg, `  <path d="%s" stroke="%s" stroke-width="2px" fill="none" />`+"\n",
			svgPathHSquiggle(
				lastRight, mid(y.Map(info.start), y.Map(info.end)),
				x, in.mid(),
			),
			stroke)
		if in.end > bot {
			bot = in.end
		}
	}

	return x + keyWidth, bot
}
