// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"slices"
	"testing"
)

func TestSweepValue(t *testing.T) {
	tests := []struct {
		input   string
		wantStr string
		max     int
		wantSeq []int
		wantErr bool
	}{
		// Single terms
		{"1..5..2", "..5..2", 10, []int{1, 3, 5}, false},
		{"..5..2", "..5..2", 10, []int{1, 3, 5}, false},
		{"1..5", "..5", 10, []int{1, 2, 3, 4, 5}, false},
		{"..5", "..5", 10, []int{1, 2, 3, 4, 5}, false},
		{"1..", "1..", 5, []int{1, 2, 3, 4, 5}, false},
		{"..", "1..", 5, []int{1, 2, 3, 4, 5}, false},
		{"5", "5", 10, []int{5}, false},
		{"N", "N", 5, []int{5}, false},

		// Cases with "N"
		{"n", "N", 5, []int{5}, false},
		{"N..", "N", 5, []int{5}, false},
		{"..N..2", "1..N..2", 5, []int{1, 3, 5}, false},
		{"1..N..2", "1..N..2", 5, []int{1, 3, 5}, false},
		{"..N", "1..", 5, []int{1, 2, 3, 4, 5}, false},
		{"1..N", "1..", 5, []int{1, 2, 3, 4, 5}, false},
		{"N..5", "N..5", 2, []int{2}, false},

		// Multiple terms
		{"1,2..5", "1,2..5", 10, []int{1, 2, 3, 4, 5}, false},
		{"1 2..5", "1,2..5", 10, []int{1, 2, 3, 4, 5}, false},
		{"N,1..2", "N,..2", 5, []int{5, 1, 2}, false},
		{"1..3,5..7", "..3,5..7", 10, []int{1, 2, 3, 5, 6, 7}, false},

		// Invalid cases
		{"invalid", "", 0, nil, true},
		{"1..2..3..4", "", 0, nil, true},
		{"1....2", "", 0, nil, true},
		{"....2", "", 0, nil, true},
		{"2..1", "", 0, nil, true},
		{"1..invalid", "", 0, nil, true},
		{"1..5..invalid", "", 0, nil, true},
		{"0..5", "", 0, nil, true},
		{"", "", 0, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var s sweepFlag
			err := s.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if s.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", s.String(), tt.wantStr)
			}

			seq := s.Sequence(tt.max)
			if !slices.Equal(seq, tt.wantSeq) {
				t.Errorf("Sequence(%d) = %v, want %v", tt.max, seq, tt.wantSeq)
			}
		})
	}
}

func TestSweepZeroValue(t *testing.T) {
	var s sweepFlag
	if s.String() != "N" {
		t.Errorf("zero value String() = %q, want \"N\"", s.String())
	}
	seq := s.Sequence(5)
	want := []int{5}
	if !slices.Equal(seq, want) {
		t.Errorf("zero value Sequence(5) = %v, want %v", seq, want)
	}
}
