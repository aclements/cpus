// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/fs"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/aclements/cpus/internal/cpuinfo"
)

func TestFilterAndSort(t *testing.T) {
	// Create a fake sysfs
	sysfs := fstest.MapFS{
		"sys/devices/system/cpu/present":  &fstest.MapFile{Data: []byte("0-3\n")},
		"sys/devices/system/cpu/possible": &fstest.MapFile{Data: []byte("0-3\n")},
		"sys/devices/system/cpu/online":   &fstest.MapFile{Data: []byte("0-1\n")},
		"sys/devices/system/cpu/offline":  &fstest.MapFile{Data: []byte("2-3\n")},
	}

	// Construct a synthetic Machine
	s0 := &cpuinfo.Socket{ID: 0}
	d0 := &cpuinfo.Die{ID: 0, Socket: s0}
	s0.Dies = []*cpuinfo.Die{d0}

	c0 := &cpuinfo.Core{ID: 0, Die: d0}
	c1 := &cpuinfo.Core{ID: 1, Die: d0}
	d0.Cores = []*cpuinfo.Core{c0, c1}

	n0 := &cpuinfo.NUMANode{ID: 0}

	t0 := &cpuinfo.Thread{ID: 0, Core: c0, Node: n0, CoreIndex: 0}
	t1 := &cpuinfo.Thread{ID: 1, Core: c0, Node: n0, CoreIndex: 1}
	t2 := &cpuinfo.Thread{ID: 2, Core: c1, Node: n0, CoreIndex: 0}
	t3 := &cpuinfo.Thread{ID: 3, Core: c1, Node: n0, CoreIndex: 1}

	c0.Threads = []*cpuinfo.Thread{t0, t1}
	c1.Threads = []*cpuinfo.Thread{t2, t3}

	n0.Threads = []*cpuinfo.Thread{t0, t1, t2, t3}

	m := &cpuinfo.Machine{
		Threads: []*cpuinfo.Thread{t0, t1, t2, t3},
		Cores:   []*cpuinfo.Core{c0, c1},
		Dies:    []*cpuinfo.Die{d0},
		Sockets: []*cpuinfo.Socket{s0},
		Nodes:   []*cpuinfo.NUMANode{n0},
	}

	tests := []struct {
		name    string
		args    []string
		wantIDs []int
		wantErr bool
	}{
		{
			name:    "default sort",
			args:    []string{},
			wantIDs: []int{0, 1, 2, 3}, // Default sort is socket, die, core, thread
		},
		{
			name:    "filter range",
			args:    []string{"0-1"},
			wantIDs: []int{0, 1},
		},
		{
			name:    "filter online",
			args:    []string{"online"},
			wantIDs: []int{0, 1},
		},
		{
			name:    "filter field",
			args:    []string{"core==1"},
			wantIDs: []int{2, 3},
		},
		{
			name:    "filter node",
			args:    []string{"node==0"},
			wantIDs: []int{0, 1, 2, 3},
		},
		{
			name:    "sort rr",
			args:    []string{"rr"},
			wantIDs: []int{0, 2, 1, 3}, // thread socket die core. threads: 0, 2 are thread 0. 1, 3 are thread 1.
		},
		{
			name:    "filter and sort",
			args:    []string{"online", "processor!=0"},
			wantIDs: []int{1},
		},
		{
			name:    "invalid filter",
			args:    []string{"unknown==0"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterAndSort(m, sysfs, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("processThreads() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			var gotIDs []int
			for _, th := range got {
				gotIDs = append(gotIDs, th.ID)
			}
			if !reflect.DeepEqual(gotIDs, tt.wantIDs) {
				t.Errorf("processThreads() = %v, want %v", gotIDs, tt.wantIDs)
			}
		})
	}
}

type mockWriteFS struct {
	fstest.MapFS
	writes map[string][]byte
}

func (m *mockWriteFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if m.writes == nil {
		m.writes = make(map[string][]byte)
	}
	m.writes[name] = data
	return nil
}

func TestDoHotplug(t *testing.T) {
	sysfs := &mockWriteFS{
		MapFS: fstest.MapFS{
			"sys/devices/system/cpu/present":  &fstest.MapFile{Data: []byte("0-3\n")},
			"sys/devices/system/cpu/possible": &fstest.MapFile{Data: []byte("0-3\n")},
			"sys/devices/system/cpu/online":   &fstest.MapFile{Data: []byte("0-1\n")},
		},
	}

	err := doHotplug(sysfs, []int{0, 2})
	if err != nil {
		t.Fatalf("doHotplug failed: %v", err)
	}

	wantWrites := map[string][]byte{
		"sys/devices/system/cpu/cpu2/online": []byte("1\n"),
		"sys/devices/system/cpu/cpu1/online": []byte("0\n"),
	}

	if !reflect.DeepEqual(sysfs.writes, wantWrites) {
		t.Errorf("writes = %v, want %v", sysfs.writes, wantWrites)
	}

	t.Run("not present", func(t *testing.T) {
		err := doHotplug(sysfs, []int{4})
		if err == nil {
			t.Errorf("expected error for non-present CPU")
		}
	})
}

func TestDoList(t *testing.T) {
	n0 := &cpuinfo.NUMANode{ID: 0}
	s0 := &cpuinfo.Socket{ID: 0}
	d0 := &cpuinfo.Die{ID: 0, Socket: s0}
	d1 := &cpuinfo.Die{ID: 1, Socket: s0}
	s0.Dies = []*cpuinfo.Die{d0, d1}
	c0 := &cpuinfo.Core{ID: 0, Die: d0}
	d0.Cores = []*cpuinfo.Core{c0}
	t0 := &cpuinfo.Thread{ID: 0, Core: c0, CoreIndex: 0, Node: n0}
	t1 := &cpuinfo.Thread{ID: 1, Core: c0, CoreIndex: 1, Node: n0}
	t2 := &cpuinfo.Thread{ID: 2, Core: c0, CoreIndex: 2, Node: n0}
	c0.Threads = []*cpuinfo.Thread{t0, t1, t2}

	m := &cpuinfo.Machine{
		Threads: []*cpuinfo.Thread{t0, t1, t2},
		Cores:   []*cpuinfo.Core{c0},
		Dies:    []*cpuinfo.Die{d0, d1},
		Sockets: []*cpuinfo.Socket{s0},
		Nodes:   []*cpuinfo.NUMANode{n0},
	}

	tests := []struct {
		name    string
		format  string
		want    string
		wantErr bool
	}{
		{
			name:   "compact",
			format: "compact",
			want:   "0-2\n",
		},
		{
			name:   "comma",
			format: "comma",
			want:   "0,1,2\n",
		},
		{
			name:   "space",
			format: "space",
			want:   "0 1 2\n",
		},
		{
			name:   "table",
			format: "table",
			want: `node socket die core thread processor
0    0      0   0    0      0
0    0      0   0    1      1
0    0      0   0    2      2
`,
		},
		{
			name:    "invalid",
			format:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := doList(&buf, m, m.Threads, tt.format)
			if (err != nil) != tt.wantErr {
				t.Fatalf("doList() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if buf.String() != tt.want {
				t.Errorf("doList() = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func BenchmarkSilly(b *testing.B) {
	var x atomic.Int32
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			x.Add(1)
		}
	})
}
