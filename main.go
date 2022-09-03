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
	values, err := readFloatValues(os.Stdin)
	if err != nil {
		return err
	}

	if len(values) == 0 {
		return errors.New("no value from stdin")
	}

	min, max := MinMaxInSlice(values)
	if !fixedAxis {
		axisMin = Min(axisMin, min)
		axisMax = Max(axisMax, max)
	}

	buckets := NewBuckets(bucketCount, axisMin, axisMax)
	for _, v := range values {
		buckets.AddValue(v)
	}

	histogram := NewHistogram(buckets, defaultBarChar, graphWidth)
	fmt.Print(histogram)

	return nil
}

const float64BitSize = 64

func readFloatValues(r io.Reader) ([]float64, error) {
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

type Histogram struct {
	buckets    *Buckets[float64]
	barChar    string
	graphWidth int
}

func NewHistogram(buckets *Buckets[float64], barChar string, graphWidth int) *Histogram {
	if len(barChar) == 0 {
		panic("barChar must not be empty")
	}
	if graphWidth == 0 {
		panic("graphWidth too small")
	}
	return &Histogram{buckets: buckets, barChar: barChar, graphWidth: graphWidth}
}

func (h *Histogram) String() string {
	var b strings.Builder

	n := len(h.buckets.ticks) - 1

	tickWidth := 0
	tickStrs := make([]string, n+1)
	for i := 0; i <= n; i++ {
		s := fmt.Sprintf("%.2f", h.buckets.ticks[i])
		tickStrs[i] = s
		tickWidth = Max(tickWidth, len(s))
	}

	maxCount := 0
	countWidth := 0
	countStrs := make([]string, n)
	for i := 0; i < n; i++ {
		count := h.buckets.counts[i]
		s := strconv.Itoa(count)
		countStrs[i] = s
		countWidth = Max(countWidth, len(s))
		maxCount = Max(maxCount, count)
	}

	barMaxWidth := h.graphWidth - (tickWidth + len(" ~ ") + tickWidth + len(" [ ") + countWidth + len(" ] "))
	if barMaxWidth <= barMinWidth {
		log.Fatalf("bar max width becomes too small, retry with larger graphWidth, barMaxWidth=%d, graphWidth=%d", barMaxWidth, h.graphWidth)
	}

	barRatio := float64(0)
	if maxCount != 0 {
		barRatio = float64(barMaxWidth) / (float64(maxCount) * float64(len(h.barChar)))
	}

	barWidths := make([]int, n)
	for i := 0; i < n; i++ {
		count := h.buckets.counts[i]
		barWidths[i] = int(float64(count) * barRatio)
	}

	for i := 0; i < n; i++ {
		_, _ = fmt.Fprintf(&b, "%*s ~ %*s [ %*s ] %s\n",
			tickWidth, tickStrs[i],
			tickWidth, tickStrs[i+1],
			countWidth, countStrs[i],
			strings.Repeat(h.barChar, barWidths[i]))
	}
	return b.String()
}

type Number interface {
	constraints.Integer | constraints.Float
}

type Buckets[T Number] struct {
	ticks  []T
	counts []int
}

func NewBuckets[T Number](count int, min, max T) *Buckets[T] {
	ticks := buildAxisTicks(count, min, max)
	counts := make([]int, len(ticks)-1)
	return &Buckets[T]{ticks: ticks, counts: counts}
}

func buildAxisTicks[T Number](count int, min, max T) []T {
	ticks := make([]T, count+1)
	for i := 0; i <= count; i++ {
		ticks[i] = min + (max-min)*T(i)/T(count)
	}
	return ticks
}

func (b *Buckets[T]) AddValue(v T) {
	i := sort.Search(len(b.ticks), func(i int) bool { return b.ticks[i] > v }) - 1
	if i < len(b.counts) {
		b.counts[i]++
	}
}

func (b *Buckets[T]) Equal(o *Buckets[T]) bool {
	return slices.Equal(b.ticks, o.ticks) && slices.Equal(b.counts, o.counts)
}

func MinMaxInSlice[T constraints.Ordered](values []T) (min T, max T) {
	min = values[0]
	max = values[0]
	for i := 1; i < len(values); i++ {
		min = Min(min, values[i])
		max = Max(max, values[i])
	}
	return min, max
}

func Min[T constraints.Ordered](x, y T) T {
	if x < y {
		return x
	}
	return y
}

func Max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	}
	return y
}
