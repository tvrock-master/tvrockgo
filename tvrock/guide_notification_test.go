package tvrock

import (
	"database/sql"
	"testing"
)

func TestEITNotifiesGuideOnlyWhenStoredProgramsChange(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateProgramTable(db); err != nil {
		t.Fatal(err)
	}
	previousStore, previousCache, previousHandlers := tvprogram, station.Cache, deviceController.Handler
	t.Cleanup(func() {
		tvprogram, station.Cache, deviceController.Handler = previousStore, previousCache, previousHandlers
	})
	tvprogram = TvProgramStore{db: &RockDB{Program: []*sql.DB{db}}}
	station.Cache = map[uint]*TvStation{0x10065: {Sid: 0x10065}}
	queue := &DevConQueue{Com: make(chan string, 8), Done: make(chan struct{})}
	deviceController.Handler = map[string]*DevConQueue{"test": queue}
	start := Now().Unix() + 3600
	programs := []TvProgram{{Sid: 101, Eid: 1, Start: start, End: start + 1800, Title: "test"},
		{Sid: 101, Eid: 2, Start: start + 1800, End: start + 3600, Title: "next"}}
	dev := RockDevice{CurrentService: 0x10065}
	dev.UpdateTVProgram(programs)
	if got := <-queue.Com; got != ControllerUpdateTimeTable {
		t.Fatal(got)
	}
	dev.UpdateTVProgram(programs)
	select {
	case got := <-queue.Com:
		t.Fatalf("unchanged EPG notified again: %s", got)
	default:
	}
	programs[0].Content = "new details"
	dev.UpdateTVProgram(programs)
	if got := <-queue.Com; got != ControllerUpdateTimeTable {
		t.Fatal(got)
	}
}
