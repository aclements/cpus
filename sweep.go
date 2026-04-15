// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

type sweepTerm struct {
	// start=0 or end=0 are sentinels for "n". If incr is 0, it was never set
	start, end, incr int
}

func (s *sweepTerm) String() string {
	if s.start == s.end {
		if s.start == 0 {
			return "N"
		}
		return strconv.Itoa(s.start)
	}
	var sb strings.Builder
	if s.start == 0 {
		sb.WriteString("N")
	} else if s.start != 1 || s.end == 0 {
		sb.WriteString(strconv.Itoa(s.start))
	}
	sb.WriteString("..")
	if s.end != 0 {
		sb.WriteString(strconv.Itoa(s.end))
	} else if s.incr != 1 {
		sb.WriteString("N")
	}
	if s.incr != 1 {
		sb.WriteString("..")
		sb.WriteString(strconv.Itoa(s.incr))
	}
	return sb.String()
}

func parseSweepTerm(value string) (sweepTerm, error) {
	var s sweepTerm
	s.incr = 1

	if !strings.Contains(value, "..") {
		if value == "N" || value == "n" {
			s.start, s.end = 0, 0
			return s, nil
		}
		v, err := strconv.Atoi(value)
		if err != nil {
			return s, err
		}
		s.start, s.end = v, v
		return s, nil
	}

	parts := strings.Split(value, "..")
	if len(parts) > 3 {
		return s, fmt.Errorf("invalid sweep syntax")
	}
	// Reject "...." (empty middle part when increment is present)
	if len(parts) == 3 && parts[1] == "" {
		return s, fmt.Errorf("sweep limit must be specified when increment is present (use 'N' for maximum)")
	}

	s.start = 1
	s.end = 0 // 0 means max

	parseNum := func(label string, arg string, maxOK bool) (int, error) {
		if maxOK && (arg == "N" || arg == "n") {
			return 0, nil
		}
		v, err := strconv.Atoi(arg)
		if err != nil {
			return 0, err
		}
		if v <= 0 {
			return 0, fmt.Errorf("sweep %s must be > 0", label)
		}
		return v, nil
	}

	var err error
	if parts[0] != "" {
		s.start, err = parseNum("start", parts[0], true)
		if err != nil {
			return s, err
		}
	}

	if len(parts) > 1 && parts[1] != "" {
		s.end, err = parseNum("limit", parts[1], true)
		if err != nil {
			return s, err
		}
	}

	if len(parts) > 2 && parts[2] != "" {
		s.incr, err = parseNum("increment", parts[2], false)
		if err != nil {
			return s, err
		}
	}

	if s.end != 0 && s.start > s.end {
		return s, fmt.Errorf("empty sweep %q", value)
	}

	return s, nil
}

func (s sweepTerm) Sequence(max int) []int {
	start := s.start
	if start == 0 {
		start = max
	}

	end := s.end
	if end == 0 || end > max {
		end = max
	}

	var seq []int
	for i := start; i <= end; i += s.incr {
		seq = append(seq, i)
	}
	return seq
}

type sweepFlag []sweepTerm

func (s sweepFlag) String() string {
	if len(s) == 0 {
		return "N"
	}
	var sb strings.Builder
	for i, term := range s {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(term.String())
	}
	return sb.String()
}

func (s *sweepFlag) Set(value string) error {
	parts := strings.FieldsFunc(value, func(c rune) bool {
		return c == ' ' || c == ','
	})

	if len(parts) == 0 {
		return fmt.Errorf("empty sweep")
	}

	*s = nil
	for _, part := range parts {
		term, err := parseSweepTerm(part)
		if err != nil {
			return err
		}
		*s = append(*s, term)
	}
	return nil
}

func (s sweepFlag) Sequence(max int) []int {
	if len(s) == 0 {
		return []int{max}
	}
	var seq []int
	for _, term := range s {
		seq = append(seq, term.Sequence(max)...)
	}
	return seq
}
