package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

const axisAuto = "auto"

func main() {
	bucketCount := flag.Int("bucket-count", 10, "histogram bucket count")
	axisMinStr := flag.String("axis-min", axisAuto, "axis minimum value")
	axisMaxStr := flag.String("axis-max", axisAuto, "axis maximum value")
	pointFmt := flag.String("point-fmt", "%.2f", "format string for axis point value")
	graphWidth := flag.Int("graph-width", 80, "graph column width including labels")
	flag.Parse()

	axisMin, err := parseAxisRangeEnd(*axisMinStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, `axis min value must be "auto" or a floating number.`)
	}
	axisMax, err := parseAxisRangeEnd(*axisMaxStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, `axis min value must be "auto" or a floating number.`)
	}

	nArg := flag.NArg()
	if nArg != 1 && nArg != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s file1\n\nYou can use - for stdin.\n", os.Args[0])
		os.Exit(2)
	}

	if err := run(*bucketCount, axisMin, axisMax, *graphWidth, *pointFmt, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

type axisRangeEnd struct {
	Auto  bool
	Value float64
}

func parseAxisRangeEnd(s string) (axisRangeEnd, error) {
	if s == axisAuto {
		return axisRangeEnd{Auto: true}, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return axisRangeEnd{}, err
	}
	return axisRangeEnd{Value: v}, nil
}

func run(bucketCount int, axisMin, axisMax axisRangeEnd, graphWidth int, pointFmt string, filenames []string) error {
	fileCount := len(filenames)
	valuesList := make([][]float64, fileCount)
	for i, filename := range filenames {
		values, err := readFloat64ValuesFile(filenames[i])
		if err != nil {
			return err
		}

		if len(values) == 0 {
			if filename == stdinFilename {
				filename = "stdin"
			}
			return fmt.Errorf("no value in %s", filename)
		}

		valuesList[i] = values
	}

	if axisMin.Auto {
		minList := make([]float64, fileCount)
		for i, values := range valuesList {
			minList[i] = Min(values...)
		}
		min := Min(minList...)
		axisMin.Value = floorSecondSignificantDigitToMultiplesOfTwoOrFive(min)
	}
	if axisMax.Auto {
		maxList := make([]float64, fileCount)
		for i, values := range valuesList {
			maxList[i] = Max(values...)
		}
		max := Max(maxList...)
		axisMax.Value = ceilSecondSignificantDigitToMultiplesOfTwoOrFive(max)
	}

	rangePoints := BuildRangePoints(bucketCount, axisMin.Value, axisMax.Value)
	histograms := make([]*Histogram[float64], fileCount)
	for i, values := range valuesList {
		histogram := NewHistogram(rangePoints)
		histogram.AddValues(values)
		histograms[i] = histogram
	}

	formatter := NewMultipleHistogramFormatter(histograms, defaultBarChar, graphWidth, pointFmt)
	fmt.Print(formatter)

	return nil
}

const stdinFilename = "-"

func readFloat64ValuesFile(filename string) ([]float64, error) {
	r, err := newReadCloserFile(filename)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return readFloat64Values(r)
}

func newReadCloserFile(filename string) (io.ReadCloser, error) {
	if filename == stdinFilename {
		return io.NopCloser(os.Stdin), nil
	}

	return os.Open(filename)
}

const float64BitSize = 64

func readFloat64Values(r io.Reader) ([]float64, error) {
	var values []float64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		value, err := strconv.ParseFloat(scanner.Text(), float64BitSize)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

const defaultBarChar = "*"
const barMinWidth = 10

type MultipleHistogramFormatter struct {
	histograms []*Histogram[float64]
	pointFmt   string
	barChar    string
	graphWidth int
}

func NewMultipleHistogramFormatter(histograms []*Histogram[float64], barChar string, graphWidth int, pointFmt string) *MultipleHistogramFormatter {
	if len(histograms) == 0 {
		panic("histograms must not be empty")
	}
	if len(barChar) == 0 {
		panic("barChar must not be empty")
	}
	if graphWidth == 0 {
		panic("graphWidth too small")
	}

	for i := 1; i < len(histograms); i++ {
		if !slices.Equal(histograms[i].rangePoints, histograms[0].rangePoints) {
			panic("all histograms rangePoints must be same")
		}
	}

	return &MultipleHistogramFormatter{
		histograms: histograms,
		barChar:    barChar,
		graphWidth: graphWidth,
		pointFmt:   pointFmt,
	}
}

func (f *MultipleHistogramFormatter) String() string {
	lines := f.LineStrings(f.graphWidth, f.barChar, false)
	return strings.Join(lines, "\n") + "\n"
}

func (f *MultipleHistogramFormatter) LineStrings(graphWidth int, barChar string, padEnd bool) []string {
	n := len(f.histograms)
	if n == 1 {
		formatter := NewHistogramFormatter(f.histograms[0], f.barChar, f.graphWidth, f.pointFmt)
		return formatter.LineStrings(graphWidth, barChar, padEnd)
	}

	maxCounts := make([]int, n)
	for i, h := range f.histograms {
		maxCounts[i] = h.MaxCount()
	}
	maxCountMax := Max(maxCounts...)

	formatters := make([]*HistogramFormatter, n)
	for i, h := range f.histograms {
		formatters[i] = NewHistogramFormatter(h, f.barChar, f.graphWidth, f.pointFmt)
	}

	ranges := formatters[0].RangeStrings()
	rangeWidth := len(ranges[0])

	countStrsList := make([][]string, n)
	for i, f2 := range formatters {
		countStrsList[i] = f2.CountStrings()
	}

	countWidthsTotal := 0
	countWidths := make([]int, n)
	for i, countStrs := range countStrsList {
		countWidths[i] = len(countStrs[0])
		countWidthsTotal += countWidths[i]
	}

	jointWidthsTotal := n - 1
	barWidthsTotal := f.graphWidth - (rangeWidth + len(" ") + countWidthsTotal + (len(" ")+len(" |"))*n + jointWidthsTotal)
	barMaxWidth := barWidthsTotal / n

	barWidthRatio := float64(0)
	if maxCountMax != 0 {
		barWidthRatio = float64(barMaxWidth) / (float64(maxCountMax) * float64(len(barChar)))
	}

	countAndBarsList := make([][]string, n)
	for i, f2 := range formatters {
		countAndBarMaxWidth := len(" ") + countWidths[i] + len(" |") + barMaxWidth
		padEnd2 := true
		if i == len(f.histograms)-1 {
			padEnd2 = padEnd
		}
		countAndBarsList[i] = f2.CountAndBarStrings(countAndBarMaxWidth, barWidthRatio, f.barChar, padEnd2)
	}

	lines := make([]string, len(ranges))
	fields := make([]string, len(f.histograms))
	for i := range ranges {
		for j := range f.histograms {
			fields[j] = countAndBarsList[j][i]
		}
		lines[i] = ranges[i] + "  " + strings.Join(fields, " ")
	}
	return lines
}

type HistogramFormatter struct {
	histogram  *Histogram[float64]
	pointFmt   string
	barChar    string
	graphWidth int
}

func NewHistogramFormatter(histogram *Histogram[float64], barChar string, graphWidth int, pointFmt string) *HistogramFormatter {
	if len(barChar) == 0 {
		panic("barChar must not be empty")
	}
	if graphWidth == 0 {
		panic("graphWidth too small")
	}
	return &HistogramFormatter{
		histogram:  histogram,
		barChar:    barChar,
		graphWidth: graphWidth,
		pointFmt:   pointFmt,
	}
}

func (f *HistogramFormatter) RangeStrings() []string {
	tickWidth := 0
	ticks := make([]string, len(f.histogram.rangePoints))
	for i, tick := range f.histogram.rangePoints {
		s := fmt.Sprintf(f.pointFmt, tick)
		ticks[i] = s
		tickWidth = Max(tickWidth, len(s))
	}

	ranges := make([]string, len(ticks))
	for i := 0; i < len(ticks)-1; i++ {
		ranges[i] = fmt.Sprintf("%*s ~ %*s",
			tickWidth, ticks[i],
			tickWidth, ticks[i+1])
	}
	ranges[len(ticks)-1] = "out of range"

	alignRightStringSlice(ranges)
	return ranges
}

func (f *HistogramFormatter) CountStrings() []string {
	countStrs := make([]string, len(f.histogram.counts)+1)
	for i, count := range f.histogram.counts {
		s := strconv.Itoa(count)
		countStrs[i] = s
	}
	countStrs[len(f.histogram.counts)] = strconv.Itoa(f.histogram.outOfRangeCount)

	alignRightStringSlice(countStrs)
	return countStrs
}

func alignRightStringSlice(ss []string) {
	w := stringSliceMaxWidth(ss)
	for i, countStr := range ss {
		ss[i] = padStartSpace(w, countStr)
	}
}

func stringSliceMaxWidth(ss []string) int {
	w := 0
	for _, s := range ss {
		w = Max(w, len(s))
	}
	return w
}

func padStartSpace(targetWidth int, s string) string {
	return fmt.Sprintf("%*s", targetWidth, s)
}

func (f *HistogramFormatter) CountAndBarStrings(countAndBarMaxWidth int, barWidthRatio float64, barChar string, padEnd bool) []string {
	counts := f.CountStrings()
	countWidth := len(counts[0])
	barMaxWidth := countAndBarMaxWidth - (len(" ") + countWidth + len(" |"))
	bars := f.BarStrings(barMaxWidth, barWidthRatio, barChar, padEnd)

	countAndBars := make([]string, len(counts))
	for i := range countAndBars {
		countAndBars[i] = fmt.Sprintf("%s |%s", counts[i], bars[i])
	}
	return countAndBars
}

func (f *HistogramFormatter) BarStrings(barMaxWidth int, barWidthRatio float64, barChar string, padEnd bool) []string {
	if barMaxWidth <= barMinWidth {
		log.Fatalf("bar max width becomes too small, retry with larger graphWidth, barMaxWidth=%d, graphWidth=%d", barMaxWidth, f.graphWidth)
	}

	bars := make([]string, len(f.histogram.counts)+1)
	for i, count := range f.histogram.counts {
		barWidth := int(float64(count) * barWidthRatio)
		if padEnd {
			bars[i] = strings.Repeat(f.barChar, barWidth) + strings.Repeat(" ", barMaxWidth-barWidth)
		} else {
			bars[i] = strings.Repeat(f.barChar, barWidth)
		}
	}
	if padEnd {
		bars[len(f.histogram.counts)] = strings.Repeat(" ", barMaxWidth)
	}
	return bars
}

func (f *HistogramFormatter) LineStrings(graphWidth int, barChar string, padEnd bool) []string {
	ranges := f.RangeStrings()
	counts := f.CountStrings()

	rangeWidth := len(ranges[0])
	countWidth := len(counts[0])
	barMaxWidth := graphWidth - (rangeWidth + len("  ") + countWidth + len(" |"))

	maxCount := f.histogram.MaxCount()
	barWidthRatio := float64(0)
	if maxCount != 0 {
		barWidthRatio = float64(barMaxWidth) / (float64(maxCount) * float64(len(barChar)))
	}

	bars := f.BarStrings(barMaxWidth, barWidthRatio, barChar, padEnd)

	lines := make([]string, len(ranges))
	for i := range lines {
		lines[i] = fmt.Sprintf("%s  %s |%s", ranges[i], counts[i], bars[i])
	}
	return lines
}

func (f *HistogramFormatter) String() string {
	lines := f.LineStrings(f.graphWidth, f.barChar, false)
	return strings.Join(lines, "\n") + "\n"
}

type Number interface {
	constraints.Integer | constraints.Float
}

type Histogram[T Number] struct {
	rangePoints     []T
	counts          []int
	outOfRangeCount int
}

func NewHistogram[T Number](rangePoints []T) *Histogram[T] {
	counts := make([]int, len(rangePoints)-1)
	return &Histogram[T]{rangePoints: rangePoints, counts: counts}
}

func BuildRangePoints[T Number](count int, min, max T) []T {
	rangePoints := make([]T, count+1)
	for i := 0; i <= count; i++ {
		rangePoints[i] = min + (max-min)*T(i)/T(count)
	}
	return rangePoints
}

func (h *Histogram[T]) AddValues(values []T) {
	for _, v := range values {
		h.AddValue(v)
	}
}

func (h *Histogram[T]) AddValue(v T) {
	if v < h.rangePoints[0] || v > h.rangePoints[len(h.rangePoints)-1] {
		h.outOfRangeCount++
		return
	}
	i := sort.Search(len(h.rangePoints), func(i int) bool { return h.rangePoints[i] > v }) - 1
	if i < len(h.counts) {
		h.counts[i]++
	}
}

func (h *Histogram[T]) MaxCount() int {
	return Max(h.counts...)
}

func (h *Histogram[T]) RangePoints() []T {
	rangePointsCopy := make([]T, len(h.rangePoints))
	copy(rangePointsCopy, h.rangePoints)
	return rangePointsCopy
}

func (h *Histogram[T]) Counts() []int {
	countsCopy := make([]int, len(h.counts))
	copy(countsCopy, h.counts)
	return countsCopy
}

func (h *Histogram[T]) Equal(o *Histogram[T]) bool {
	return slices.Equal(h.rangePoints, o.rangePoints) && slices.Equal(h.counts, o.counts)
}

func Min[T constraints.Ordered](values ...T) T {
	if len(values) == 0 {
		panic("values must not be empty")
	}

	min := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] < min {
			min = values[i]
		}
	}
	return min
}

func Max[T constraints.Ordered](values ...T) T {
	if len(values) == 0 {
		panic("values must not be empty")
	}

	max := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] > max {
			max = values[i]
		}
	}
	return max
}

func ceilSecondSignificantDigitToMultiplesOfTwoOrFive(v float64) float64 {
	if v < 0 {
		return -floorSecondSignificantDigitToMultiplesOfTwoOrFive(-v)
	}

	s := fmt.Sprintf("%.1e", v)
	// s is like 4.6e+01
	d1 := mustAtoi(s[0:1])
	d2 := mustAtoi(s[2:3])
	exp := mustAtoi(s[4:])
	if v > mustParseFloat(s, float64BitSize) {
		if d2 == 9 {
			d1++
			d2 = 0
		} else {
			d2++
		}
	}
	switch d2 {
	case 1:
		d2 = 2
	case 3:
		d2 = 4
	case 7:
		d2 = 8
	case 9:
		d1++
		d2 = 0
	}
	s2 := fmt.Sprintf("%d.%de%d", d1, d2, exp)
	return mustParseFloat(s2, float64BitSize)
}

func floorSecondSignificantDigitToMultiplesOfTwoOrFive(v float64) float64 {
	if v < 0 {
		return -ceilSecondSignificantDigitToMultiplesOfTwoOrFive(-v)
	}

	s := fmt.Sprintf("%.1e", v)
	// s is like 4.6e+01
	d1 := mustAtoi(s[0:1])
	d2 := mustAtoi(s[2:3])
	exp := mustAtoi(s[4:])
	if v < mustParseFloat(s, float64BitSize) {
		if d2 == 0 {
			d1--
			d2 = 9
		} else {
			d2--
		}
	}
	switch d2 {
	case 1, 3, 7, 9:
		d2--
	}
	s2 := fmt.Sprintf("%d.%de%d", d1, d2, exp)
	return mustParseFloat(s2, float64BitSize)
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic("expected integer string")
	}
	return i
}

func mustParseFloat(s string, bitSize int) float64 {
	f, err := strconv.ParseFloat(s, bitSize)
	if err != nil {
		panic("failed to parse float value")
	}
	return f
}
