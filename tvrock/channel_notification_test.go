package tvrock

import "testing"

func TestSatelliteChannelNotificationDoesNotRequireFrequencyTable(t *testing.T) {
	previousList, previousCache := configure.TsidList, station.Cache
	t.Cleanup(func() { configure.TsidList, station.Cache = previousList, previousCache })
	configure.TsidList = nil
	station.Cache = map[uint]*TvStation{}
	for _, band := range []TuningSpace{TuningSpaceBS, TuningSpaceCS} {
		for _, tsid := range []uint{0, 0x40f1, 0x4fff, 0x8fff, 0xffff} {
			for _, sid := range []uint{101, 102, 0x10000 | 102} {
				want := uint(band)<<16 | sid&0xffff
				if got := calcLocalSID(uint(band)<<16|tsid, sid); got != want {
					t.Fatalf("band=%d TSID=%d SID=%d: got %d, want %d", band, tsid, sid, got, want)
				}
			}
		}
	}
}

func TestTerrestrialChannelNotificationPreservesServiceBand(t *testing.T) {
	previousList, previousCache := configure.TsidList, station.Cache
	t.Cleanup(func() { configure.TsidList, station.Cache = previousList, previousCache })
	configure.TsidList = nil
	station.Cache = map[uint]*TvStation{
		66560:  {Sid: 66560, Tsp: uint(TuningSpaceTS)},
		263552: {Sid: 263552, Tsp: uint(TuningSpaceOS)},
	}
	for _, sid := range []uint{66560, 263552} {
		if got := calcLocalSID(27, sid&0xffff); got != sid {
			t.Fatalf("terrestrial notification SID=%d: got %d", sid, got)
		}
	}
}

func TestTypedTerrestrialChannelNotification(t *testing.T) {
	previousList, previousCache := configure.TsidList, station.Cache
	t.Cleanup(func() { configure.TsidList, station.Cache = previousList, previousCache })
	// 地上波の下位ワードはTSIDではなくチャンネル値。
	configure.TsidList = []FrequencyTSID{{TSID: 27, TSP: uint(TuningSpaceCS)}}
	const sid = uint(1024)
	for _, tc := range []struct {
		name  string
		bands []TuningSpace
		want  TuningSpace
	}{
		{"fullseg", []TuningSpace{TuningSpaceTS, TuningSpaceBS, TuningSpaceCS}, TuningSpaceTS},
		{"oneseg", []TuningSpace{TuningSpaceOS, TuningSpaceBS, TuningSpaceCS}, TuningSpaceOS},
		{"satellite_only", []TuningSpace{TuningSpaceBS, TuningSpaceCS}, TuningSpaceNone},
		{"unknown", nil, TuningSpaceNone},
		{"ambiguous", []TuningSpace{TuningSpaceTS, TuningSpaceOS}, TuningSpaceNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			station.Cache = map[uint]*TvStation{}
			for _, band := range tc.bands {
				id := uint(band)<<16 | sid
				station.Cache[id] = &TvStation{Sid: id, Tsp: uint(band)}
			}
			want := uint(tc.want)<<16 | sid
			for _, notifiedSID := range []uint{sid, 0x10000 | sid} {
				if got := calcLocalSID(uint(TuningSpaceTS)<<16|27, notifiedSID); got != want {
					t.Fatalf("SID=%d: got %d, want %d", notifiedSID, got, want)
				}
			}
		})
	}
}
