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
	"golang.org/x/perf/v2/benchstat"
	"golang.org/x/perf/v2/benchunit"
)

// Qualitative palettes from Color Brewer.
var Pastel1_9 = []color.Color{color.RGBA{251, 180, 174, 255}, color.RGBA{179, 205, 227, 255}, color.RGBA{204, 235, 197, 255}, color.RGBA{222, 203, 228, 255}, color.RGBA{254, 217, 166, 255}, color.RGBA{255, 255, 204, 255}, color.RGBA{229, 216, 189, 255}, color.RGBA{253, 218, 236, 255}, color.RGBA{242, 242, 242, 255}}
var Accent_8 = []color.Color{color.RGBA{127, 201, 127, 255}, color.RGBA{190, 174, 212, 255}, color.RGBA{253, 192, 134, 255}, color.RGBA{255, 255, 153, 255}, color.RGBA{56, 108, 176, 255}, color.RGBA{240, 2, 127, 255}, color.RGBA{191, 91, 23, 255}, color.RGBA{102, 102, 102, 255}}
var Dark2_8 = []color.Color{color.RGBA{27, 158, 119, 255}, color.RGBA{217, 95, 2, 255}, color.RGBA{117, 112, 179, 255}, color.RGBA{231, 41, 138, 255}, color.RGBA{102, 166, 30, 255}, color.RGBA{230, 171, 2, 255}, color.RGBA{166, 118, 29, 255}, color.RGBA{102, 102, 102, 255}}
var Set1_9 = []color.Color{color.RGBA{228, 26, 28, 255}, color.RGBA{55, 126, 184, 255}, color.RGBA{77, 175, 74, 255}, color.RGBA{152, 78, 163, 255}, color.RGBA{255, 127, 0, 255}, color.RGBA{255, 255, 51, 255}, color.RGBA{166, 86, 40, 255}, color.RGBA{247, 129, 191, 255}, color.RGBA{153, 153, 153, 255}}

var pal = Set1_9

// A Cell captures data from a sequence of phases in a given benchmark
// configuration.
type Cell interface {
	Extents(*Extents)
	Render(svg *SVG, scales *Scales, prev Cell, prevRight float64)
	RenderKey(svg *SVG, x float64, y scale.QQ, lastRight float64) (right, bot float64)
}

type Box struct {
	Top, Right, Bottom, Left float64
}

type Extents struct {
	X, X2 scale.Linear
	Y     scale.Linear

	Margins Box
}

type Scales struct {
	X, X2 scale.QQ
	Y     scale.QQ

	Outer   Box
	Margins Box
}

func expandScale(s *scale.Linear, min, max float64) {
	if s.Min == 0 && s.Max == 0 {
		s.Min, s.Max = min, max
	} else {
		s.Min = math.Min(s.Min, min)
		s.Max = math.Max(s.Max, max)
	}
}

const labelFontSize = 8
const labelFontHeight = labelFontSize * 5 / 4

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

	// XXX Take this as an argument?
	units := make(map[*benchproc.Config]unitInfo) // unit config
	var tidier benchunit.Tidier
	for _, unit := range []string{"ns/op", "B/op"} {
		cfg := cs.KeyVal(".unit", unit)
		tidyUnit, tidyFactor := tidier.Tidy(unit)
		unitClass := benchunit.UnitClassOf(tidyUnit)
		units[cfg] = unitInfo{cfg, tidyUnit, tidyFactor, unitClass}
	}

	// Parse measurements into cells.
	var measurements OMap2D // (unit, colBy) -> *OMap<phaseCfg -> []float64>
	measurements.New = func(row, col *benchproc.Config) interface{} {
		return &OMap{
			New: func(key *benchproc.Config) interface{} {
				return ([]float64)(nil)
			},
		}
	}
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
			if !strings.HasSuffix(string(res.FullName), "_GC") {
				continue
			}
			// if strings.HasSuffix(string(res.FullName), "_GC") {
			// 	continue
			// }

			// Ignore total time benchmark.
			if strings.HasPrefix(string(res.FullName), "TotalTime") {
				continue
			}

			// Strip fake Loadlibfull phase from old linker.
			if strings.HasPrefix(string(res.FullName), "Loadlibfull") {
				if ns, ok := res.Value("ns/op"); ok && ns < 1000 {
					continue
				}
			}

			colCfg := colBy.Project(cs, res)
			phaseCfg := phaseBy.Project(cs, res)

			// Assign colors to phases.
			if phaseColors.Load(phaseCfg) == nil {
				phaseColors.Store(phaseCfg, pal[len(phaseColors.Keys)%len(pal)])
			}

			for _, value := range res.Values {
				unitCfg := cs.KeyVal(".unit", value.Unit)
				unitInfo, ok := units[unitCfg]
				if !ok {
					continue
				}
				val := value.Value * unitInfo.tidyFactor

				cell := measurements.LoadOrNew(unitCfg, colCfg).(*OMap)
				vals := cell.LoadOrNew(phaseCfg).([]float64)
				cell.Store(phaseCfg, append(vals, val))
			}
		}
		if err := reader.Err(); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	if len(measurements.Rows) == 0 {
		log.Fatal("no data")
	}

	// Transform distributions into cells by row.
	var cells OMap2D     // (unit, colBy) -> Cell
	var rowDists []*OMap // phaseCfg -> *Distribution
	for _, unitCfg := range measurements.Rows {
		rowDists = rowDists[:0]
		for _, colCfg := range measurements.Cols {
			if phases, ok := measurements.LoadOK(unitCfg, colCfg); ok {
				dists := phases.(*OMap).Map(func(key *benchproc.Config, val interface{}) interface{} {
					return benchstat.NewDistribution(val.([]float64), benchstat.DistributionOptions{})
				})
				rowDists = append(rowDists, dists)
			}
		}
		rowCells := NewStacks(rowDists, units[unitCfg].class)
		for _, colCfg := range measurements.Cols {
			if _, ok := measurements.LoadOK(unitCfg, colCfg); ok {
				cells.Store(unitCfg, colCfg, rowCells[0])
				rowCells = rowCells[1:]
			}
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
	maxBot := topSpace

	// Rows
	var maxRight float64
	for rowI, unitCfg := range cells.Rows {
		top := topSpace + float64(rowI*(rowHeight+rowGap))
		bot := top + rowHeight
		if bot > maxBot {
			maxBot = bot
		}

		unitInfo := units[unitCfg]

		// Unit label
		fmt.Fprintf(svg, `  <text font-size="%d" text-anchor="middle" transform="translate(%f %f) rotate(-90)">%s</text>`+"\n", unitFontSize, float64(unitFontSize), float64(top+rowHeight/2), unitInfo.tidyUnit)

		// Construct scalers for this row.
		var ext Extents
		var scales Scales
		for _, colCfg := range cells.Cols {
			cell := cells.Load(unitCfg, colCfg).(Cell)
			if cell == nil {
				continue
			}
			cell.Extents(&ext)
		}
		scales.Margins = ext.Margins
		scales.Outer.Top = top
		scales.Outer.Bottom = bot
		yOut := scale.Linear{Min: top + ext.Margins.Top, Max: bot - ext.Margins.Bottom}
		scales.Y = scale.QQ{&ext.Y, &yOut}

		// Render cells.
		var prev Cell
		var prevRight float64
		for i, colCfg := range cells.Cols {
			cell := cells.Load(unitCfg, colCfg).(Cell)
			if cell == nil {
				continue
			}

			l, r := x(i)
			scales.Outer.Left = l
			scales.Outer.Right = r
			xOut := scale.Linear{Min: l + ext.Margins.Left, Max: r - ext.Margins.Right}
			scales.X = scale.QQ{&ext.X, &xOut}
			scales.X2 = scale.QQ{&ext.X2, &xOut}
			cell.Render(svg, &scales, prev, prevRight)
			prev, prevRight = cell, r
		}

		// Render key.
		keyLeft, _ := x(len(cells.Cols))
		keyRight, keyBot := prev.RenderKey(svg, keyLeft, scales.Y, prevRight)
		if keyRight > maxRight {
			maxRight = keyRight
		}
		if keyBot > maxBot {
			maxBot = keyBot
		}
	}

	// Finalize SVG.
	fmt.Printf(
		`<svg version="1.1" width="%f" height="%f" xmlns="http://www.w3.org/2000/svg">
%s</svg>`,
		maxRight,
		maxBot,
		svgBuf.Bytes(),
	)
}

func mid(a, b float64) float64 {
	return (a + b) / 2
}

func svgColor(c color.Color) string {
	c2 := color.NRGBAModel.Convert(c).(color.NRGBA)
	if c2.A == 255 {
		return fmt.Sprintf("rgb(%d,%d,%d)", c2.R, c2.G, c2.B)
	} else {
		return fmt.Sprintf("rgba(%d,%d,%d,%f)", c2.R, c2.G, c2.B, float64(c2.A)/255)
	}
}

func svgPathRect(x1, y1, x2, y2 float64) string {
	return fmt.Sprintf("M%f %fH%fV%fH%fz", x1, y1, x2, y2, x1)
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
