// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aclements/cpus/internal/cpuinfo"
)

func filterAndSort(m *cpuinfo.Machine, sysfs fs.FS, toolArgs []string) ([]*cpuinfo.Thread, error) {
	filterRegexp := regexp.MustCompile(`^([a-z]+)(==|!=|<=|>=|<|>)([0-9]+)$`)

	var sorters []string
	selection := m.Threads

	for _, arg := range toolArgs {
		if isRange(arg) {
			cpus, err := cpuinfo.ParseRange(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid range %q: %v", arg, err)
			}
			set := make(map[int]bool)
			for _, cpu := range cpus {
				set[cpu] = true
			}
			selection = filterThreads(selection, func(t *cpuinfo.Thread) bool {
				return set[t.ID]
			})
		} else if arg == "present" || arg == "possible" || arg == "online" || arg == "offline" {
			set := getSet(sysfs, arg)
			if set != nil {
				selection = filterThreads(selection, func(t *cpuinfo.Thread) bool {
					return set[t.ID]
				})
			}
		} else if matches := filterRegexp.FindStringSubmatch(arg); matches != nil {
			field := matches[1]
			op := matches[2]
			val, _ := strconv.Atoi(matches[3])
			proj := projections[field]
			if proj == nil {
				return nil, fmt.Errorf("unknown field: %s", field)
			}
			selection = filterThreads(selection, func(t *cpuinfo.Thread) bool {
				fieldVal := proj(t)

				switch op {
				case "==":
					return fieldVal == val
				case "!=":
					return fieldVal != val
				case "<":
					return fieldVal < val
				case "<=":
					return fieldVal <= val
				case ">":
					return fieldVal > val
				case ">=":
					return fieldVal >= val
				default:
					return false
				}
			})
		} else if arg == "rr" {
			sorters = append(sorters, "thread", "node", "socket", "die", "core")
		} else if _, ok := projections[arg]; ok {
			sorters = append(sorters, arg)
		} else {
			return nil, fmt.Errorf("unknown filter/sort %q", arg)
		}
	}

	// Apply default initial sort as fallback
	sorters = append(sorters, "node", "socket", "die", "core", "thread")

	// Apply sorting
	if len(sorters) > 0 {
		type projection func(*cpuinfo.Thread) int
		var resolvedSorters []projection
		for _, sorter := range sorters {
			proj := projections[sorter]
			if proj == nil {
				return nil, fmt.Errorf("unknown sorter: %s", sorter)
			}
			resolvedSorters = append(resolvedSorters, proj)
		}

		sort.SliceStable(selection, func(i, j int) bool {
			ti, tj := selection[i], selection[j]
			for _, proj := range resolvedSorters {
				vi, vj := proj(ti), proj(tj)
				if vi != vj {
					return vi < vj
				}
			}
			return false
		})
	}

	return selection, nil
}

var projections = map[string]func(*cpuinfo.Thread) int{
	"processor": func(t *cpuinfo.Thread) int { return t.ID },
	"socket":    func(t *cpuinfo.Thread) int { return t.Core.Die.Socket.ID },
	"die":       func(t *cpuinfo.Thread) int { return t.Core.Die.ID },
	"core":      func(t *cpuinfo.Thread) int { return t.Core.ID },
	"thread":    func(t *cpuinfo.Thread) int { return t.CoreIndex },
	"node":      func(t *cpuinfo.Thread) int { return t.Node.ID },
}

func isRange(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "-") || strings.Contains(s, ",") {
		return true
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func filterThreads(threads []*cpuinfo.Thread, f func(*cpuinfo.Thread) bool) []*cpuinfo.Thread {
	var res []*cpuinfo.Thread
	for _, t := range threads {
		if f(t) {
			res = append(res, t)
		}
	}
	return res
}
