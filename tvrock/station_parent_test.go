package tvrock

import (
	"database/sql"
	"fmt"
	"testing"
)

func TestReadChFilePreservesServiceParents(t *testing.T) {
	const mainSID, childSID = uint(66560), uint(66561)
	local := Station{Cache: map[uint]*TvStation{}}
	line := func(sid uint) string {
		return fmt.Sprintf("テスト局,0,14,0,0,%d,32736,32736,1", sid&0xffff)
	}
	read := func(sid uint) {
		t.Helper()
		if _, err := local.ReadChFile(line(sid), TuningSpaceTS); err != nil {
			t.Fatal(err)
		}
	}
	read(mainSID)
	local.ModifyStation2(&TvStation{Sid: childSID, Tsid: 32736, Tsp: 1, Parent: mainSID, View: 1})
	for range 5 {
		read(mainSID)
		read(childSID)
		if local.Cache[mainSID].Parent != 0 || local.Cache[childSID].Parent != mainSID {
			t.Fatalf("parents changed on reload: main=%d child=%d", local.Cache[mainSID].Parent, local.Cache[childSID].Parent)
		}
	}
	local.Cache[mainSID].Parent = mainSID
	read(mainSID)
	if local.Cache[mainSID].Parent != 0 {
		t.Fatal("self-parent was retained")
	}
}

func TestStationModificationUpdatesTSID(t *testing.T) {
	for _, modify := range []struct {
		name string
		call func(*Station, *TvStation)
	}{{"settings", (*Station).ModifyStation}, {"device", (*Station).ModifyStation2}} {
		t.Run(modify.name, func(t *testing.T) {
			local := Station{Cache: map[uint]*TvStation{66560: {Sid: 66560, Tsp: 1, Tsid: 123}}}
			modify.call(&local, &TvStation{Sid: 66560, Tsp: 1, Tsid: 32736})
			if got := local.Cache[66560]; got.Tsid != 32736 || !got.Update {
				t.Fatalf("TSID not updated: %+v", got)
			}
		})
	}
}

func TestUpdateChannelInfoInitializesParentsWithoutSDT(t *testing.T) {
	setServiceGroups(t, []ServiceGroup{{Tsp: TuningSpaceBS, Parent: 101, Children: []uint{102}}})
	for _, reversed := range []bool{false, true} {
		t.Run(fmt.Sprintf("reversed=%t", reversed), func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if err := CreateTable(db); err != nil {
				t.Fatal(err)
			}
			previousCache, previousDB := station.Cache, station.db
			t.Cleanup(func() { station.Cache, station.db = previousCache, previousDB })
			station.Cache = map[uint]*TvStation{}
			station.db = &RockDB{Data: []*sql.DB{db}}
			line := func(sid, tsid, ch uint) string {
				return fmt.Sprintf("局%d,0,%d,0,0,%d,0,%d,1", sid, ch, sid, tsid)
			}
			pair := func(a, b string) []string {
				if reversed {
					return []string{b, a}
				}
				return []string{a, b}
			}
			lines := []string{";#SPACE(3,UHF)"}
			lines = append(lines, pair(line(1024, 32736, 0), line(1025, 32736, 14))...)
			lines = append(lines, ";#SPACE(4,BS)")
			lines = append(lines, pair(line(101, 16625, 13), line(102, 16625, 13))...)
			lines = append(lines, line(201, 0, 1), line(202, 0, 1))
			lines = append(lines, ";#SPACE(9,Unknown)", line(301, 12345, 3))
			lines = append(lines, ";#SPACE(7,cs110)")
			lines = append(lines, pair(line(101, 16625, 13), line(102, 16625, 13))...)
			device := RockDevice{Com: make(chan RequestMessage, 1)}
			for iteration := range 3 {
				device.UpdateChannelInfo(lines)
				<-device.Com
				loaded, err := station.GetStations(0)
				if err != nil {
					t.Fatal(err)
				}
				if len(loaded) != 8 {
					t.Fatalf("unexpected stations after unknown space: %+v", loaded)
				}
				for sid, parent := range map[uint]uint{
					1<<16 | 1024: 0, 1<<16 | 1025: 1<<16 | 1024,
					2<<16 | 101: 0, 2<<16 | 102: 2<<16 | 101,
					2<<16 | 201: 0, 2<<16 | 202: 0,
					3<<16 | 101: 0, 3<<16 | 102: 0,
				} {
					if got := loaded[sid]; got == nil || got.Parent != parent {
						t.Fatalf("reload %d sid=%d: got %+v, want parent=%d", iteration, sid, got, parent)
					}
				}
				station.Cache = loaded
			}
		})
	}
}

func TestUpdateServiceInfoPreservesParentsAcrossReload(t *testing.T) {
	setServiceGroups(t, []ServiceGroup{{Tsp: TuningSpaceBS, Parent: 101, Children: []uint{102}}, {Tsp: TuningSpaceBS, Parent: 1024, Children: []uint{1025}}})
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateTable(db); err != nil {
		t.Fatal(err)
	}
	previousCache, previousDB := station.Cache, station.db
	t.Cleanup(func() { station.Cache, station.db = previousCache, previousDB })
	station.db = &RockDB{Data: []*sql.DB{db}}
	t.Run("BS parents ignore channel values and previous parents", func(t *testing.T) {
		const mainSID, childSID = uint(2<<16 | 101), uint(2<<16 | 102)
		for _, channels := range [][2]uint{{13, 13}, {0, 13}, {13, 0}, {0, 0}} {
			station.Cache = map[uint]*TvStation{
				mainSID:  {Sid: mainSID, Tsid: 16625, Tsp: 2, Ch: channels[0]},
				childSID: {Sid: childSID, Tsid: 16625, Tsp: 2, Ch: channels[1]},
			}
			device := RockDevice{}
			packets := []PacketSDT{
				{Sid: 102, Tsid: 16625, ServiceType: 1},
				{Sid: 101, Tsid: 16625, ServiceType: 1},
			}
			for range 2 {
				device.UpdateServiceInfo(packets, TuningSpaceBS)
				if station.Cache[mainSID].Parent != 0 || station.Cache[childSID].Parent != mainSID {
					t.Fatalf("Ch=%v: main parent=%d, child parent=%d", channels,
						station.Cache[mainSID].Parent, station.Cache[childSID].Parent)
				}
				if station.Cache[mainSID].Ch != channels[0] || station.Cache[childSID].Ch != channels[1] {
					t.Fatal("channel values changed")
				}
				packets[0], packets[1] = packets[1], packets[0]
			}
		}
	})
	t.Run("repair self and cyclic parents from SDT", func(t *testing.T) {
		for _, brokenParent := range []uint{66560, 66561} {
			station.Cache = map[uint]*TvStation{
				66560: {Sid: 66560, Tsid: 32736, Tsp: 1, Ch: 14, Parent: brokenParent},
				66561: {Sid: 66561, Tsid: 32736, Tsp: 1, Parent: 66560},
			}
			device := RockDevice{}
			device.UpdateServiceInfo([]PacketSDT{
				{Sid: 1025, Tsid: 32736, ServiceType: 1},
				{Sid: 1024, Tsid: 32736, ServiceType: 1},
			}, TuningSpaceTS)
			if station.Cache[66560].Parent != 0 || station.Cache[66561].Parent != 66560 {
				t.Fatalf("broken parent %d not repaired: main=%d child=%d", brokenParent,
					station.Cache[66560].Parent, station.Cache[66561].Parent)
			}
		}
	})
	t.Run("partial SDT keeps the registered parent", func(t *testing.T) {
		for _, tsp := range []TuningSpace{TuningSpaceTS, TuningSpaceBS} {
			mainSID, childSID := uint(tsp)<<16|1024, uint(tsp)<<16|1025
			station.Cache = map[uint]*TvStation{
				mainSID:  {Sid: mainSID, Tsid: 32736, Tsp: uint(tsp)},
				childSID: {Sid: childSID, Tsid: 32736, Tsp: uint(tsp), Parent: mainSID},
			}
			device := RockDevice{}
			device.UpdateServiceInfo([]PacketSDT{{Sid: 1025, Tsid: 32736, ServiceType: 1, Section: 1}}, tsp)
			if station.Cache[mainSID].Parent != 0 || station.Cache[childSID].Parent != mainSID {
				t.Fatalf("partial SDT changed parent: tsp=%d", tsp)
			}
		}
	})
	t.Run("parent arriving later repairs an orphan and startup applies removed settings", func(t *testing.T) {
		const mainSID, childSID = uint(2<<16 | 101), uint(2<<16 | 102)
		station.Cache = map[uint]*TvStation{}
		device := RockDevice{}
		device.UpdateServiceInfo([]PacketSDT{{Sid: 102, Tsid: 16625, ServiceType: 1}}, TuningSpaceBS)
		if station.Cache[childSID].Parent != 0 {
			t.Fatal("missing parent was assigned")
		}
		device.UpdateServiceInfo([]PacketSDT{{Sid: 101, Tsid: 16625, ServiceType: 1}}, TuningSpaceBS)
		if station.Cache[childSID].Parent != mainSID {
			t.Fatal("late parent was not assigned")
		}
		groups := configure.ServiceGroups
		configure.ServiceGroups = nil
		defer func() { configure.ServiceGroups = groups }()
		local := Station{}
		if err := local.Initialize(station.db); err != nil {
			t.Fatal(err)
		}
		loaded, err := local.GetStations(0)
		if err != nil {
			t.Fatal(err)
		}
		if loaded[childSID].Parent != 0 {
			t.Fatal("startup did not persist removal of a BS definition")
		}
	})
	for _, tsp := range []TuningSpace{TuningSpaceTS, TuningSpaceBS} {
		t.Run(fmt.Sprint(tsp), func(t *testing.T) {
			mainSID := uint(tsp)<<16 | 1024
			childSID := uint(tsp)<<16 | 1025
			otherSID := uint(tsp)<<16 | 1032
			station.Cache = map[uint]*TvStation{
				mainSID:  {Sid: mainSID, Tsid: 32736, Tsp: uint(tsp), Ch: 14, View: 0, No: 1, FetchProgram: true, HashTag: "keep"},
				otherSID: {Sid: otherSID, Tsid: 32736, Tsp: uint(tsp), Ch: 13, View: 1, No: 2},
			}
			packets := []PacketSDT{
				{Sid: 999, Tsid: 32736, ServiceType: 0xc0},
				{Sid: 1000, Tsid: 32736, ServiceType: 1, OneSeg: -1},
				{Sid: 1026, Tsid: 32736, ServiceType: 0xc0, OneSeg: 1, ServiceName: "ワンセグ"},
				{Sid: 1025, Tsid: 32736, ServiceType: 1, ServiceName: "子局"},
				{Sid: 1024, Tsid: 32736, ServiceType: 1, ServiceName: "親局"},
				{Sid: 1032, Tsid: 32736, ServiceType: 1, ServiceName: "登録済み子局"},
				{Sid: 1040, Tsid: 32738, ServiceType: 1, ServiceName: "別TS"},
			}
			device := RockDevice{}
			for iteration := range 3 {
				if iteration > 0 {
					loaded, err := station.GetStations(0)
					if err != nil {
						t.Fatal(err)
					}
					station.Cache = loaded
				}
				if _, err := station.ReadChFile("親局,0,14,0,0,1024,32736,32736,1", tsp); err != nil {
					t.Fatal(err)
				}
				device.UpdateServiceInfo(packets, tsp)
				parents := map[uint]uint{mainSID: 0, childSID: mainSID, otherSID: 0,
					uint(TuningSpaceOS)<<16 | 1026: mainSID, uint(tsp)<<16 | 1040: 0}
				if tsp == TuningSpaceTS {
					parents[otherSID] = mainSID
				}
				for sid, parent := range parents {
					if got := station.Cache[sid]; got == nil || got.Parent != parent {
						t.Fatalf("reload %d sid=%d: got %+v, want parent=%d", iteration, sid, got, parent)
					}
				}
				if got := station.Cache[mainSID]; got.View != 0 || got.No != 1 || !got.FetchProgram || got.HashTag != "keep" {
					t.Fatalf("user settings changed: %+v", got)
				}
			}
		})
	}
}

func TestOneSegDataServiceIsSavedAfterClassification(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateTable(db); err != nil {
		t.Fatal(err)
	}
	previousCache, previousDB := station.Cache, station.db
	t.Cleanup(func() { station.Cache, station.db = previousCache, previousDB })
	station.Cache = map[uint]*TvStation{}
	station.db = &RockDB{Data: []*sql.DB{db}}
	const parentSID, oneSegSID = uint(1<<16 | 1024), uint(4<<16 | 1408)
	packets := []PacketSDT{
		{Sid: 1408, Tsid: 32736, ServiceType: 0xc0, OneSeg: -1, ServiceName: "ワンセグ"},
		{Sid: 1024, Tsid: 32736, ServiceType: 1, ServiceName: "親局"},
		{Sid: 900, Tsid: 32736, ServiceType: 0xc0},
		{Sid: 901, Tsid: 32736, ServiceType: 2},
	}
	device := RockDevice{}
	device.UpdateServiceInfo(packets, TuningSpaceTS)
	if station.Cache[oneSegSID] != nil {
		t.Fatal("unclassified service was registered as one-seg")
	}
	packets[0].OneSeg = 1
	for range 2 {
		device.UpdateServiceInfo(packets, TuningSpaceTS)
		loaded, err := station.GetStations(0)
		if err != nil {
			t.Fatal(err)
		}
		got := loaded[oneSegSID]
		if got == nil || got.Tsp != uint(TuningSpaceOS) || got.Parent != parentSID || got.View != 1 || got.Tsid != 32736 || got.Name != "ワンセグ" {
			t.Fatalf("one-seg service was not saved correctly: %+v", got)
		}
		if len(loaded) != 2 || loaded[parentSID] == nil || loaded[parentSID].Parent != 0 {
			t.Fatalf("unexpected parent or non-TV services: %+v", loaded)
		}
		station.Cache = loaded
	}
}
