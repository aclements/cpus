// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpuinfo

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		input   string
		want    []int
		wantErr bool
	}{
		{"0-3", []int{0, 1, 2, 3}, false},
		{"0-3,8-11", []int{0, 1, 2, 3, 8, 9, 10, 11}, false},
		{"0", []int{0}, false},
		{"", []int{}, false},
		{"  0-3  ", []int{0, 1, 2, 3}, false},
		{"invalid", nil, true},
		{"0-", nil, true},
		{"-3", nil, true},
	}

	for _, tt := range tests {
		got, err := ParseRange(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseRange(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseRange(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStrRange(t *testing.T) {
	tests := []struct {
		input []int
		want  string
	}{
		{[]int{0, 1, 2, 3}, "0-3"},
		{[]int{0, 1, 2, 3, 8, 9, 10, 11}, "0-3,8-11"},
		{[]int{0}, "0"},
		{[]int{}, ""},
		{[]int{0, 1}, "0,1"},
		{[]int{0, 2}, "0,2"},
		{[]int{0, 1, 3, 4}, "0,1,3,4"},
	}

	for _, tt := range tests {
		got := StrRange(tt.input)
		if got != tt.want {
			t.Errorf("StrRange(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"sys/devices/system/cpu/cpu0/topology/physical_package_id": &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu0/topology/die_id":              &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu0/topology/core_id":             &fstest.MapFile{Data: []byte("0\n")},

		"sys/devices/system/cpu/cpu1/topology/physical_package_id": &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu1/topology/die_id":              &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu1/topology/core_id":             &fstest.MapFile{Data: []byte("1\n")},

		"sys/devices/system/cpu/cpu2/topology/physical_package_id": &fstest.MapFile{Data: []byte("1\n")},
		"sys/devices/system/cpu/cpu2/topology/die_id":              &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu2/topology/core_id":             &fstest.MapFile{Data: []byte("0\n")},

		"sys/devices/system/node/node0/cpulist": &fstest.MapFile{Data: []byte("0-1\n")},
		"sys/devices/system/node/node1/cpulist": &fstest.MapFile{Data: []byte("2\n")},
	}

	m, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	// Verify topology
	if len(m.Sockets) != 2 {
		t.Errorf("len(Sockets) = %d, want 2", len(m.Sockets))
	}
	if len(m.Threads) != 3 {
		t.Errorf("len(Threads) = %d, want 3", len(m.Threads))
	}
	if len(m.Nodes) != 2 {
		t.Errorf("len(Nodes) = %d, want 2", len(m.Nodes))
	}

	// Check specific thread
	t0 := m.Threads[0]
	if t0.ID != 0 || t0.Core.ID != 0 || t0.Core.Die.ID != 0 || t0.Core.Die.Socket.ID != 0 || t0.Node.ID != 0 {
		t.Errorf("Thread 0 unexpected topology: %+v", t0)
	}
}

func TestLoadNoNUMA(t *testing.T) {
	fsys := fstest.MapFS{
		"sys/devices/system/cpu/cpu0/topology/physical_package_id": &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu0/topology/die_id":              &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu0/topology/core_id":             &fstest.MapFile{Data: []byte("0\n")},

		"sys/devices/system/cpu/cpu1/topology/physical_package_id": &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu1/topology/die_id":              &fstest.MapFile{Data: []byte("0\n")},
		"sys/devices/system/cpu/cpu1/topology/core_id":             &fstest.MapFile{Data: []byte("1\n")},
	}

	m, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(m.Nodes))
	}
	if m.Nodes[0].ID != 0 {
		t.Errorf("m.Nodes[0].ID = %d, want 0", m.Nodes[0].ID)
	}

	for _, thread := range m.Threads {
		if thread.Node == nil {
			t.Errorf("Thread %d has nil Node", thread.ID)
		} else if thread.Node.ID != 0 {
			t.Errorf("Thread %d has Node ID %d, want 0", thread.ID, thread.Node.ID)
		}
	}
}
