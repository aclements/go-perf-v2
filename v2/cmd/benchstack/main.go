// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc"
	"golang.org/x/perf/v2/benchstat"
	"golang.org/x/perf/v2/benchunit"
)

// Qualitative palettes from Color Brewer.
var Pastel1_9 = []color.Color{color.RGBA{251, 180, 174, 255}, color.RGBA{179, 205, 227, 255}, color.RGBA{204, 235, 197, 255}, color.RGBA{222, 203, 228, 255}, color.RGBA{254, 217, 166, 255}, color.RGBA{255, 255, 204, 255}, color.RGBA{229, 216, 189, 255}, color.RGBA{253, 218, 236, 255}, color.RGBA{242, 242, 242, 255}}
var Accent_8 = []color.Color{color.RGBA{127, 201, 127, 255}, color.RGBA{190, 174, 212, 255}, color.RGBA{253, 192, 134, 255}, color.RGBA{255, 255, 153, 255}, color.RGBA{56, 108, 176, 255}, color.RGBA{240, 2, 127, 255}, color.RGBA{191, 91, 23, 255}, color.RGBA{102, 102, 102, 255}}
var Dark2_8 = []color.Color{color.RGBA{27, 158, 119, 255}, color.RGBA{217, 95, 2, 255}, color.RGBA{117, 112, 179, 255}, color.RGBA{231, 41, 138, 255}, color.RGBA{102, 166, 30, 255}, color.RGBA{230, 171, 2, 255}, color.RGBA{166, 118, 29, 255}, color.RGBA{102, 102, 102, 255}}
var Set1_9 = []color.Color{color.RGBA{228, 26, 28, 255}, color.RGBA{55, 126, 184, 255}, color.RGBA{77, 175, 74, 255}, color.RGBA{152, 78, 163, 255}, color.RGBA{255, 127, 0, 255}, color.RGBA{255, 255, 51, 255}, color.RGBA{166, 86, 40, 255}, color.RGBA{247, 129, 191, 255}, color.RGBA{153, 153, 153, 255}}

var pal = Set1_9

type col struct {
	phases OMap // phase config -> measurements

	csum map[*benchproc.Config]phase
	sum  float64
}

type phase struct {
	start, end float64
}

func (p phase) len() float64 {
	return p.end - p.start
}

func newCol() *col {
	return &col{phases: OMap{New: func(*benchproc.Config) interface{} { return ([]float64)(nil) }}}
}

type unitInfo struct {
	cfg        *benchproc.Config
	tidyUnit   string
	tidyFactor float64
	class      benchunit.UnitClass
}

func main() {
	flagCol := flag.String("col", "benchmark,branch,commit-date", "split columns by distinct values of `projection`")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	cs := new(benchproc.ConfigSet)

	colBys, err := benchproc.ParseProjectionBundle([]string{*flagCol}, benchproc.ParseOpts{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing -col: %s", err)
		os.Exit(1)
	}
	colBy := colBys[0]
	phaseBy := &benchproc.ProjectFullName{}
	var phaseTracker OMap

	// Map from (unit, colBy) -> *col
	cols := OMap2D{
		New: func(_, _ *benchproc.Config) interface{} {
			return newCol()
		},
	}

	// XXX Take this as an argument?
	units := make(map[string]unitInfo)
	var tidier benchunit.Tidier
	for _, unit := range []string{"ns/op", "B/op"} {
		cfg := cs.KeyVal(".unit", unit)
		tidyUnit, tidyFactor := tidier.Tidy(unit)
		unitClass := benchunit.UnitClassOf(tidyUnit)
		units[unit] = unitInfo{cfg, tidyUnit, tidyFactor, unitClass}
	}

	// Parse into data.
	var reader benchfmt.Reader
	for _, file := range flag.Args() {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}
		reader.Reset(f, file)
		for reader.Scan() {
			res, err := reader.Result()
			if err != nil {
				log.Print(err)
				continue
			}

			// TODO: Nicer filtering
			if !strings.HasSuffix(string(res.FullName), "_GC") || string(res.FullName) == "TotalTime_GC" {
				continue
			}
			// if strings.HasSuffix(string(res.FullName), "_GC") || string(res.FullName) == "TotalTime" {
			// 	continue
			// }

			colCfg := colBy.Project(cs, res)
			phaseCfg := phaseBy.Project(cs, res)
			phaseTracker.Store(phaseCfg, nil)

			for _, value := range res.Values {
				unitInfo, ok := units[value.Unit]
				if !ok {
					continue
				}
				val := value.Value * unitInfo.tidyFactor

				col := cols.LoadOrNew(unitInfo.cfg, colCfg).(*col)
				col.phases.Store(phaseCfg, append(col.phases.LoadOrNew(phaseCfg).([]float64), val))
			}
		}
		if err := reader.Err(); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	if len(cols.Rows) == 0 {
		log.Fatal("no data")
	}

	// Finalize distributions.
	maxSum := make(map[string]float64)
	cols.Map(func(unitCfg, _ *benchproc.Config, col1 interface{}) interface{} {
		col := col1.(*col)
		col.csum = make(map[*benchproc.Config]phase)
		var csum float64
		for _, cfg := range col.phases.Keys {
			dist := benchstat.NewDistribution(col.phases.Load(cfg).([]float64), benchstat.DistributionOptions{})
			col.csum[cfg] = phase{csum, csum + dist.Center}
			csum += dist.Center
		}
		col.sum = csum

		unit := unitCfg.Val()
		if csum > maxSum[unit] {
			maxSum[unit] = csum
		}

		return col
	})

	// Assign colors to phases.
	colors := make(map[*benchproc.Config]color.Color)
	for i, cfg := range phaseTracker.Keys {
		colors[cfg] = pal[i%len(pal)]
	}

	// Emit SVG
	svg := new(bytes.Buffer)
	const unitFontSize = 12
	const unitFontHeight = 12 * 5 / 4
	const colWidth = 100
	const colSpace = 50
	const colFontSize = 12
	const colFontHeight = 12 * 5 / 4
	const rowHeight = 300
	const rowGap = 10
	const labelFontSize = 8
	const phaseWidth = 150
	var eltID int
	x := func(col int) (float64, float64) {
		l := unitFontHeight + col*(colWidth+colSpace)
		return float64(l), float64(l + colWidth)
	}

	// Column labels
	var topSpace float64
	colTree, _ := benchproc.NewConfigTree(cols.Cols)
	var walkColTree func(tree *benchproc.ConfigTree, rowI, colI int)
	walkColTree = func(tree *benchproc.ConfigTree, rowI, colI int) {
		if tree.Config != nil {
			l, _ := x(colI)
			_, r := x(colI + tree.Width - 1)
			label := tree.Config.Val()
			fmt.Fprintf(svg, `  <text x="%f" y="%d" font-size="%d" text-anchor="middle">%s</text>`+"\n", (l+r)/2, rowI*colFontHeight+colFontSize, colFontSize, label)
			// Emit grouping bar (except at the bottom)
			if tree.Children != nil {
				fmt.Fprintf(svg, `  <path d="M%f %fH%f" stroke="black" stroke-width="1px" />`+"\n", l, float64(rowI+1)*colFontHeight, r)
			}
			if bot := colFontHeight * float64(1+rowI); bot > topSpace {
				topSpace = bot
			}
		}
		for _, child := range tree.Children {
			walkColTree(child, rowI+1, colI)
			colI += child.Width
		}
	}
	colI := 0
	for _, tree := range colTree {
		walkColTree(tree, 0, colI)
		colI += tree.Width
	}
	bot := topSpace

	// Unit rows
	var maxRight float64
	for rowI, unitCfg := range cols.Rows {
		top := topSpace + float64(rowI*(rowHeight+rowGap))
		if top+rowHeight > bot {
			bot = top + rowHeight
		}

		unit := unitCfg.Val()
		unitInfo := units[unit]
		valToPix := rowHeight / maxSum[unit]
		y := func(val float64) float64 {
			return top + val*valToPix
		}

		// Unit label
		fmt.Fprintf(svg, `  <text font-size="%d" text-anchor="middle" transform="translate(%f %f) rotate(-90)">%s</text>`+"\n", unitFontSize, float64(unitFontSize), float64(top+rowHeight/2), unitInfo.tidyUnit)

		// Phase bars
		var prevCol *col
		for colI, colCfg := range cols.Cols {
			colX, _ := cols.Load(unitCfg, colCfg)
			if colX == nil {
				continue
			}
			col := colX.(*col)
			l, _ := x(colI)
			for _, phaseCfg := range col.phases.Keys {
				phase := col.csum[phaseCfg]
				fill := svgColor(colors[phaseCfg])
				title := phaseCfg.Val()

				// Draw rectangle for this phase.
				path := fmt.Sprintf("M%f %fh%dV%fh%dz", l, y(phase.start), colWidth, y(phase.end), -colWidth)
				fmt.Fprintf(svg, `  <path d="%s" fill="%s"><title>%s (%s)</title></path>`+"\n", path, fill, title, benchunit.Scale(phase.len(), unitInfo.class))

				// Phase label.
				clipID := fmt.Sprintf("clip%d", eltID)
				eltID++
				fmt.Fprintf(svg, `  <clipPath id="%s"><path d="%s" /></clipPath>`+"\n", clipID, path)
				fmt.Fprintf(svg, `  <text x="%f" y="%f" clip-path="url(#%s)" font-size="%d" text-anchor="middle" dy=".4em">%s (%.0f%%)</text>`+"\n", l+colWidth/2, (y(phase.start)+y(phase.end))/2, clipID, labelFontSize, benchunit.Scale(phase.len(), unitInfo.class), 100*phase.len()/col.sum)

				// Connect to phase in previous column.
				if prevCol != nil {
					phase0, ok := prevCol.csum[phaseCfg]
					if !ok {
						continue
					}
					_, pr := x(colI - 1)
					fmt.Fprintf(svg, `  <path d="M%f %fL%f %fV%fL%f %fz" fill="%s" fill-opacity="0.5" />`+"\n", pr, y(phase0.start), l, y(phase.start), y(phase.end), pr, y(phase0.end), fill)
				}
			}
			prevCol = col
		}
		_, right := x(len(cols.Cols) - 1)

		// Label top N phases > 1% in right-most column.
		const numPhases = 15
		const minPhaseFrac = 0.01
		const phaseFontSize = colFontSize
		const phaseFontHeight = phaseFontSize * 5 / 4
		var topPhases []*benchproc.Config
		for cfg, phase := range prevCol.csum {
			if phase.len() < prevCol.sum*minPhaseFrac {
				continue
			}
			topPhases = append(topPhases, cfg)
		}
		// Get the top N phases.
		sort.Slice(topPhases, func(i, j int) bool {
			p1 := prevCol.csum[topPhases[i]]
			p2 := prevCol.csum[topPhases[j]]
			return p1.len() > p2.len()
		})
		if len(topPhases) > numPhases {
			topPhases = topPhases[:numPhases]
		}
		// Sort back into phase order.
		sort.Slice(topPhases, func(i, j int) bool {
			order := prevCol.phases.KeyPos
			return order[topPhases[i]] < order[topPhases[j]]
		})
		// Create initial visual intervals.
		intervals := make([]interval, len(topPhases))
		for i := range intervals {
			phase := prevCol.csum[topPhases[i]]
			mid := (y(phase.start) + y(phase.end)) / 2
			intervals[i] = interval{mid - phaseFontHeight/2, mid + phaseFontHeight/2}
		}
		// Slide intervals to remove overlaps.
		removeIntervalOverlaps(intervals)
		// Emit labels
		l := right + colSpace
		for i, cfg := range topPhases {
			phase := prevCol.csum[cfg]
			_, label := cfg.KeyVal()
			in := intervals[i]
			stroke := svgColor(colors[cfg])
			fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%d" dominant-baseline="central">%s</text>`+"\n", l+phaseFontSize/2, in.mid(), phaseFontSize, label)
			fmt.Fprintf(svg, `  <path d="M%f %fC%f %f,%f %f,%f %f" stroke="%s" stroke-width="2px" />`+"\n",
				l-colSpace, (y(phase.start)+y(phase.end))/2,
				l-colSpace/2, (y(phase.start)+y(phase.end))/2,
				l-colSpace/2, in.mid(),
				l, in.mid(),
				stroke)
			if in.end > bot {
				bot = in.end
			}
		}
		right = l + phaseWidth

		if right > maxRight {
			maxRight = right
		}
	}

	// Finalize SVG.
	fmt.Printf(
		`<svg version="1.1" width="%f" height="%f" xmlns="http://www.w3.org/2000/svg">
%s</svg>`,
		maxRight,
		bot,
		svg.Bytes(),
	)
}

func svgColor(c color.Color) string {
	switch c := c.(type) {
	case color.RGBA:
		if c.A == 255 {
			return fmt.Sprintf("rgb(%d,%d,%d)", c.R, c.G, c.B)
		} else {
			return fmt.Sprintf("rgba(%d,%d,%d,%f)", c.R, c.G, c.B, float64(c.A)/256)
		}
	}
	panic("not implemented")
}

type interval struct {
	start, end float64
}

func (in interval) mid() float64 {
	return (in.start + in.end) / 2
}

func removeIntervalOverlaps(ints []interval) {
	nints := make([]interval, len(ints))
	copy(nints, ints)

	supers := make([]int, len(ints)+1)
	for {
		// Create super-intervals from overlapping intervals.
		supers = append(supers[:0], 0)
		overlaps := 0
		for i := 1; i < len(nints); i++ {
			if nints[i].start < nints[i-1].end {
				overlaps++
			} else if nints[i].start > nints[i-1].end {
				// Gap between i-1 and i, so split the
				// super-interval.
				supers = append(supers, i)
			}
		}
		supers = append(supers, len(nints))
		if overlaps == 0 {
			// No overlapping intervals, so we're done.
			copy(ints, nints)
			return
		}

		// Spread out intervals in each super-interval and
		// move the super-interval to the overall center
		// (calculated from the original intervals).
		for i := 1; i < len(supers); i++ {
			super := nints[supers[i-1]:supers[i]]
			if len(super) == 1 {
				// No need to adjust.
				continue
			}
			orig := ints[supers[i-1]:supers[i]]

			// Get total height and center average.
			var height, midSum float64
			for _, in := range orig {
				height += in.end - in.start
				midSum += in.mid()
			}
			mid := midSum / float64(len(orig))

			// Construct adjusted intervals.
			pos := mid - height/2
			for i := range super {
				h := orig[i].end - orig[i].start
				super[i] = interval{pos, pos + h}
				pos += h
			}
		}
	}
}

// OMap is an insertion-ordered map keyed by *Config.
//
// The zero value of OMap is a usable map.
type OMap struct {
	// New is called to create new values for LoadOrNew.
	New func(key *benchproc.Config) interface{}

	// Keys is the keys of this map in insertion order.
	Keys []*benchproc.Config

	// KeyPos is a map from key to that key's position in the map.
	KeyPos map[*benchproc.Config]int

	vals map[*benchproc.Config]interface{}
}

// Load returns the value associated with key, or nil if key is not in
// the map.
func (m *OMap) Load(key *benchproc.Config) interface{} {
	return m.vals[key]
}

// LoadOK returns the value associated with key and whether or not it
// is in the map.
func (m *OMap) LoadOK(key *benchproc.Config) (interface{}, bool) {
	val, ok := m.vals[key]
	if !ok {
		_, ok = m.KeyPos[key]
	}
	return val, ok
}

// LoadOrNew is like Load, but if key doesn't exist, it first invokes
// m.New and stores the returned value under key.
func (m *OMap) LoadOrNew(key *benchproc.Config) interface{} {
	val, ok := m.LoadOK(key)
	if !ok {
		val = m.New(key)
		m.Store(key, val)
	}
	return val
}

// Store sets key's value to value. If this is the first time key has
// been stored, it adds key to the insertion order.
func (m *OMap) Store(key *benchproc.Config, value interface{}) {
	if value != nil {
		if m.vals == nil {
			m.vals = make(map[*benchproc.Config]interface{})
		}
		m.vals[key] = value
	} else if m.vals != nil {
		delete(m.vals, key)
	}

	if _, ok := m.KeyPos[key]; !ok {
		if m.KeyPos == nil {
			m.KeyPos = make(map[*benchproc.Config]int)
		}
		m.KeyPos[key] = len(m.Keys)
		m.Keys = append(m.Keys, key)
	}
}

type OMap2D struct {
	Rows, Cols []*benchproc.Config

	RowPos, ColPos map[*benchproc.Config]int

	cells map[oMap2DKey]interface{}

	New func(row, col *benchproc.Config) interface{}
}

type oMap2DKey struct {
	a, b *benchproc.Config
}

func (m *OMap2D) Load(row, col *benchproc.Config) (interface{}, bool) {
	cell, ok := m.cells[oMap2DKey{row, col}]
	return cell, ok
}

func (m *OMap2D) LoadOrNew(row, col *benchproc.Config) interface{} {
	cell, ok := m.cells[oMap2DKey{row, col}]
	if !ok {
		cell = m.New(row, col)
		m.Store(row, col, cell)
	}
	return cell
}

func (m *OMap2D) Store(row, col *benchproc.Config, value interface{}) {
	if m.cells == nil {
		m.cells = make(map[oMap2DKey]interface{})
		m.RowPos = make(map[*benchproc.Config]int)
		m.ColPos = make(map[*benchproc.Config]int)
	}

	m.cells[oMap2DKey{row, col}] = value
	if _, ok := m.RowPos[row]; !ok {
		m.RowPos[row] = len(m.Rows)
		m.Rows = append(m.Rows, row)
	}
	if _, ok := m.ColPos[col]; !ok {
		m.ColPos[col] = len(m.Cols)
		m.Cols = append(m.Cols, col)
	}
}

func (m *OMap2D) Map(f func(row, col *benchproc.Config, val interface{}) interface{}) {
	for k, v := range m.cells {
		m.cells[k] = f(k.a, k.b, v)
	}
}

func (m *OMap2D) Sort(rowLess, colLess func(a, b *benchproc.Config) bool) {
	if len(m.Rows) == 0 {
		return
	}
	sort1 := func(less func(a, b *benchproc.Config) bool, sl []*benchproc.Config, m map[*benchproc.Config]int) {
		// Sort the slice.
		sort.Slice(sl, func(i, j int) bool {
			return less(sl[i], sl[j])
		})
		// Reconstruct the position map.
		for i, cfg := range sl {
			m[cfg] = i
		}
	}
	if rowLess != nil {
		sort1(rowLess, m.Rows, m.RowPos)
	}
	if colLess != nil {
		sort1(colLess, m.Cols, m.ColPos)
	}
}
