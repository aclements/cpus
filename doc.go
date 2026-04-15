// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The cpus command provides a simple expression language to select a set of
// CPUs and run a command limited to those CPUs.
//
// Usage: cpus [flags] [filters/sorters...] [-- command args...]
//
// By default, cpus runs the given command with a restricted CPU set.
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
// Fields:
//
//	socket     Physical CPU socket or package ID
//	die        Die index within a multi-die socket (if applicable)
//	core       Core index within a socket/die (different threads in the same
//	           core will have equal core IDs)
//	thread     Hardware thread index within a core
//	processor  Processor's global number (each hardware thread counts
//	           as a 'processor').
//	node       NUMA node ID
package main
