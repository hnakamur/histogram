package main

import (
	"sort"
	"testing"
)

func TestSearchFloat64s(t *testing.T) {
	ticks := []float64{0, 1, 2, 3, 4}
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
		got := sort.SearchFloat64s(ticks, tc.input)
		if got != tc.want {
			t.Errorf("result mismatch, input=%f, got=%d, want=%d", tc.input, got, tc.want)
		}
	}
}
