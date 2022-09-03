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

func main() {
	bucketCount := flag.Int("bucket-count", 10, "histogram bucket count")
	axisMin := flag.Float64("axis-min", 0, "axis minimum value")
	axisMax := flag.Float64("axis-max", 10, "axis maximum value")
	fracPrec := flag.Uint("frac-prec", 2, "number fraction precision width")
	fixedAxis := flag.Bool("fixed-axis", false, "if enabled, axis min and max are fixed even if some of values are out of range")
	graphWidth := flag.Int("graph-width", 80, "graph column width including labels")
	flag.Parse()

	nArg := flag.NArg()
	if nArg != 1 && nArg != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s file1\n\nYou can use - for stdin.\n", os.Args[0])
		os.Exit(2)
	}

	if err := run(*bucketCount, *axisMin, *axisMax, *fixedAxis, *graphWidth, *fracPrec, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

func run(bucketCount int, axisMin, axisMax float64, fixedAxis bool, graphWidth int, fracPrec uint, filenames []string) error {
	fileCount := len(filenames)
	minList := make([]float64, fileCount)
	maxList := make([]float64, fileCount)
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
		minList[i] = Min(values...)
		maxList[i] = Max(values...)
	}

	min := Min(minList...)
	max := Max(maxList...)
	if !fixedAxis {
		axisMin = Min(axisMin, min)
		axisMax = Max(axisMax, max)
		axisMax = adjustMax(axisMax)
	}

	rangePoints := BuildRangePoints(bucketCount, axisMin, axisMax)
	histograms := make([]*Histogram[float64], fileCount)
	for i, values := range valuesList {
		histogram := NewHistogram(rangePoints)
		histogram.AddValues(values)
		histograms[i] = histogram
	}

	formatter := NewMultipleHistogramFormatter(histograms, defaultBarChar, graphWidth, fracPrec)
	fmt.Print(formatter)

	return nil
}

const stdinFilename = "-"

func readFloat64ValuesFile(filename string) ([]float64, error) {
	if filename == stdinFilename {
		return readFloat64Values(os.Stdin)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return readFloat64Values(file)
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
	fracPrec   uint
	barChar    string
	graphWidth int
}

func NewMultipleHistogramFormatter(histograms []*Histogram[float64], barChar string, graphWidth int, fracPrec uint) *MultipleHistogramFormatter {
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
		fracPrec:   fracPrec,
	}
}

func (f *MultipleHistogramFormatter) String() string {
	lines := f.LineStrings(f.graphWidth, f.barChar, false)
	return strings.Join(lines, "\n") + "\n"
}

func (f *MultipleHistogramFormatter) LineStrings(graphWidth int, barChar string, padEnd bool) []string {
	n := len(f.histograms)
	if n == 1 {
		formatter := NewHistogramFormatter(f.histograms[0], f.barChar, f.graphWidth, f.fracPrec)
		return formatter.LineStrings(graphWidth, barChar, padEnd)
	}

	maxCounts := make([]int, n)
	for i, h := range f.histograms {
		maxCounts[i] = h.MaxCount()
	}
	maxCountMax := Max(maxCounts...)

	formatters := make([]*HistogramFormatter, n)
	for i, h := range f.histograms {
		formatters[i] = NewHistogramFormatter(h, f.barChar, f.graphWidth, f.fracPrec)
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
		countAndBarMaxWidth := countWidths[i] + barMaxWidth
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
	fracPrec   uint
	barChar    string
	graphWidth int
}

func NewHistogramFormatter(histogram *Histogram[float64], barChar string, graphWidth int, fracPrec uint) *HistogramFormatter {
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
		fracPrec:   fracPrec,
	}
}

func (f *HistogramFormatter) RangeStrings() []string {
	tickWidth := 0
	ticks := make([]string, len(f.histogram.rangePoints))
	for i, tick := range f.histogram.rangePoints {
		s := fmt.Sprintf("%.*f", f.fracPrec, tick)
		ticks[i] = s
		tickWidth = Max(tickWidth, len(s))
	}

	ranges := make([]string, len(ticks)-1)
	for i := range ranges {
		ranges[i] = fmt.Sprintf("%*s ~ %*s",
			tickWidth, ticks[i],
			tickWidth, ticks[i+1])
	}
	return ranges
}

func (f *HistogramFormatter) CountStrings() []string {
	countWidth := 0
	countStrs := make([]string, len(f.histogram.counts))
	for i, count := range f.histogram.counts {
		s := strconv.Itoa(count)
		countStrs[i] = s
		countWidth = Max(countWidth, len(s))
	}

	for i, countStr := range countStrs {
		countStrs[i] = padStartSpace(countWidth, countStr)
	}
	return countStrs
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

	bars := make([]string, len(f.histogram.counts))
	for i, count := range f.histogram.counts {
		barWidth := int(float64(count) * barWidthRatio)
		if padEnd {
			bars[i] = strings.Repeat(f.barChar, barWidth) + strings.Repeat(" ", barMaxWidth-barWidth)
		} else {
			bars[i] = strings.Repeat(f.barChar, barWidth)
		}
	}
	return bars
}

func (f *HistogramFormatter) LineStrings(graphWidth int, barChar string, padEnd bool) []string {
	ranges := f.RangeStrings()
	counts := f.CountStrings()

	rangeWidth := len(ranges[0])
	countWidth := len(counts[0])
	barMaxWidth := graphWidth - (rangeWidth + len(" ") + countWidth + len(" |"))

	maxCount := f.histogram.MaxCount()
	barWidthRatio := float64(0)
	if maxCount != 0 {
		barWidthRatio = float64(barMaxWidth) / (float64(maxCount) * float64(len(barChar)))
	}

	bars := f.BarStrings(barMaxWidth, barWidthRatio, barChar, padEnd)

	lines := make([]string, len(ranges))
	for i := range lines {
		lines[i] = fmt.Sprintf("%s %s |%s", ranges[i], counts[i], bars[i])
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
	rangePoints []T
	counts      []int
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

func adjustMax(max float64) float64 {
	if max < 0 {
		panic("negative max")
	}

	s := fmt.Sprintf("%.1e", max)
	// s is like 4.6e+01
	d1 := mustAtoi(s[0:1])
	d2 := mustAtoi(s[2:3])
	exp := mustAtoi(s[4:])
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
