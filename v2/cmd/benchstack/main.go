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
	"reflect"
	"sort"
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

var (
	topPal   = Dark2_8[:len(Dark2_8)-2]
	otherPal = Dark2_8[len(Dark2_8)-2:]
)

// A Cell captures data from a sequence of phases in a given benchmark
// configuration.
type Cell interface {
	Extents(*Extents)
	Render(svg *SVG, scales *Scales, prev Cell, prevRight float64)
	RenderKey(svg *SVG, x float64, lastScales *Scales) (right, bot float64)
}

type Box struct {
	Top, Right, Bottom, Left float64
}

type Extents struct {
	X, X2 scale.Linear
	Y     scale.Linear

	Margins Box

	// TopPhases and OtherPhases are graphs of visually adjacent
	// phase configurations. These graphs are colored to determine
	// phase colors.
	TopPhases, OtherPhases ConfigGraph
}

type Scales struct {
	X, X2 scale.QQ
	Y     scale.QQ

	Outer   Box
	Margins Box

	// Colors assigns colors to phases based on the adjacent phase
	// graph.
	Colors map[benchproc.Config]color.Color

	PhaseField benchproc.Field
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

const keyFontSize = 12
const keyFontHeight = keyFontSize * 5 / 4
const keyWidth = 150

type SVG struct {
	w   io.Writer
	gen int
}

func (s *SVG) Write(x []byte) (int, error) {
	return s.w.Write(x)
}

func (s *SVG) GenID(prefix string) string {
	id := fmt.Sprintf("%s%d", prefix, s.gen)
	s.gen++
	return id
}

type unitInfo struct {
	class    benchunit.UnitClass
	newCells func(dists []*OMap, unitClass benchunit.UnitClass) []Cell
}

func main() {
	flagCol := flag.String("col", "branch,commit-date,commit", "split columns by distinct values of `projection`")
	flagRow := flag.String("row", "benchmark,/kind", "split rows by distinct values of `projection`")
	flagFilter := flag.String("filter", "*", "use only benchmarks matching benchfilter `query`")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	// TODO: Put filter arg in a package along with FileArgs.
	filter, err := benchproc.NewFilter(*flagFilter)
	if err != nil {
		log.Fatal(err)
	}

	var parser benchproc.ProjectionParser
	colBy, err := parser.Parse(*flagCol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing -col: %s", err)
		os.Exit(1)
	}
	rowBy, err := parser.Parse(*flagRow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing -row: %s", err)
		os.Exit(1)
	}
	unitField := rowBy.AddValues() // ".unit" is always the tidy unit
	phaseBy, _ := parser.Parse(".name")

	// XXX Take this as an argument?
	units := make(map[string]unitInfo) // Keyed by tidy unit
	for _, unit := range []string{"sec/op", "B/op", "live-B", "heap-B"} {
		unitClass := benchunit.UnitClassOf(unit)
		var newCells func(dists []*OMap, unitClass benchunit.UnitClass) []Cell
		switch unit {
		case "sec/op", "B/op":
			newCells = NewStacks
		case "live-B", "heap-B":
			newCells = NewDeltaCells
		}
		units[unit] = unitInfo{unitClass, newCells}
	}

	// Parse measurements into cells.
	type cellKey struct {
		row benchproc.Config
		col benchproc.Config
	}
	// TODO: The remaining uses of OMap are pretty uninteresting
	// at this point. Can I make a Schema track the ordering and
	// just use a regular map? Part of why that's hard is that
	// each cell tracks its own order and I combine them using
	// globalOrder. I'm not sure how to make Schema do something
	// like that.
	measurements := make(map[cellKey]*OMap) // OMap is phaseCfg -> []float64
	rowSet := make(map[benchproc.Config]bool)
	colSet := make(map[benchproc.Config]bool)

	files := benchfmt.Files{Paths: flag.Args(), AllowStdin: true}
	for files.Scan() {
		res, err := files.Result()
		if err != nil {
			log.Print(err)
			continue
		}
		benchunit.Tidy(res)

		// Canonicalize "_GC" to a name key (that's
		// how it should have been in the first
		// place).
		if strings.HasSuffix(string(res.FullName), "_GC") {
			res.FullName = append(res.FullName[:len(res.FullName)-len("_GC")], "/kind=mem"...)
		} else {
			res.FullName = append(res.FullName, "/kind=cpu"...)
		}

		match := filter.Match(res)
		if !match.Apply(res) {
			continue
		}

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

		colCfg, ok1 := colBy.Project(res)
		rowCfgs, ok2 := rowBy.ProjectValues(res)
		phaseCfg, _ := phaseBy.Project(res)
		if !ok1 || !ok2 {
			continue
		}

		for i, value := range res.Values {
			if _, ok := units[value.Unit]; !ok {
				// Ignored unit.
				continue
			}

			key := cellKey{rowCfgs[i], colCfg}
			rowSet[key.row] = true
			colSet[key.col] = true

			cell := measurements[key]
			if cell == nil {
				cell = &OMap{
					New: func(key benchproc.Config) interface{} {
						return ([]float64)(nil)
					},
				}
				measurements[key] = cell
			}

			vals := cell.LoadOrNew(phaseCfg).([]float64)
			cell.Store(phaseCfg, append(vals, value.Value))
		}
	}
	if err := files.Err(); err != nil {
		log.Fatal(err)
	}

	if len(measurements) == 0 {
		log.Fatal("no data")
	}

	// Construct sorted rows and columns.
	rows := mapKeys(rowSet).([]benchproc.Config)
	benchproc.SortConfigs(rows)
	cols := mapKeys(colSet).([]benchproc.Config)
	benchproc.SortConfigs(cols)

	// Transform distributions into cells by row.
	cells := make(map[cellKey]Cell)
	for _, row := range rows {
		var rowDists []*OMap // OMap is phaseCfg -> *Distribution
		for _, col := range cols {
			if phases, ok := measurements[cellKey{row, col}]; ok {
				dists := phases.Map(func(key benchproc.Config, val interface{}) interface{} {
					return benchstat.NewDistribution(val.([]float64), benchstat.DistributionOptions{})
				})
				rowDists = append(rowDists, dists)
			}
		}
		unit := row.Get(unitField)
		rowCells := units[unit].newCells(rowDists, units[unit].class)
		for _, col := range cols {
			if _, ok := measurements[cellKey{row, col}]; ok {
				cells[cellKey{row, col}] = rowCells[0]
				rowCells = rowCells[1:]
			}
		}
	}

	// Emit SVG
	svgBuf := new(bytes.Buffer)
	svg := &SVG{w: svgBuf}
	const configFontSize float64 = 12
	const configFontHeight = configFontSize * 5 / 4
	const colWidth = 100
	const colSpace = 30 // Enough for "-100%"
	const rowHeight = 300
	const rowGap = 10

	// Column and row labels
	rowHdr := benchproc.NewConfigHeader(rows)
	colHdr := benchproc.NewConfigHeader(cols)
	cellTop := float64(len(colBy.Fields())) * configFontHeight
	cellLeft := float64(len(rowBy.Fields())) * configFontHeight
	x := func(col int) (float64, float64) {
		l := cellLeft + float64(col)*(colWidth+colSpace)
		return l, l + colWidth
	}
	y := func(row int) (float64, float64) {
		t := cellTop + float64(row)*(rowHeight+rowGap)
		return t, t + rowHeight
	}

	for _, col := range colHdr {
		for _, cell := range col {
			l, _ := x(cell.Start)
			_, r := x(cell.Start + cell.Len - 1)
			fmt.Fprintf(svg, `  <text x="%f" y="%f" font-size="%f" text-anchor="middle">%s</text>`+"\n", (l+r)/2, float64(cell.Field)*configFontHeight+configFontSize, configFontSize, cell.Value)
			// Emit grouping bar if this is a group
			if cell.Len > 1 {
				fmt.Fprintf(svg, `  <path d="M%f %fH%f" stroke="black" stroke-width="1px" />`+"\n", l, float64(cell.Field+1)*configFontHeight, r)
			}
		}
	}

	for _, row := range rowHdr {
		for _, cell := range row {
			t, _ := y(cell.Start)
			_, b := y(cell.Start + cell.Len - 1)
			fmt.Fprintf(svg, `  <text transform="translate(%f %f) rotate(-90)" font-size="%f" text-anchor="middle">%s</text>`+"\n", float64(cell.Field)*configFontHeight+configFontSize, (t+b)/2, configFontSize, cell.Value)
			// Emit grouping bar if this is a group
			if cell.Len > 1 {
				fmt.Fprintf(svg, `  <path d="M%f %fV%f" stroke="black" stroke-width="1px" />`+"\n", float64(cell.Field+1)*configFontHeight, t, b)
			}
		}
	}

	_, maxRight := x(len(cols) - 1)
	_, maxBot := y(len(rows) - 1)

	// Cell rows
	for rowI, rowCfg := range rows {
		top, bot := y(rowI)
		if bot > maxBot {
			maxBot = bot
		}

		// Construct scalers for this row.
		var ext Extents
		var scales Scales
		for _, colCfg := range cols {
			cell, ok := cells[cellKey{rowCfg, colCfg}]
			if !ok {
				continue
			}
			cell.Extents(&ext)
		}
		scales.Margins = ext.Margins
		scales.Outer.Top = top
		scales.Outer.Bottom = bot
		yOut := scale.Linear{Min: top + ext.Margins.Top, Max: bot - ext.Margins.Bottom}
		scales.Y = scale.QQ{&ext.Y, &yOut}
		scales.PhaseField = phaseBy.Fields()[0]

		// Color phases.
		scales.Colors = make(map[benchproc.Config]color.Color)
		assignColors(scales.Colors, &ext.TopPhases, topPal)
		assignColors(scales.Colors, &ext.OtherPhases, otherPal)

		// Render cells.
		var prev Cell
		var prevRight float64
		for i, colCfg := range cols {
			cell, ok := cells[cellKey{rowCfg, colCfg}]
			if !ok {
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
		keyLeft, _ := x(len(cols))
		keyRight, keyBot := prev.RenderKey(svg, keyLeft, &scales)
		if keyRight > maxRight {
			maxRight = keyRight
		}
		if keyBot > maxBot {
			maxBot = keyBot
		}
	}

	// Finalize SVG.
	fmt.Printf(
		`<svg version="1.1" width="%f" height="%f" xmlns="http://www.w3.org/2000/svg" font-family="sans-serif">
%s</svg>`,
		maxRight,
		maxBot,
		svgBuf.Bytes(),
	)
}

func mapKeys(m interface{}) interface{} {
	mv := reflect.ValueOf(m)
	keys := mv.MapKeys()
	out := reflect.MakeSlice(reflect.SliceOf(mv.Type().Key()), len(keys), len(keys))
	for i, key := range keys {
		out.Index(i).Set(key)
	}
	return out.Interface()
}

func assignColors(out map[benchproc.Config]color.Color, g *ConfigGraph, pal []color.Color) {
	for cfg, idx := range g.Color(len(pal)) {
		out[cfg] = pal[idx%len(pal)]
	}
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

func svgPathHSquiggle(x1, y1, x2, y2 float64) string {
	return fmt.Sprintf("M%f %fC%f %f,%f %f,%f %f",
		x1, y1,
		mid(x1, x2), y1,
		mid(x1, x2), y2,
		x2, y2)
}

func colorBlend(a, b color.Color, by float64) color.Color {
	x := color.NRGBA64Model.Convert(a).(color.NRGBA64)
	y := color.NRGBA64Model.Convert(b).(color.NRGBA64)
	blend := func(x, y uint16) uint16 {
		z := int32(0.5 + float64(x)*(1-by) + float64(y)*by)
		if z <= 0 {
			return 0
		} else if z >= 0xffff {
			return 0xffff
		}
		return uint16(z)
	}
	return color.NRGBA64{
		blend(x.R, y.R),
		blend(x.G, y.G),
		blend(x.B, y.B),
		blend(x.A, y.A),
	}
}

type interval struct {
	start, end float64
	data       interface{}
}

func (in interval) mid() float64 {
	return (in.start + in.end) / 2
}

func removeIntervalOverlaps(ints []interval) {
	sort.Slice(ints, func(i, j int) bool {
		return ints[i].mid() < ints[j].mid()
	})

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
				super[i] = interval{pos, pos + h, orig[i].data}
				pos += h
			}
		}
	}
}
