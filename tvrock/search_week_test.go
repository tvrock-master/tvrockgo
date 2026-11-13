package tvrock

import (
	"database/sql"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

func TestMatchProgramWeek(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	t.Cleanup(func() { time.Local = previous })
	sunday := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	for day := range 7 {
		for _, hour := range []int{0, 23} {
			start := sunday.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour)
			prg := &TvProgram{Start: start.Unix(), End: start.Add(2 * time.Hour).Unix()}
			for mask := range 128 {
				want := mask == 0 || mask&(1<<day) != 0
				if got := matchProgramWeek(uint8(mask), prg); got != want {
					t.Fatalf("day=%d hour=%d mask=%d: got %v, want %v", day, hour, mask, got, want)
				}
			}
		}
	}
}

func TestSearchWeekBackends(t *testing.T) {
	previousDB, previousPrograms := rockDB, tvprogram
	previousIndex, previousEnabled, previousLimit := bleveProgram.index, configure.Bleve, configure.SearchLimit
	t.Cleanup(func() {
		rockDB, tvprogram = previousDB, previousPrograms
		bleveProgram.index, configure.Bleve, configure.SearchLimit = previousIndex, previousEnabled, previousLimit
	})
	rockDB = RockDB{Program: []*sql.DB{newSearchTestDB(t, false), newSearchTestDB(t, false)},
		Caption: []*sql.DB{newSearchTestDB(t, true), newSearchTestDB(t, true)}}
	start := time.Date(2026, 11, 1, 23, 30, 0, 0, time.Local).Unix()
	end := start + 7*86400
	tvprogram = TvProgramStore{db: &rockDB, FirstStartTime: []int64{start, start}, LastEndTime: []int64{end, end}}
	mapping, err := newTvProgramBleveMapping()
	if err != nil {
		t.Fatal(err)
	}
	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	bleveProgram.index = index
	store := BleveProgramStore{index: index}
	for day := range 7 {
		prg := &TvProgram{Sid: 101, Eid: uint(day + 1), Start: start + int64(day)*86400,
			End: start + int64(day)*86400 + 3600, Title: "ニュース"}
		for no := range 2 {
			if _, err := rockDB.Program[no].Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (?,?,?,?,?,'','',0,0)`,
				prg.Sid, prg.Eid, prg.Start, prg.End, prg.Title); err != nil {
				t.Fatal(err)
			}
			if err := rockDB.InsertCaption(no, prg.Sid, prg.Eid, prg.Start, &PacketCaption{Text: "字幕"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.BatchIndexProgram(newBleveProgramDocument(prg, "字幕")); err != nil {
			t.Fatal(err)
		}
	}
	for _, backend := range []string{"current", "archive-scan", "archive-bleve"} {
		for _, caption := range []bool{false, true} {
			for _, mask := range []uint8{0, 1, 2, 64, 65, 127} {
				configure.Bleve, configure.SearchLimit = backend == "archive-bleve", 1
				opt := NewSearch()
				opt.StartTime, opt.EndTime, opt.Week = start, end-start, mask
				opt.ArchiveSearch, opt.CaptionSearch, opt.FuzzySearch = backend != "current", caption, false
				opt.Title = "ニュース"
				if caption {
					opt.Title = "字幕"
				}
				task := SearchTask{ID: ^uint64(0), option: opt, title: regexp.MustCompile(opt.Title), current: start,
					leng: 86400, sid: []uint{101}, com: make(chan SearchJobCom)}
				task.Exec()
				want := uint(1)
				for mask != 0 && mask&(1<<(want-1)) == 0 {
					want++
				}
				if task.state != SearchJobFinish || len(task.result) != 1 || task.result[0].Eid != want {
					t.Fatalf("backend=%s caption=%v week=%d: state=%v results=%+v, want eid=%d", backend, caption, mask, task.state, task.result, want)
				}
			}
		}
	}
}

func TestKeywordReserveWeekStorageAndArchive(t *testing.T) {
	previousDB, previousThreshold := rockDB, configure.CurrentKeywordReserveThreshold
	t.Cleanup(func() { rockDB, configure.CurrentKeywordReserveThreshold = previousDB, previousThreshold })
	rockDB = RockDB{Data: []*sql.DB{newSearchTestDB(t, false), newSearchTestDB(t, false)}}
	for _, db := range rockDB.Data {
		if err := CreateTable(db); err != nil {
			t.Fatal(err)
		}
	}
	kr := NewKeywordReserve()
	kr.Title, kr.Week, kr.Sid = "ニュース", 65, []uint{101}
	if err := rockDB.InsertKeywordReserve(0, kr); err != nil {
		t.Fatal(err)
	}
	check := func(no int, want uint8) {
		t.Helper()
		got, err := rockDB.GetKeywordReserve(no, kr.ID)
		if err != nil || got.Week != want || got.Title != kr.Title || !reflect.DeepEqual(got.Sid, kr.Sid) {
			t.Fatalf("slot=%d: got=%+v err=%v", no, got, err)
		}
		list, err := rockDB.GetKeywordReserves(no)
		if err != nil || len(list) != 1 || list[0].Week != want {
			t.Fatalf("slot=%d: list=%+v err=%v", no, list, err)
		}
	}
	check(0, 65)
	for _, mask := range []uint8{0, 42} {
		kr.Week = mask
		if err := rockDB.UpdateKeywordReserve(0, kr); err != nil {
			t.Fatal(err)
		}
		check(0, mask)
	}
	configure.CurrentKeywordReserveThreshold = 1
	if _, err := rockDB.Data[0].Exec(`UPDATE keywordreserve SET updatedat=?`, Now().Unix()-10); err != nil {
		t.Fatal(err)
	}
	if err := archiveKeywordReserveContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(1, 42)
	if _, err := rockDB.GetKeywordReserve(0, kr.ID); err != sql.ErrNoRows {
		t.Fatalf("archived reservation remains in current slot: %v", err)
	}
}

func TestKeywordReserveWeekSchedules(t *testing.T) {
	db := newSearchTestDB(t, false)
	if err := CreateTable(db); err != nil {
		t.Fatal(err)
	}
	store := RockDB{Data: []*sql.DB{db}, Program: []*sql.DB{db}}
	programs := TvProgramStore{db: &store}
	previous := scheduler.db
	scheduler.db = &store
	t.Cleanup(func() { scheduler.db = previous })
	s := Scheduler{db: &store, tvp: &programs}
	start := Now().Add(time.Hour)
	for day := range 7 {
		tm := start.AddDate(0, 0, day)
		if _, err := db.Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (101,?,?,?,'ニュース','','',0,0)`,
			day+1, tm.Unix(), tm.Add(time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	for _, mask := range []uint8{0, 1, 2, 64, 65, 127} {
		if _, err := db.Exec(`DELETE FROM schedule`); err != nil {
			t.Fatal(err)
		}
		kr := NewKeywordReserve()
		kr.ID, kr.Title, kr.Sid, kr.Days, kr.Week = 1, "ニュース", []uint{101}, 7*24, mask
		want := 0
		for day := range 7 {
			if mask == 0 || mask&(1<<uint(start.AddDate(0, 0, day).Weekday())) != 0 {
				want++
			}
		}
		count, err := s.addKeywordReserveSchedulesNoNotify(kr)
		if err != nil || count != want {
			t.Fatalf("week=%d: count=%d want=%d err=%v", mask, count, want, err)
		}
		rows, err := s.GetSchedules(0, start.Add(-time.Hour), start.AddDate(0, 0, 7), 0)
		if err != nil || len(rows) != want {
			t.Fatalf("week=%d: schedules=%d want=%d err=%v", mask, len(rows), want, err)
		}
		for _, row := range rows {
			if mask != 0 && mask&(1<<uint(time.Unix(row.StartTime, 0).Weekday())) == 0 {
				t.Fatalf("reservation on an excluded weekday: %+v", row)
			}
		}
	}
}
