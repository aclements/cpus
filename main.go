// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/sys/unix"

	"github.com/aclements/cpus/internal/cpuinfo"
)

//go:embed doc.go
var docStr string

func main() {
	flag.Usage = func() {
		progRe := regexp.MustCompile(`\bcpus\b`)
		progRepl := filepath.Base(os.Args[0])
		var lines []string
		flagIndex := -1
		for line := range strings.SplitSeq(docStr, "\n") {
			if line == "" {
				lines = lines[:0]
			} else if strings.HasPrefix(line, "package") {
				break
			}
			if after, ok := strings.CutPrefix(line, "//"); ok {
				after = strings.TrimPrefix(after, " ")
				if strings.HasPrefix(after, "\t") {
					after = "    " + progRe.ReplaceAllLiteralString(after[1:], progRepl)
				}
				after = progRe.ReplaceAllString(after, progRepl)
				lines = append(lines, after)
			}
			if strings.Contains(line, "[flags]") {
				flagIndex = len(lines)
			}
		}

		w := flag.CommandLine.Output()
		fmt.Fprintln(w, strings.Join(lines[:flagIndex], "\n"))
		fmt.Fprintf(w, "\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintln(w, strings.Join(lines[flagIndex:], "\n"))
	}

	hotplugFlag := flag.Bool("hotplug", false, "enable/disable CPUs via hotplug")
	formatFlag := flag.String("format", "", "output format: compact (default), comma, space, table")
	var limitFlag sweepFlag
	flag.Var(&limitFlag, "limit", "limit to `n` processors, or sweep [n]..[m][..incr] processors")
	startFlag := flag.Int("start", 0, "skip the first `start` matching processors")

	flag.Parse()

	// Find "--" to separate tool args from command in flag.Args()
	var cmdArgs []string
	toolArgs := flag.Args()
	for i, arg := range toolArgs {
		if arg == "--" {
			cmdArgs = toolArgs[i+1:]
			toolArgs = toolArgs[:i]
			break
		}
	}

	var mode string
	switch {
	case len(cmdArgs) > 0:
		if *hotplugFlag {
			fmt.Fprintf(os.Stderr, "Cannot use -hotplug with a command\n")
			os.Exit(1)
		}
		mode = "taskset"
	case *hotplugFlag:
		mode = "hotplug"
	case len(toolArgs) == 0 && *formatFlag == "" && len(limitFlag) == 0 && *startFlag == 0:
		// No filters, sorters, or command.
		flag.Usage()
		os.Exit(1)
	default:
		mode = "list"
	}
	if *formatFlag == "" {
		*formatFlag = "compact"
	}

	sysfs := os.DirFS("/")
	m, err := cpuinfo.Load(sysfs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading CPU info: %v\n", err)
		os.Exit(1)
	}

	selection, err := filterAndSort(m, sysfs, toolArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing threads: %v\n", err)
		os.Exit(1)
	}

	// Apply start and limit
	if *startFlag > 0 {
		if *startFlag < len(selection) {
			selection = selection[*startFlag:]
		} else {
			selection = nil
		}
	}

	limits := limitFlag.Sequence(len(selection))
	if mode == "hotplug" && len(limits) != 1 {
		fmt.Fprintf(os.Stderr, "Cannot use -hotplug with a -limit sweep\n")
		os.Exit(1)
	}

	for _, limit := range limits {
		sel1 := selection[:limit]
		switch mode {
		case "taskset":
			if err := doTaskset(threadIDs(sel1), cmdArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

		case "hotplug":
			if err := doHotplug(realFS, threadIDs(sel1)); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

		case "list":
			if err := doList(os.Stdout, m, sel1, *formatFlag); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}
}

func threadIDs(selection []*cpuinfo.Thread) []int {
	var ids []int
	for _, t := range selection {
		ids = append(ids, t.ID)
	}
	return ids
}

func getSet(sysfs fs.FS, name string) map[int]bool {
	cpus, err := cpuinfo.GetCPUSet(sysfs, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read CPU set %q: %v\n", name, err)
		return nil
	}
	set := make(map[int]bool)
	for _, cpu := range cpus {
		set[cpu] = true
	}
	return set
}

func doTaskset(resolvedIDs []int, cmdArgs []string) error {
	var mask unix.CPUSet
	for _, id := range resolvedIDs {
		mask.Set(id)
	}

	err := unix.SchedSetaffinity(0, &mask)
	if err != nil {
		return fmt.Errorf("setting affinity: %v", err)
	}

	// Ignore signals in the parent so they go to the child.
	signal.Ignore(os.Interrupt, syscall.SIGQUIT)
	defer signal.Reset(os.Interrupt, syscall.SIGQUIT)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

type writeFileFS interface {
	fs.FS
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

var realFS = realWriteFS{os.DirFS("/")}

type realWriteFS struct {
	fs.FS // Must be os.DirFS("/")
}

func (r realWriteFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile("/"+name, data, perm)
}

func doHotplug(sysfs writeFileFS, resolvedIDs []int) error {
	presentSet := getSet(sysfs, "present")
	possibleSet := getSet(sysfs, "possible")
	onlineSet := getSet(sysfs, "online")

	if presentSet == nil || possibleSet == nil || onlineSet == nil {
		return fmt.Errorf("error reading CPU sets for hotplug")
	}

	wantOnline := make(map[int]bool)
	for _, id := range resolvedIDs {
		wantOnline[id] = true
	}

	// Check if wanted CPUs are present and possible
	for id := range wantOnline {
		if !presentSet[id] {
			return fmt.Errorf("CPU %d is not present", id)
		}
		if !possibleSet[id] {
			return fmt.Errorf("CPU %d is not possible", id)
		}
	}

	// Determine CPUs to enable and disable
	var enable []int
	var disable []int

	for id := range wantOnline {
		if !onlineSet[id] {
			enable = append(enable, id)
		}
	}

	for id := range presentSet {
		if !wantOnline[id] && onlineSet[id] {
			disable = append(disable, id)
		}
	}

	fmt.Fprintf(os.Stderr, "Enabling: %s\n", cpuinfo.StrRange(enable))
	fmt.Fprintf(os.Stderr, "Disabling: %s\n", cpuinfo.StrRange(disable))

	// Perform hotplug. We have to bring CPUs online first or else we may end up
	// with zero online CPUs.
	for _, id := range enable {
		path := fmt.Sprintf("sys/devices/system/cpu/cpu%d/online", id)
		err := sysfs.WriteFile(path, []byte("1\n"), 0644)
		if err != nil {
			return fmt.Errorf("error enabling CPU %d: %v", id, err)
		}
	}

	for _, id := range disable {
		path := fmt.Sprintf("sys/devices/system/cpu/cpu%d/online", id)
		err := sysfs.WriteFile(path, []byte("0\n"), 0644)
		if err != nil {
			return fmt.Errorf("error disabling CPU %d: %v", id, err)
		}
	}
	return nil
}

func doList(w io.Writer, m *cpuinfo.Machine, selection []*cpuinfo.Thread, format string) error {
	switch format {
	case "compact":
		fmt.Fprintln(w, cpuinfo.StrRange(threadIDs(selection)))
	case "comma":
		var s []string
		for _, id := range threadIDs(selection) {
			s = append(s, strconv.Itoa(id))
		}
		fmt.Fprintln(w, strings.Join(s, ","))
	case "space":
		var s []string
		for _, id := range threadIDs(selection) {
			s = append(s, strconv.Itoa(id))
		}
		fmt.Fprintln(w, strings.Join(s, " "))
	case "table":
		columns := []string{"node", "socket", "die", "core", "thread", "processor"}
		if shouldHideDie(m) {
			i := slices.Index(columns, "die")
			columns = slices.Replace(columns, i, i+1)
		}

		tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
		fmt.Fprintln(tw, strings.Join(columns, "\t"))
		for _, t := range selection {
			var vals []string
			for _, col := range columns {
				proj := projections[col]
				vals = append(vals, strconv.Itoa(proj(t)))
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		tw.Flush()
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
	return nil
}

func shouldHideDie(m *cpuinfo.Machine) bool {
	for _, s := range m.Sockets {
		if len(s.Dies) > 1 {
			return false
		}
	}
	return true
}
