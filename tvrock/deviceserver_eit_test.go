package tvrock

import (
	"database/sql"
	"testing"
)

func TestEITBatchContinuesAfterSkippedEvent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateProgramTable(db); err != nil {
		t.Fatal(err)
	}
	oldStore, oldCache := tvprogram, station.Cache
	defer func() { tvprogram = oldStore; station.Cache = oldCache }()
	tvprogram = TvProgramStore{db: &RockDB{Program: []*sql.DB{db}}}
	const sid = uint(0x10065)
	station.Cache = map[uint]*TvStation{sid: {Sid: sid}}
	start := Now().Unix() + 3600
	for _, kind := range []string{"expired", "current", "next"} {
		t.Run(kind, func(t *testing.T) {
			if _, err := db.Exec("DELETE FROM program"); err != nil {
				t.Fatal(err)
			}
			first := TvProgram{Sid: 101, Eid: 1, Start: start, End: start + 1800}
			dev := RockDevice{CurrentService: sid}
			switch kind {
			case "expired":
				first.Start = -1
				first.End = -1
			case "current":
				dev.CurrentProgram = &TvProgram{Sid: sid, Eid: 1}
			case "next":
				dev.NextProgram = &TvProgram{Sid: sid, Eid: 1}
			}
			short := TvProgram{Sid: 101, Eid: 2, Start: start + 1800, End: start + 2100, Title: "five minute program", CnibA: 15, CnibB: 15}
			dev.UpdateTVProgram([]TvProgram{first, short})
			got, err := tvprogram.Get(0, sid, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].End-got[0].Start != 300 {
				t.Fatalf("short program lost after %s: %+v", kind, got)
			}
			short.CnibA, short.CnibB = 1, 2
			dev.UpdateTVProgram([]TvProgram{short})
			got, err = tvprogram.Get(0, sid, 2)
			if err != nil || len(got) != 1 || got[0].CnibA != 1 || got[0].CnibB != 2 {
				t.Fatalf("late genre update lost: %+v, %v", got, err)
			}
		})
	}
}

func TestEITBatchPreservesDetailsWithinRepeatedEvent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateProgramTable(db); err != nil {
		t.Fatal(err)
	}
	previousStore, previousCache := tvprogram, station.Cache
	t.Cleanup(func() { tvprogram, station.Cache = previousStore, previousCache })
	tvprogram = TvProgramStore{db: &RockDB{Program: []*sql.DB{db}}}
	station.Cache = map[uint]*TvStation{0x10065: {Sid: 0x10065}}
	start := Now().Unix() + 3600
	first := TvProgram{Sid: 101, Eid: 1, Start: start, End: start + 1800, Content: "details", CnibA: 1, CnibB: 2}
	second := first
	second.Title = "new title"
	second.Content = ""
	second.CnibA = 15
	second.CnibB = 15
	dev := RockDevice{CurrentService: 0x10065}
	dev.UpdateTVProgram([]TvProgram{first, second})
	got, err := tvprogram.Get(0, 0x10065, 1)
	if err != nil || len(got) != 1 || got[0].Content != "details" || got[0].Title != "new title" || got[0].CnibA != 1 || got[0].CnibB != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	// 一括保存が失敗した場合、削除・途中の書き込みをまとめて戻す。
	if _, err = db.Exec(`CREATE TRIGGER reject_test_program BEFORE INSERT ON program WHEN NEW.title='reject' BEGIN SELECT RAISE(ABORT,'test rejection'); END`); err != nil {
		t.Fatal(err)
	}
	changed := *got[0]
	changed.Title = "must roll back"
	rejected := changed
	rejected.Eid = 2
	rejected.Title = "reject"
	if err := tvprogram.InsertBatch(0, []TvProgram{changed, rejected}); err == nil {
		t.Fatal("batch unexpectedly committed")
	}
	got, err = tvprogram.Get(0, 0x10065, 1)
	if err != nil || len(got) != 1 || got[0].Title != "new title" {
		t.Fatalf("rollback failed: %+v %v", got, err)
	}
}
