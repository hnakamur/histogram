package main

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"golang.org/x/exp/slices"
)

func TestSearchFloat64s(t *testing.T) {
	rangePoints := []float64{0, 1, 2, 3, 4}
	testCases := []struct {
		input float64
		want  int
	}{
		{input: 0, want: 0},
		{input: 0.9, want: 1},
		{input: 1, want: 1},
		{input: 1.2, want: 2},
		{input: 3.9, want: 4},
		{input: 4, want: 4},
		{input: 4.1, want: 5},
	}
	for _, tc := range testCases {
		got := sort.SearchFloat64s(rangePoints, tc.input)
		if got != tc.want {
			t.Errorf("result mismatch, input=%f, got=%d, want=%d", tc.input, got, tc.want)
		}
	}
}

func TestSortSearch(t *testing.T) {
	rangePoints := []float64{0, 1, 2, 3, 4}
	testCases := []struct {
		input float64
		want  int
	}{
		{input: 0, want: 1},
		{input: 0.9, want: 1},
		{input: 1, want: 2},
		{input: 1.2, want: 2},
		{input: 3.9, want: 4},
		{input: 4, want: 5},
		{input: 4.1, want: 5},
	}
	for _, tc := range testCases {
		got := sort.Search(len(rangePoints), func(i int) bool { return rangePoints[i] > tc.input })
		if got != tc.want {
			t.Errorf("result mismatch, input=%f, got=%d, want=%d", tc.input, got, tc.want)
		}
	}
}

func TestHistogram_AddValue(t *testing.T) {
	testCases := []struct {
		inputs []float64
		want   []int
	}{
		{inputs: []float64{0}, want: []int{1, 0, 0, 0, 0}},
		{inputs: []float64{0.5}, want: []int{1, 0, 0, 0, 0}},
		{inputs: []float64{0.99}, want: []int{1, 0, 0, 0, 0}},
		{inputs: []float64{1}, want: []int{0, 1, 0, 0, 0}},
		{inputs: []float64{0, 1, 1}, want: []int{1, 2, 0, 0, 0}},
		{inputs: []float64{4.9999}, want: []int{0, 0, 0, 0, 1}},
		{inputs: []float64{5}, want: []int{0, 0, 0, 0, 0}},
	}
	for _, tc := range testCases {
		h := NewHistogram(BuildRangePoints[float64](5, 0, 5))
		for _, v := range tc.inputs {
			h.AddValue(v)
		}
		if got, want := h.RangePoints(), []float64{0, 1, 2, 3, 4, 5}; !slices.Equal(got, want) {
			t.Errorf("ticks mismatch, testCase=%+v, got=%v, want=%v", tc, got, want)
		}
		if got, want := h.Counts(), tc.want; !slices.Equal(got, want) {
			t.Errorf("counts mismatch, testCase=%+v, got=%v, want=%v", tc, got, want)
		}
		if got, want := h, (&Histogram[float64]{rangePoints: []float64{0, 1, 2, 3, 4, 5}, counts: tc.want}); !got.Equal(want) {
			t.Errorf("counts mismatch, testCase=%+v, got=%v, want=%v", tc, got, want)
		}
	}
}

func TestHistogramFormatter(t *testing.T) {
	t.Run("case1", func(t *testing.T) {
		histogram := NewHistogram(BuildRangePoints[float64](10, 0, 10))
		for i := 0; i < 10; i++ {
			for j := 0; j < i*2; j++ {
				histogram.AddValue(float64(i))
			}
		}

		formatter := NewHistogramFormatter(histogram, defaultBarChar, 40, 2)
		got := formatter.String()
		want := ` 0.00 ~  1.00 [  0 ] 
 1.00 ~  2.00 [  2 ] **
 2.00 ~  3.00 [  4 ] ****
 3.00 ~  4.00 [  6 ] ******
 4.00 ~  5.00 [  8 ] ********
 5.00 ~  6.00 [ 10 ] **********
 6.00 ~  7.00 [ 12 ] ************
 7.00 ~  8.00 [ 14 ] **************
 8.00 ~  9.00 [ 16 ] ****************
 9.00 ~ 10.00 [ 18 ] *******************
`
		if got != want {
			t.Errorf("result mismatch,\n got=%q,\nwant=%q", got, want)
		}
	})
	t.Run("allZero", func(t *testing.T) {
		histogram := NewHistogram(BuildRangePoints[float64](10, 0, 10))

		formatter := NewHistogramFormatter(histogram, defaultBarChar, 40, 2)
		got := formatter.String()
		want := ` 0.00 ~  1.00 [ 0 ] 
 1.00 ~  2.00 [ 0 ] 
 2.00 ~  3.00 [ 0 ] 
 3.00 ~  4.00 [ 0 ] 
 4.00 ~  5.00 [ 0 ] 
 5.00 ~  6.00 [ 0 ] 
 6.00 ~  7.00 [ 0 ] 
 7.00 ~  8.00 [ 0 ] 
 8.00 ~  9.00 [ 0 ] 
 9.00 ~ 10.00 [ 0 ] 
`
		if got != want {
			t.Errorf("result mismatch,\n got=%q,\nwant=%q", got, want)
		}
	})
}

func TestAdjustMax(t *testing.T) {
	testCases := []struct {
		input float64
		want  float64
	}{
		{input: 0, want: 0},
		{input: 1, want: 1},
		{input: 0.21, want: 0.22},
		{input: 0.22, want: 0.22},
		{input: 0.23, want: 0.24},
		{input: 0.25, want: 0.25},
		{input: 0.26, want: 0.26},
		{input: 0.27, want: 0.28},
		{input: 0.28, want: 0.28},
		{input: 0.29, want: 0.30},
		{input: 0.30, want: 0.30},
		{input: 0.235, want: 0.24},
		{input: 0.281, want: 0.30},
		{input: 0.2800001, want: 0.30},
		{input: 0.289, want: 0.30},
		{input: 0.99, want: 1.0},
		{input: 9.9, want: 10},
	}
	for _, tc := range testCases {
		got := adjustMax(tc.input)
		if got != tc.want {
			t.Errorf("result mismatch, input=%g, got=%g, want=%g", tc.input, got, tc.want)
		}
	}
}

func TestAdjustMaxProperty(t *testing.T) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	const n = 100000
	for i := 0; i < n; i++ {
		v := rnd.Float64()
		v2 := adjustMax(v)
		if v2 < v {
			t.Errorf("adjustMax output must not be smaller than input, input=%g, output=%g", v, v2)
		}
	}
}
