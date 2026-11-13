package tvrock

import "testing"

func TestChannelCodeSelectsParentDeterministically(t *testing.T) {
	for _, tt := range []struct {
		name     string
		stations []*TvStation
		want     uint
	}{
		{
			name: "parent over child with smaller SID",
			stations: []*TvStation{
				{Sid: 66560, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: 66561},
				{Sid: 66561, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1},
			},
			want: 66561,
		},
		{
			name: "main and subchannel",
			stations: []*TvStation{
				{Sid: 66560, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1},
				{Sid: 66561, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: 66560},
			},
			want: 66560,
		},
		{
			name: "stable fallback without parent",
			stations: []*TvStation{
				{Sid: 66562, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: 66560},
				{Sid: 66561, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: 66560},
			},
			want: 66561,
		},
		{
			name: "ignore disabled or unrelated stations",
			stations: []*TvStation{
				{Sid: 66560, Tsp: uint(TuningSpaceTS), Ch: 14, View: 0},
				{Sid: 66561, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: 66560},
				{Sid: 66559, Tsp: uint(TuningSpaceTS), Ch: 15, View: 1},
				{Sid: 132096, Tsp: uint(TuningSpaceBS), Ch: 14, View: 1},
			},
			want: 66561,
		},
		{name: "missing channel"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			local := &Station{Cache: make(map[uint]*TvStation)}
			for _, sta := range tt.stations {
				local.Cache[sta.Sid] = sta
			}
			for range 100 {
				if got := local.GetServiceIDByChannelCode(14); got != tt.want {
					t.Fatalf("got %d, want %d", got, tt.want)
				}
			}
		})
	}
}
