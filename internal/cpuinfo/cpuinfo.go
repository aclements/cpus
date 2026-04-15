// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpuinfo

import (
	"cmp"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// Machine represents the entire system topology.
//
// Sockets, Dies, Cores, and Threads exist in a hierarchy.
// NUMA nodes generally correspond to Dies, but this isn't guaranteed.
type Machine struct {
	Threads []*Thread   // All logical CPUs in the system
	Cores   []*Core     // All physical cores
	Dies    []*Die      // All dies
	Sockets []*Socket   // All physical packages
	Nodes   []*NUMANode // All NUMA nodes
}

// Socket corresponds to a physical package (socket).
type Socket struct {
	ID   int // Kernel global physical_package_id
	Dies []*Die
}

// Die corresponds to a silicon die within a package.
type Die struct {
	ID     int // Kernel socket-local die_id
	Socket *Socket
	Cores  []*Core
}

// Core corresponds to a physical hardware core.
type Core struct {
	ID      int // Kernel die-local core_id
	Die     *Die
	Threads []*Thread
}

// Thread corresponds to a logical CPU (hardware thread).
type Thread struct {
	ID        int // Kernel global CPU ID (e.g., N from cpuN)
	Core      *Core
	Node      *NUMANode
	CoreIndex int // Tool-created index of this thread within its Core (0, 1, ...)
}

// NUMANode corresponds to a region of memory and associated CPUs.
type NUMANode struct {
	ID      int // Kernel NUMA node ID
	Threads []*Thread
}

// Load reads the CPU topology from the provided filesystem.
// For production, pass os.DirFS("/").
func Load(sysfs fs.FS) (*Machine, error) {
	m := &Machine{}

	cpuDir := "sys/devices/system/cpu"
	cpuEntries, err := readNumberedEntries(sysfs, cpuDir, "cpu")
	if err != nil {
		return nil, err
	}

	// Maps to track unique entities and build hierarchy
	sockets := make(map[int]*Socket)
	dies := make(map[string]*Die)   // Key: "packageID-dieID"
	cores := make(map[string]*Core) // Key: "packageID-dieID-coreID"

	for _, ce := range cpuEntries {
		cpuID := ce.id
		name := ce.entry.Name()
		topoDir := cpuDir + "/" + name + "/topology"

		packageID, err := readIntFile(sysfs, topoDir+"/physical_package_id")
		if err != nil {
			return nil, err
		}
		dieID, err := readIntFile(sysfs, topoDir+"/die_id")
		if err != nil {
			return nil, err
		}
		coreID, err := readIntFile(sysfs, topoDir+"/core_id")
		if err != nil {
			return nil, err
		}

		// Get or create Socket
		socket, ok := sockets[packageID]
		if !ok {
			socket = &Socket{ID: packageID}
			sockets[packageID] = socket
			m.Sockets = append(m.Sockets, socket)
		}

		// Get or create Die
		dieKey := fmt.Sprintf("%d-%d", packageID, dieID)
		die, ok := dies[dieKey]
		if !ok {
			die = &Die{ID: dieID, Socket: socket}
			dies[dieKey] = die
			socket.Dies = append(socket.Dies, die)
			m.Dies = append(m.Dies, die)
		}

		// Get or create Core
		coreKey := fmt.Sprintf("%d-%d-%d", packageID, dieID, coreID)
		core, ok := cores[coreKey]
		if !ok {
			core = &Core{ID: coreID, Die: die}
			cores[coreKey] = core
			die.Cores = append(die.Cores, core)
			m.Cores = append(m.Cores, core)
		}

		// Create Thread
		thread := &Thread{
			ID:        cpuID,
			Core:      core,
			CoreIndex: len(core.Threads),
		}
		core.Threads = append(core.Threads, thread)
		m.Threads = append(m.Threads, thread)
	}

	// Sort entities to ensure stable order
	slices.SortFunc(m.Sockets, func(a, b *Socket) int {
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortFunc(m.Dies, func(a, b *Die) int {
		if a.Socket.ID != b.Socket.ID {
			return cmp.Compare(a.Socket.ID, b.Socket.ID)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortFunc(m.Cores, func(a, b *Core) int {
		if a.Die.Socket.ID != b.Die.Socket.ID {
			return cmp.Compare(a.Die.Socket.ID, b.Die.Socket.ID)
		}
		if a.Die.ID != b.Die.ID {
			return cmp.Compare(a.Die.ID, b.Die.ID)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortFunc(m.Threads, func(a, b *Thread) int {
		return cmp.Compare(a.ID, b.ID)
	})

	// Read NUMA node information
	nodeDir := "sys/devices/system/node"
	nodeEntries, err := readNumberedEntries(sysfs, nodeDir, "node")
	if err == nil {
		for _, ne := range nodeEntries {
			nodeID := ne.id
			name := ne.entry.Name()

			cpulistStr, err := fs.ReadFile(sysfs, nodeDir+"/"+name+"/cpulist")
			if err != nil {
				continue
			}
			cpus, err := ParseRange(string(cpulistStr))
			if err != nil {
				continue
			}

			node := &NUMANode{ID: nodeID}
			m.Nodes = append(m.Nodes, node)

			// Link threads to node
			cpuSet := make(map[int]bool)
			for _, cpu := range cpus {
				cpuSet[cpu] = true
			}

			for _, thread := range m.Threads {
				if cpuSet[thread.ID] {
					thread.Node = node
					node.Threads = append(node.Threads, thread)
				}
			}
		}
		slices.SortFunc(m.Nodes, func(a, b *NUMANode) int {
			return cmp.Compare(a.ID, b.ID)
		})
	}

	// If no NUMA nodes were discovered, synthesize node 0.
	if len(m.Nodes) == 0 {
		node := &NUMANode{ID: 0}
		m.Nodes = []*NUMANode{node}
		for _, thread := range m.Threads {
			thread.Node = node
			node.Threads = append(node.Threads, thread)
		}
	}

	return m, nil
}

func readIntFile(fsys fs.FS, path string) (int, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	return strconv.Atoi(s)
}

func isDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type numberedEntry struct {
	id    int
	entry fs.DirEntry
}

// readNumberedEntries reads directory entries, filters by prefix followed by a number,
// and returns them sorted by that number.
func readNumberedEntries(sysfs fs.FS, dir string, prefix string) ([]numberedEntry, error) {
	entries, err := fs.ReadDir(sysfs, dir)
	if err != nil {
		return nil, err
	}

	var res []numberedEntry
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) || !isDigit(name[len(prefix):]) {
			continue
		}
		id, err := strconv.Atoi(name[len(prefix):])
		if err != nil {
			continue
		}
		res = append(res, numberedEntry{id, entry})
	}

	slices.SortFunc(res, func(a, b numberedEntry) int {
		return cmp.Compare(a.id, b.id)
	})

	return res, nil
}
