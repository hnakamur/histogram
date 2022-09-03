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
)

func main() {
	bucketCount := flag.Int("bucket-count", 10, "histogram bucket count")
	axisMin := flag.Float64("axis-min", 0, "axis minimum value")
	axisMax := flag.Float64("axis-max", 10, "axis maximum value")
	fixedAxis := flag.Bool("fixed-axis", true, "if enabled, axis min and max are fixed even if some of values are out of range")
	flag.Parse()

	if err := run(*bucketCount, *axisMin, *axisMax, *fixedAxis); err != nil {
		log.Fatal(err)
	}
}

func run(bucketCount int, axisMin, axisMax float64, fixedAxis bool) error {
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

	log.Printf("min=%f, max=%f, axisMin=%f, axisMax=%f", min, max, axisMin, axisMax)
	ticks := buildAxisTicks(bucketCount, axisMin, axisMax)
	log.Printf("ticks=%v", ticks)

	for _, v := range values {
		i := sort.SearchFloat64s(ticks, v)
		log.Printf("v=%f, i=%d", v, i)
	}

	return nil
}

func readFloatValues(r io.Reader) ([]float64, error) {
	var values []float64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		value, err := strconv.ParseFloat(scanner.Text(), 64)
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

type Number interface {
	constraints.Integer | constraints.Float
}

const defaultBarChar = "*"

type Histogram[T Number] struct {
	buckets    *Buckets[T]
	barChar    string
	graphWidth int
}

func NewHistogram[T Number](buckets *Buckets[T], barChar string, graphWidth int) *Histogram[T] {
	return &Histogram[T]{buckets: buckets, barChar: barChar, graphWidth: graphWidth}
}

func (h *Histogram[T]) String() string {
	var b strings.Builder
	// const valueWidth = 6
	// barMaxWidth := h.graphWidth - valueWidth - 2
	for i, start := range h.buckets.ticks {
		count := h.buckets.counts[i]
		_, _ = fmt.Fprintf(&b, "%6.2f: %s\n", start, strings.Repeat(h.barChar, count))
	}
	return b.String()
}

type Buckets[T Number] struct {
	ticks  []T
	counts []int
}

func NewBuckets[T Number](count int, min, max T) *Buckets[T] {
	ticks := buildAxisTicks(count, min, max)
	counts := make([]int, len(ticks))
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
	i := sort.Search(len(b.ticks), func(i int) bool { return b.ticks[i] > v })
	if v != b.ticks[i] {
		i--
	}
	b.counts[i]++
}

type Range[T constraints.Ordered] struct {
	Start T // inclusive
	End   T // exclusive
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
