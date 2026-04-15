// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpuinfo

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// ParseRange parses a range string like "0-3,8-11" into a list of ints.
func ParseRange(r string) ([]int, error) {
	r = strings.TrimSpace(r)
	if r == "" {
		return []int{}, nil
	}
	var res []int
	for piece := range strings.SplitSeq(r, ",") {
		lr := strings.Split(piece, "-")
		if len(lr) == 1 {
			n, err := strconv.Atoi(strings.TrimSpace(lr[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range syntax: %q", r)
			}
			res = append(res, n)
		} else if len(lr) == 2 {
			start, err := strconv.Atoi(strings.TrimSpace(lr[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range syntax: %q", r)
			}
			end, err := strconv.Atoi(strings.TrimSpace(lr[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range syntax: %q", r)
			}
			for i := start; i <= end; i++ {
				res = append(res, i)
			}
		} else {
			return nil, fmt.Errorf("invalid range syntax: %q", r)
		}
	}
	slices.Sort(res)
	return res, nil
}

// StrRange formats a list of ints into a range string like "0-3,8-11".
func StrRange(cpus []int) string {
	if len(cpus) == 0 {
		return ""
	}
	if !slices.IsSorted(cpus) {
		cpus = slices.Clone(cpus)
		slices.Sort(cpus)
	}

	var spans [][]int
	for _, cpu := range cpus {
		if len(spans) > 0 && cpu == spans[len(spans)-1][1]+1 {
			spans[len(spans)-1][1] = cpu
		} else {
			spans = append(spans, []int{cpu, cpu})
		}
	}

	var pieces []string
	for _, s := range spans {
		if s[0] == s[1] {
			pieces = append(pieces, fmt.Sprintf("%d", s[0]))
		} else if s[1] == s[0]+1 {
			pieces = append(pieces, fmt.Sprintf("%d,%d", s[0], s[1]))
		} else {
			pieces = append(pieces, fmt.Sprintf("%d-%d", s[0], s[1]))
		}
	}
	return strings.Join(pieces, ",")
}

// GetCPUSet reads a CPU set range from sysfs (e.g., "present", "online").
func GetCPUSet(sysfs fs.FS, name string) ([]int, error) {
	path := "sys/devices/system/cpu/" + name
	b, err := fs.ReadFile(sysfs, path)
	if err != nil {
		return nil, err
	}
	return ParseRange(string(b))
}
