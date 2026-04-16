// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The cpus command provides a simple expression language to select a set of
// CPUs and run a command limited to those CPUs.
//
// Usage:
//
//	cpus [flags] [filters/sorters...] [-- command args...]
//
// By default, cpus runs the given command with a restricted CPU set. If -limit
// is a sweep expression, it runs the command for each limit in the sweep.
//
// If no command is provided, it prints the matching CPUs.
//
// If the -hotplug flag is provided, it uses CPU hotplug to enable only the
// matching CPUs. This ensures at a kernel level that nothing can be
// interferring from other CPUs, but also requires root access and may not be
// supported by all hardware.
//
// Sorters specify how to sort the list of CPUs and can be used to generate
// different sequences of CPUs. This is useful even for CPU masks in conjunction
// with the -limit flag. Sorters are applied to the list of CPUs in the order
// given on the command line. For example,
//
//	cpus -limit 4 core thread
//
// will list all of the threads within each core. Any field can be given as a
// sorter. In addition, 'rr' is an alias for 'thread node socket die core',
// which is a "round robin" order that fills the first hardware thread across
// the machine before populating additional hardware threads. The default
// initial sort is 'node socket die core thread'.
//
// Filters restrict the set of CPUs and are either a CPU mask ("0-6,10"), an
// expression "<field><op><val>" where <op> is ==, !=, <, <=, >, or >=, or a
// named filter. For example,
//
//	cpus thread==0
//
// restricts to hardware thread 0 across all cores. The named filters are
// 'present', 'possible', 'online', and 'offline'.
//
// # Fields
//
//	socket     Physical CPU socket or package ID
//	die        Die index within a multi-die socket (if applicable)
//	core       Core index within a socket/die (different threads in the same
//	           core will have equal core IDs)
//	thread     Hardware thread index within a core
//	processor  Processor's global number (each hardware thread counts
//	           as a 'processor').
//	node       NUMA node ID
//
// # Sweeps
//
// The -limit flag accepts a "sweep" expression, which will run the command or
// print the matching CPUs for a series of limits. The general form of a sweep
// is a space- or comma-separated list of terms, where each term is either a
// single number, "N", or "start..end..incr". "start" may be omitted and
// defaults to 1. "end" may be omitted and defaults to "N". "..incr" may be
// omitted and defaults to 1. "start" or "end" can be "N", which represents the
// max (the number of processes minus the -start flag).
//
// # Examples
//
// To see the topology of your system, run:
//
//	./cpus --format table
//
// On an 88 CPU machine with two hyperthreads per core and multiple sockets and
// NUMA nodes, this may print something like
//
//	node socket core thread processor
//	0    0      0    0      0
//	0    0      0    1      44
//	0    0      1    0      1
//	0    0      1    1      45
//	...
//	1    1      43   0      43
//	1    1      43   1      87
//
// If we want to run a command without hyperthread interference, we can `cpus` in
// "taskset" mode with a filter:
//
//	./cpus thread==0 -- ls
//
// If we want a command to have 8 CPUs, preferring to keep it on a single NUMA
// node, we can use a sorter with a limit.
//
//	./cpus -limit 8 node -- ls
//
// # Benchmark examples
//
// We can use "sweep" syntax to run scalability benchmarks across a sequence of CPU
// masks. A basic sweep looks like
//
//	./cpus -limit 1.. -- go test . -run ^$ -bench .
//
// This will run a Go benchmark with 1 CPU, then 2 CPUs, etc.
//
// We can combine this with filters and sorters to achieve complex effects. For
// example, to minimize hyperthread interactions, we can limit the sweep to only
// run on the first hyperthread of each core with
//
//	./cpus -limit 1.. thread==0 -- go test . -run ^$ -bench .
//
// We can use a sort to make sure the benchmark fills up resources in a predictable
// order. For example, this uses the "round robin" order to fill up each socket and
// NUMA node before spilling on the next, while still limiting the command to one
// hyperthread
//
//	./cpus -limit 1.. thread==0 rr -- go test . -run ^$ -bench .
//
// Sweeps can also be used to print CPU sets that can be passed to other commands.
// The following command uses a shell loop with `taskset` to implement CPU masks
//
//	for mask in $(cpus -limit 1.. rr); do taskset -c $mask ls; done
package main
