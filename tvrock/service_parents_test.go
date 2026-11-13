package tvrock

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func setServiceGroups(t *testing.T, groups []ServiceGroup) {
	t.Helper()
	previous := configure.ServiceGroups
	configure.ServiceGroups = groups
	t.Cleanup(func() { configure.ServiceGroups = previous })
}

func TestServiceParentsUseMinimumSIDAndConfiguredGroups(t *testing.T) {
	setServiceGroups(t, []ServiceGroup{{Tsp: TuningSpaceBS, Parent: 101, Children: []uint{102, 103}}, {Tsp: TuningSpaceBS, Parent: 141, Children: []uint{142}}})
	local := Station{Cache: map[uint]*TvStation{}}
	add := func(tsp TuningSpace, sid, tsid, oldParent uint) uint {
		id := uint(tsp)<<16 | sid
		local.Cache[id] = &TvStation{Sid: id, Tsp: uint(tsp), Tsid: tsid, Parent: oldParent}
		return id
	}
	main := add(TuningSpaceTS, 1024, 32736, 0)
	child := add(TuningSpaceTS, 1025, 32736, main)
	other := add(TuningSpaceTS, 1032, 32736, main)
	orphan := add(TuningSpaceTS, 1041, 32736, main)
	wrongTS := add(TuningSpaceTS, 1026, 32737, main)
	bsMain := add(TuningSpaceBS, 101, 16625, 131174)
	bsChild := add(TuningSpaceBS, 102, 16625, 0)
	bsWrongTS := add(TuningSpaceBS, 103, 16626, bsMain)
	bsOther := add(TuningSpaceBS, 141, 16625, bsMain)
	bsOtherChild := add(TuningSpaceBS, 142, 16625, 0)
	bsUnknown := add(TuningSpaceBS, 191, 16625, bsMain)
	cs := add(TuningSpaceCS, 102, 16625, 123)
	for iteration := range 2 {
		local.updateServiceParents()
		for sid, want := range map[uint]uint{
			main: 0, child: main, other: main, orphan: main, wrongTS: 0,
			bsMain: 0, bsChild: bsMain, bsWrongTS: 0, bsOther: 0,
			bsOtherChild: bsOther, bsUnknown: 0, cs: 0,
		} {
			if got := local.Cache[sid].Parent; got != want {
				t.Fatalf("sid=%d: parent=%d, want %d", sid, got, want)
			}
			if iteration > 0 && local.Cache[sid].Update {
				t.Fatal("unchanged relationships were marked for update")
			}
			local.Cache[sid].Update = false
		}
	}
	configure.ServiceGroups = nil
	local.updateServiceParents()
	if local.Cache[bsChild].Parent != 0 || local.Cache[bsOtherChild].Parent != 0 {
		t.Fatal("removed BS definitions were retained")
	}
}

func TestServiceGroupConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		invalid       bool
	}{
		{"missing", "host: localhost:80", false},
		{"empty", "servicegroups: []", false},
		{"valid", "servicegroups: [{tsp: 2, parent: 101, children: [102, 103]}]", false},
		{"self", "servicegroups: [{tsp: 2, parent: 101, children: [101]}]", true},
		{"cycle", "servicegroups: [{tsp: 2, parent: 101, children: [102]}, {tsp: 2, parent: 102, children: [101]}]", true},
		{"duplicate child", "servicegroups: [{tsp: 2, parent: 101, children: [102]}, {tsp: 2, parent: 141, children: [102]}]", true},
		{"duplicate parent", "servicegroups: [{tsp: 2, parent: 101, children: [102]}, {tsp: 2, parent: 101, children: [103]}]", true},
		{"out of range", "servicegroups: [{tsp: 2, parent: 131173, children: [102]}]", true},
		{"empty children", "servicegroups: [{tsp: 2, parent: 101, children: []}]", true},
		{"zero child", "servicegroups: [{tsp: 2, parent: 101, children: [0]}]", true},
		{"common BS", "servicegroups: [{tsp: 2, parent: 101, children: [102]}]", false},
		{"common CS", "servicegroups: [{tsp: 3, parent: 101, children: [102]}]", false},
		{"common terrestrial", "servicegroups: [{tsp: 1, parent: 1025, children: [1026]}]", false},
		{"common one-seg", "servicegroups: [{tsp: 4, parent: 1408, children: [1409]}]", false},
		{"common 4K", "servicegroups: [{tsp: 16, parent: 101, children: [102]}]", false},
		{"same SID across types", "servicegroups: [{tsp: 2, parent: 101, children: [102]}, {tsp: 3, parent: 101, children: [102]}]", false},
		{"missing type", "servicegroups: [{parent: 101, children: [102]}]", true},
		{"reserved type", "servicegroups: [{tsp: 8, parent: 101, children: [102]}]", true},
		{"common cycle", "servicegroups: [{tsp: 3, parent: 101, children: [102]}, {tsp: 3, parent: 102, children: [101]}]", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.yml")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			conf := &TvRockConf{}
			if err := conf.Load(path); (err != nil) != tc.invalid {
				t.Fatalf("Load error=%v, want invalid=%t", err, tc.invalid)
			}
			if !tc.invalid {
				if err := conf.Save(path); err != nil {
					t.Fatal(err)
				}
				loaded := &TvRockConf{}
				if err := loaded.Load(path); err != nil {
					t.Fatal(err)
				}
				if got, want := loaded.serviceParents()[2<<16|102], conf.serviceParents()[2<<16|102]; got != want {
					t.Fatalf("saved mapping=%d, want %d", got, want)
				}
				if !reflect.DeepEqual(loaded.serviceParents(), conf.serviceParents()) {
					t.Fatal("common groups changed on save/reload")
				}
			}
		})
	}
}

func TestCommonServiceGroupsOverrideAutomaticParentsAcrossTypes(t *testing.T) {
	setServiceGroups(t, nil)
	configure.ServiceGroups = []ServiceGroup{
		{Tsp: TuningSpaceTS, Parent: 1025, Children: []uint{1026}},
		{Tsp: TuningSpaceBS, Parent: 101, Children: []uint{102}},
		{Tsp: TuningSpaceCS, Parent: 101, Children: []uint{102}},
		{Tsp: TuningSpaceOS, Parent: 1409, Children: []uint{1408}},
		{Tsp: TuningSpace4K, Parent: 101, Children: []uint{102}},
	}
	local := Station{Cache: map[uint]*TvStation{}}
	for _, group := range configure.ServiceGroups {
		root := uint(group.Tsp)<<16 | group.Parent
		local.Cache[root] = &TvStation{Sid: root, Tsp: uint(group.Tsp), Tsid: 123}
		for _, sid := range group.Children {
			id := uint(group.Tsp)<<16 | sid
			local.Cache[id] = &TvStation{Sid: id, Tsp: uint(group.Tsp), Tsid: 123}
		}
	}
	const automaticRoot = uint(1<<16 | 1024)
	local.Cache[automaticRoot] = &TvStation{Sid: automaticRoot, Tsp: 1, Tsid: 123}
	local.updateServiceParents()
	for _, group := range configure.ServiceGroups {
		root := uint(group.Tsp)<<16 | group.Parent
		if local.Cache[root].Parent != 0 {
			t.Fatalf("explicit root became a child: %+v", local.Cache[root])
		}
		for _, sid := range group.Children {
			id := uint(group.Tsp)<<16 | sid
			if local.Cache[id].Parent != root {
				t.Fatalf("tsp=%d: %+v", group.Tsp, local.Cache[id])
			}
		}
	}
	configure.ServiceGroups = nil
	local.updateServiceParents()
	if local.Cache[1<<16|1025].Parent != automaticRoot || local.Cache[1<<16|1026].Parent != automaticRoot {
		t.Fatal("terrestrial automatic grouping was not restored")
	}
	for _, tsp := range []TuningSpace{TuningSpaceBS, TuningSpaceCS, TuningSpaceOS, TuningSpace4K} {
		for _, sta := range local.Cache {
			if sta.Tsp == uint(tsp) && sta.Parent != 0 {
				t.Fatalf("removed definition retained: %+v", sta)
			}
		}
	}
}

func TestConfiguredServiceStreamChangeAcrossTypes(t *testing.T) {
	previousDevices, previousStations, previousRecording := slot.Device, station.Cache, scheduler.CurrentRecording
	t.Cleanup(func() {
		slot.Device, station.Cache, scheduler.CurrentRecording = previousDevices, previousStations, previousRecording
	})
	for _, tsp := range []TuningSpace{TuningSpaceTS, TuningSpaceBS, TuningSpaceCS, TuningSpaceOS, TuningSpace4K} {
		parent, child := uint(tsp)<<16|101, uint(tsp)<<16|102
		station.Cache = map[uint]*TvStation{child: {Sid: child, Tsp: uint(tsp), Parent: parent}}
		queue := make(chan RequestMessage, 2)
		slot.Device = map[string]*RockDevice{"test-device": {ID: "test-device", State: StateReady, Com: queue}}
		scheduler.CurrentRecording = map[string]*RecordingSchedule{"test-device": {ID: 77}}
		job := &RecordingJob{ID: 77, DeviceID: "test-device", Sid: child}
		if err := job.StreamChange(); err != nil {
			t.Fatal(err)
		}
		if len(queue) != 2 {
			t.Fatalf("tsp=%d: got %d commands", tsp, len(queue))
		}
		for _, want := range []uint{SubChannnelCode<<16 | child&0xffff, parent} {
			got := <-queue
			if got.Command != "StreamChange" || got.Message != strconv.FormatUint(uint64(want), 10) {
				t.Fatalf("tsp=%d: got %+v, want sid=%d", tsp, got, want)
			}
		}
	}
}

func TestTerrestrialParentsUseMinimumSIDWithoutZeroServiceNumber(t *testing.T) {
	setServiceGroups(t, nil)
	local := Station{Cache: map[uint]*TvStation{}}
	check := func(root uint) {
		t.Helper()
		for sid, sta := range local.Cache {
			want := uint(TuningSpaceTS)<<16 | root
			if sid == want {
				want = 0
			}
			if sta.Parent != want {
				t.Fatalf("sid=%d: got parent=%d, want %d", sid, sta.Parent, want)
			}
		}
	}
	// 下位3bitが0のサービスがなく、登録順も逆でも最小SIDを選ぶ。
	for _, sid := range []uint{1041, 1033, 1025} {
		id := uint(TuningSpaceTS)<<16 | sid
		local.Cache[id] = &TvStation{Sid: id, Tsp: 1, Tsid: 32736}
		local.updateServiceParents()
		check(sid)
	}
	for range 5 {
		local.updateServiceParents()
		check(1025)
	}
	configure.ServiceGroups = []ServiceGroup{{Tsp: TuningSpaceTS, Parent: 1041, Children: []uint{1025, 1033}}}
	local.updateServiceParents()
	check(1041)
	configure.ServiceGroups = nil
	local.updateServiceParents()
	check(1025)
}
