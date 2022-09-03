package main

import (
	"bufio"
	"errors"
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
	fixedAxis := flag.Bool("fixed-axis", false, "if enabled, axis min and max are fixed even if some of values are out of range")
	graphWidth := flag.Int("graph-width", 60, "graph column width including labels")
	flag.Parse()

	if err := run(*bucketCount, *axisMin, *axisMax, *fixedAxis, *graphWidth); err != nil {
		log.Fatal(err)
	}
}

func run(bucketCount int, axisMin, axisMax float64, fixedAxis bool, graphWidth int) error {
	values, err := readFloat64Values(os.Stdin)
	if err != nil {
		return err
	}

	if len(values) == 0 {
		return errors.New("no value from stdin")
	}

	min := Min(values...)
	max := Max(values...)
	if !fixedAxis {
		axisMin = Min(axisMin, min)
		axisMax = Max(axisMax, max)
	}

	histogram := NewHistogram(bucketCount, axisMin, axisMax)
	for _, v := range values {
		histogram.AddValue(v)
	}

	formatter := NewHistogramFormatter(histogram, defaultBarChar, graphWidth)
	fmt.Print(formatter)

	return nil
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

type HistogramFormatter struct {
	histogram  *Histogram[float64]
	barChar    string
	graphWidth int
}

func NewHistogramFormatter(histogram *Histogram[float64], barChar string, graphWidth int) *HistogramFormatter {
	if len(barChar) == 0 {
		panic("barChar must not be empty")
	}
	if graphWidth == 0 {
		panic("graphWidth too small")
	}
	return &HistogramFormatter{histogram: histogram, barChar: barChar, graphWidth: graphWidth}
}

func (h *HistogramFormatter) RangeStrings() []string {
	tickWidth := 0
	ticks := make([]string, len(h.histogram.rangePoints))
	for i, tick := range h.histogram.rangePoints {
		s := fmt.Sprintf("%.2f", tick)
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

func (h *HistogramFormatter) CountStrings() []string {
	countWidth := 0
	countStrs := make([]string, len(h.histogram.counts))
	for i, count := range h.histogram.counts {
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

func (h *HistogramFormatter) BarStrings(barMaxWidth int, barChar string, padEnd bool) []string {
	if barMaxWidth <= barMinWidth {
		log.Fatalf("bar max width becomes too small, retry with larger graphWidth, barMaxWidth=%d, graphWidth=%d", barMaxWidth, h.graphWidth)
	}

	maxCount := h.histogram.MaxCount()
	barRatio := float64(0)
	if maxCount != 0 {
		barRatio = float64(barMaxWidth) / (float64(maxCount) * float64(len(barChar)))
	}

	bars := make([]string, len(h.histogram.counts))
	for i, count := range h.histogram.counts {
		barWidth := int(float64(count) * barRatio)
		if padEnd {
			bars[i] = strings.Repeat(h.barChar, barWidth) + strings.Repeat(" ", barMaxWidth-barWidth)
		} else {
			bars[i] = strings.Repeat(h.barChar, barWidth)
		}
	}
	return bars
}

func (h *HistogramFormatter) LineStrings(graphWidth int, barChar string, padEnd bool) []string {
	ranges := h.RangeStrings()
	counts := h.CountStrings()

	rangeWidth := len(ranges[0])
	countWidth := len(counts[0])
	barMaxWidth := graphWidth - (rangeWidth + len(" [ ") + countWidth + len(" ] "))
	bars := h.BarStrings(barMaxWidth, barChar, padEnd)

	lines := make([]string, len(ranges))
	for i := range lines {
		lines[i] = fmt.Sprintf("%s [ %s ] %s", ranges[i], counts[i], bars[i])
	}
	return lines
}

func (h *HistogramFormatter) String() string {
	lines := h.LineStrings(h.graphWidth, h.barChar, false)
	return strings.Join(lines, "\n") + "\n"
}

type Number interface {
	constraints.Integer | constraints.Float
}

type Histogram[T Number] struct {
	rangePoints []T
	counts      []int
}

func NewHistogram[T Number](count int, min, max T) *Histogram[T] {
	rangePoints := buildRangePoints(count, min, max)
	counts := make([]int, len(rangePoints)-1)
	return &Histogram[T]{rangePoints: rangePoints, counts: counts}
}

func buildRangePoints[T Number](count int, min, max T) []T {
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
