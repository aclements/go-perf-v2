// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"math"
	"os"
	"strings"

	"github.com/aclements/go-moremath/scale"
	"golang.org/x/perf/v2/benchfmt"
	"golang.org/x/perf/v2/benchproc"
	"golang.org/x/perf/v2/benchunit"
)

// Qualitative palettes from Color Brewer.
var Pastel1_9 = []color.Color{color.RGBA{251, 180, 174, 255}, color.RGBA{179, 205, 227, 255}, color.RGBA{204, 235, 197, 255}, color.RGBA{222, 203, 228, 255}, color.RGBA{254, 217, 166, 255}, color.RGBA{255, 255, 204, 255}, color.RGBA{229, 216, 189, 255}, color.RGBA{253, 218, 236, 255}, color.RGBA{242, 242, 242, 255}}
var Accent_8 = []color.Color{color.RGBA{127, 201, 127, 255}, color.RGBA{190, 174, 212, 255}, color.RGBA{253, 192, 134, 255}, color.RGBA{255, 255, 153, 255}, color.RGBA{56, 108, 176, 255}, color.RGBA{240, 2, 127, 255}, color.RGBA{191, 91, 23, 255}, color.RGBA{102, 102, 102, 255}}
var Dark2_8 = []color.Color{color.RGBA{27, 158, 119, 255}, color.RGBA{217, 95, 2, 255}, color.RGBA{117, 112, 179, 255}, color.RGBA{231, 41, 138, 255}, color.RGBA{102, 166, 30, 255}, color.RGBA{230, 171, 2, 255}, color.RGBA{166, 118, 29, 255}, color.RGBA{102, 102, 102, 255}}
var Set1_9 = []color.Color{color.RGBA{228, 26, 28, 255}, color.RGBA{55, 126, 184, 255}, color.RGBA{77, 175, 74, 255}, color.RGBA{152, 78, 163, 255}, color.RGBA{255, 127, 0, 255}, color.RGBA{255, 255, 51, 255}, color.RGBA{166, 86, 40, 255}, color.RGBA{247, 129, 191, 255}, color.RGBA{153, 153, 153, 255}}

var pal = Set1_9

// A Row collects data from comparable benchmark runs and for a
// particular unit.
type Row interface {
	Add(colCfg, phaseCfg *benchproc.Config, val float64)
	Finish(cols []*benchproc.Config) []Cell
	RenderKey(svg *SVG, x float64, y scale.QQ, last Cell, lastRight float64) (right, bot float64)
}

// A Cell captures data from a sequence of phases in a given benchmark
// configuration.
type Cell interface {
	Extent() (xmin, xmax, ymin, ymax float64)
	Render(svg *SVG, x, y scale.QQ, prev Cell, prevRight float64)
}

const labelFontSize = 8

type SVG struct {
	w           io.Writer
	phaseColors OMap
	gen         int
}

func (s *SVG) Write(x []byte) (int, error) {
	return s.w.Write(x)
}

func (s *SVG) PhaseColor(phaseCfg *benchproc.Config) string {
	return svgColor(s.phaseColors.Load(phaseCfg).(color.Color))
}

func (s *SVG) GenID(prefix string) string {
	id := fmt.Sprintf("%s%d", prefix, s.gen)
	s.gen++
	return id
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

	var phaseColors OMap // phase config -> color.Color

	// cells maps from (unit, colBy) to Cell. Initially we fill
	// this with nils and then populate the Cells when we can
	// finalize the Rows.
	var cells OMap2D

	// rows tracks the Row for each unit config.
	rows := make(map[*benchproc.Config]Row)

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

			// Assign colors to phases.
			if phaseColors.Load(phaseCfg) == nil {
				phaseColors.Store(phaseCfg, pal[len(phaseColors.Keys)%len(pal)])
			}

			for _, value := range res.Values {
				unitInfo, ok := units[value.Unit]
				if !ok {
					continue
				}
				val := value.Value * unitInfo.tidyFactor

				// Record order in cells.
				cells.Store(unitInfo.cfg, colCfg, nil)

				// Add measurement to the row.
				row := rows[unitInfo.cfg]
				if row == nil {
					row = NewStackRow(unitInfo.class)
					rows[unitInfo.cfg] = row
				}
				row.Add(colCfg, phaseCfg, val)
			}
		}
		if err := reader.Err(); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	if len(cells.Rows) == 0 {
		log.Fatal("no data")
	}

	// Finalize rows and populate cells map.
	for rowCfg, row := range rows {
		rowCells := row.Finish(cells.Cols)
		for i, colCfg := range cells.Cols {
			cells.Store(rowCfg, colCfg, rowCells[i])
		}
	}

	// Emit SVG
	svgBuf := new(bytes.Buffer)
	svg := &SVG{w: svgBuf, phaseColors: phaseColors}
	const unitFontSize = 12
	const unitFontHeight = 12 * 5 / 4
	const colWidth = 100
	const colSpace = 50
	const colFontSize = 12
	const colFontHeight = 12 * 5 / 4
	const rowHeight = 300
	const rowGap = 10
	x := func(col int) (float64, float64) {
		l := unitFontHeight + col*(colWidth+colSpace)
		return float64(l), float64(l + colWidth)
	}

	// Column labels
	var topSpace float64
	colTree, _ := benchproc.NewConfigTree(cells.Cols)
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

	// Rows
	var maxRight float64
	for rowI, unitCfg := range cells.Rows {
		top := topSpace + float64(rowI*(rowHeight+rowGap))

		unit := unitCfg.Val()
		unitInfo := units[unit]

		// Unit label
		fmt.Fprintf(svg, `  <text font-size="%d" text-anchor="middle" transform="translate(%f %f) rotate(-90)">%s</text>`+"\n", unitFontSize, float64(unitFontSize), float64(top+rowHeight/2), unitInfo.tidyUnit)

		// Construct X and Y scalers for this row.
		var xIn, yIn scale.Linear
		for i, colCfg := range cells.Cols {
			cell := cells.Load(unitCfg, colCfg).(Cell)
			if cell == nil {
				continue
			}
			xmin, xmax, ymin, ymax := cell.Extent()
			if i == 0 {
				xIn.Min, xIn.Max = xmin, xmax
				yIn.Min, yIn.Max = ymin, ymax
			} else {
				xIn.Min, xIn.Max = math.Min(xIn.Min, xmin), math.Max(xIn.Max, xmax)
				yIn.Min, yIn.Max = math.Min(yIn.Min, ymin), math.Max(yIn.Max, ymax)
			}
		}
		yOut := scale.Linear{Min: top, Max: top + rowHeight}
		yScale := scale.QQ{&yIn, &yOut}
		if yOut.Max > bot {
			bot = yOut.Max
		}

		// Render cells.
		var prev Cell
		var prevRight float64
		for i, colCfg := range cells.Cols {
			cell := cells.Load(unitCfg, colCfg).(Cell)
			if cell == nil {
				continue
			}

			l, r := x(i)
			xScale := scale.QQ{&xIn, &scale.Linear{Min: l, Max: r}}
			cell.Render(svg, xScale, yScale, prev, prevRight)
			prev, prevRight = cell, r
		}

		// Render key.
		keyLeft, _ := x(len(cells.Cols))
		row := rows[unitCfg]
		keyRight, keyBot := row.RenderKey(svg, keyLeft, yScale, prev, prevRight)
		if keyRight > maxRight {
			maxRight = keyRight
		}
		if keyBot > bot {
			bot = keyBot
		}
	}

	// Finalize SVG.
	fmt.Printf(
		`<svg version="1.1" width="%f" height="%f" xmlns="http://www.w3.org/2000/svg">
%s</svg>`,
		maxRight,
		bot,
		svgBuf.Bytes(),
	)
}

func mid(a, b float64) float64 {
	return (a + b) / 2
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
