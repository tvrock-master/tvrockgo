package tvrock

import (
	"testing"
	"time"
)

func TestGetDuringWithStationEmptyStations(t *testing.T) {
	// With no stations selected, return an empty result without accessing a DB.
	store := &TvProgramStore{}
	for _, stations := range [][]uint{nil, {}} {
		programs, err := store.GetDuringWithStation(0, stations, time.Unix(0, 0), time.Unix(3600, 0), 0)
		if err != nil {
			t.Fatalf("GetDuringWithStation(%v): %v", stations, err)
		}
		if programs == nil || len(programs) != 0 {
			t.Fatalf("GetDuringWithStation(%v) = %#v, want an empty map", stations, programs)
		}
	}
}
