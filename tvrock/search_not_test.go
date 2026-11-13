package tvrock

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSearchNOTPatterns(t *testing.T) {
	for _, tt := range []struct {
		query, text string
		fuzzy, want bool
	}{
		{"ニュース NOT QZX", "夕方ニュース", false, true},
		{"ニュース NOT QZX", "QZXニュース", false, false},
		{"ニュース NOT QZX", "ドラマ", false, false},
		{"ニュース NOT QZX", "ＱＺＸにゅーす", true, false},
		{"ニュース NOT QZX", "にゅーす", true, true},
		{"ニュース NOT ミラキュア", "ニュース ミラキア", true, false},
		{"ニュース NOT QZX", "qzxニュース", false, true},
		{"ニュース　NOT　QZX", "QZXニュース", false, false},
		{"ニュース NOT QZX 特集", "ニュース QZX", false, true},
		{"ニュース NOT QZX 特集", "ニュース QZX 特集", false, false},
		{"ニュース NOT QZX|再放送", "ニュース 再放送", false, false},
		{"NOTHING", "NOTHING", false, true},
		{"ニュースnotQZX", "ニュースnotQZX", false, true},
	} {
		include, exclude, err := compileSearchPattern(tt.query, tt.fuzzy)
		if err != nil {
			t.Fatal(err)
		}
		got := matchRegexpText(include, tt.text, tt.fuzzy) && !matchRegexpText(exclude, tt.text, tt.fuzzy)
		if got != tt.want {
			t.Errorf("query=%q text=%q fuzzy=%v: got %v, want %v", tt.query, tt.text, tt.fuzzy, got, tt.want)
		}
	}
	for _, query := range []string{"NOT QZX", "ニュース NOT", "ニュース NOT [", "[ NOT QZX"} {
		if _, _, err := compileSearchPattern(query, true); err == nil {
			t.Errorf("expected error for %q", query)
		}
	}
}

func TestSearchNOTArchive(t *testing.T) {
	previousDB, previousPrograms := rockDB, tvprogram
	previousBleve, previousLimit := configure.Bleve, configure.SearchLimit
	t.Cleanup(func() {
		rockDB, tvprogram = previousDB, previousPrograms
		configure.Bleve, configure.SearchLimit = previousBleve, previousLimit
	})
	configure.Bleve, configure.SearchLimit = true, 1
	rockDB = RockDB{Program: []*sql.DB{newSearchTestDB(t, false), newSearchTestDB(t, false)}}
	tvprogram = TvProgramStore{db: &rockDB, FirstStartTime: []int64{100, 1}, LastEndTime: []int64{200, 200}}
	for i, title := range []string{"QZXニュース", "夕方ニュース"} {
		if _, err := rockDB.Program[1].Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (101,?,?,150,?,'','',0,0)`, i+1, 100+i*10, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, fuzzy := range []bool{false, true} {
		opt := NewSearch()
		opt.Title, opt.ArchiveSearch, opt.FuzzySearch = "ニュース NOT QZX", true, fuzzy
		opt.StartTime, opt.EndTime = 100, 100
		if opt.useBleve() {
			t.Fatal("NOT must use database scan even with Bleve enabled")
		}
		include, exclude, err := compileSearchPattern(opt.Title, fuzzy)
		if err != nil {
			t.Fatal(err)
		}
		task := SearchTask{option: opt, title: include, exclusions: searchExclusions{title: exclude}, current: 100, leng: 100, sid: []uint{101}, com: make(chan SearchJobCom)}
		task.Exec()
		if task.state != SearchJobFinish || len(task.result) != 1 || task.result[0].Title != "夕方ニュース" {
			t.Fatalf("fuzzy=%v: state=%v, results=%+v", fuzzy, task.state, task.result)
		}
	}
}

func TestSearchNOTCaptionAndContent(t *testing.T) {
	store := RockDB{Caption: []*sql.DB{newSearchTestDB(t, true)}}
	for _, text := range []string{"ニュース", "QZX"} {
		if err := store.InsertCaption(0, 101, 2, 100, &PacketCaption{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	include, exclude, err := compileSearchPattern("ニュース NOT QZX", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, prg := range []*TvProgram{
		{Sid: 101, Eid: 2, Start: 100, Title: "ニュース"},
		{Sid: 101, Eid: 2, Start: 100, Title: "別番組"},
	} {
		got, err := store.matchProgramCaption(context.Background(), false, true, include, nil, prg, searchExclusions{title: exclude})
		if err != nil || got {
			t.Fatalf("caption exclusion: match=%v, err=%v", got, err)
		}
	}
	for _, prg := range []*TvProgram{{Subtitle: "QZX"}, {Content: "ＱＺＸ"}} {
		if !(searchExclusions{content: exclude}).matchesProgram(prg, true) {
			t.Fatal("content exclusion must cover subtitle and content")
		}
	}
}

func TestKeywordReserveNOT(t *testing.T) {
	db := newSearchTestDB(t, false)
	if err := CreateTable(db); err != nil {
		t.Fatal(err)
	}
	store := RockDB{Data: []*sql.DB{db}, Program: []*sql.DB{db}}
	programs := TvProgramStore{db: &store}
	previousDB := scheduler.db
	scheduler.db = &store
	t.Cleanup(func() { scheduler.db = previousDB })
	s := Scheduler{db: &store, tvp: &programs}
	start := Now().Add(time.Hour).Unix()
	for i, title := range []string{"QZXニュース", "夕方ニュース"} {
		if _, err := db.Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (101,?,?,?,?,'','',0,0)`, i+1, start+int64(i)*1800, start+int64(i+1)*1800, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, fuzzy := range []bool{false, true} {
		if _, err := db.Exec(`DELETE FROM schedule`); err != nil {
			t.Fatal(err)
		}
		kr := NewKeywordReserve()
		// Days stores a duration in hours; include both future programs.
		kr.ID, kr.Title, kr.Sid, kr.Days, kr.FuzzySearch = 1, "ニュース NOT QZX", []uint{101}, 24, fuzzy
		count, err := s.addKeywordReserveSchedulesNoNotify(kr)
		if err != nil || count != 1 {
			t.Fatalf("fuzzy=%v: count=%d err=%v", fuzzy, count, err)
		}
		var title string
		if err := db.QueryRow(`SELECT title FROM schedule`).Scan(&title); err != nil || title != "夕方ニュース" {
			t.Fatalf("reserved title=%q err=%v", title, err)
		}
	}
}
