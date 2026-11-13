package tvrock

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/blevesearch/bleve/v2"
)

func TestCaptionTextStatements(t *testing.T) {
	for _, no := range []int{0, 1} {
		store := RockDB{Caption: make([]*sql.DB, 2)}
		store.Caption[no] = newSearchTestDB(t, true)
		for _, packet := range []PacketCaption{
			{Pts: 8589934590, Text: "世界"}, {Pts: 8589934590, Text: "遺産"},
			{Pts: 8589934590, Clear: true}, {Pts: 8589934590, Text: "京都"},
			{Pts: 10, Text: "旅行"}, {Pts: 10, Lang: 1, Text: "別言語"},
			{Pts: 10, Lang: 1, Clear: true, Text: "新しい文章"},
		} {
			if err := store.InsertCaption(no, 101, 2, 100, &packet); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.InsertCaption(no, 101, 2, 200, &PacketCaption{Text: "別番組"}); err != nil {
			t.Fatal(err)
		}
		text, err := store.CaptionText(context.Background(), 101, 2, 100)
		if err != nil || text != "世界遺産\n京都\n旅行\n別言語\n新しい文章" {
			t.Fatalf("text = %q, err = %v", text, err)
		}
		matched, err := store.matchProgramCaption(context.Background(), false, true, regexp.MustCompile("世界遺産"), nil, &TvProgram{Sid: 101, Eid: 2, Start: 100})
		if err != nil || !matched {
			t.Fatalf("fragment-spanning DB match = %v, %v", matched, err)
		}
		// Clear must prevent unrelated statements from becoming a single phrase.
		matched, err = store.matchProgramCaption(context.Background(), false, false, regexp.MustCompile("世界遺産京都"), nil, &TvProgram{Sid: 101, Eid: 2, Start: 100})
		if err != nil || matched {
			t.Fatalf("cross-statement match = %v, %v", matched, err)
		}
	}
}

func TestSearchBackendSelection(t *testing.T) {
	previous := configure.Bleve
	t.Cleanup(func() { configure.Bleve = previous })
	for _, enabled := range []bool{false, true} {
		configure.Bleve = enabled
		for _, archive := range []bool{false, true} {
			for _, fuzzy := range []bool{false, true} {
				for _, caption := range []bool{false, true} {
					opt := &SearchOption{ArchiveSearch: archive, FuzzySearch: fuzzy, CaptionSearch: caption}
					if got := opt.useBleve(); got != (enabled && archive && !fuzzy) {
						t.Fatalf("unexpected backend: %+v, enabled=%v", opt, enabled)
					}
				}
			}
		}
	}
}

func TestBleveCaptionSearch(t *testing.T) {
	im, err := newTvProgramBleveMapping()
	if err != nil {
		t.Fatal(err)
	}
	index, err := bleve.NewMemOnly(im)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	store := BleveProgramStore{index: index}
	prg := &TvProgram{Sid: 101, Eid: 2, Start: 100, End: 200, Title: "特別番組"}
	if err := store.BatchIndexProgram(newBleveProgramDocument(prg, "世界遺産\n京都への旅行")); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, title, content string
		captions, and        bool
		want                 uint64
	}{
		{"caption-title", "世界遺産", "", true, false, 1},
		{"caption-content", "", "京都", true, false, 1},
		{"caption-disabled", "世界遺産", "", false, false, 0},
		{"metadata-and-caption", "特別番組", "京都", true, true, 1},
		{"and-missing", "宇宙人", "京都", true, true, 0},
		{"metadata-preserved", "特別番組", "", false, false, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opt := NewSearch()
			opt.Title = tt.title
			opt.Content = tt.content
			opt.StartTime = 100
			opt.EndTime = 100
			opt.CaptionSearch = tt.captions
			opt.AndSearch = tt.and
			res, err := index.Search(bleve.NewSearchRequest(buildTvProgramBleveQuery(opt)))
			if err != nil {
				t.Fatal(err)
			}
			if res.Total != tt.want {
				t.Fatalf("hits = %d, want %d", res.Total, tt.want)
			}
		})
	}
	// Exercise the actual task dispatch: metadata and subtitle searches both use
	// Bleve, even without any database store or compiled regular expression.
	previousIndex, previousEnabled := bleveProgram.index, configure.Bleve
	bleveProgram.index, configure.Bleve = index, true
	t.Cleanup(func() { bleveProgram.index, configure.Bleve = previousIndex, previousEnabled })
	for _, caption := range []bool{false, true} {
		opt := NewSearch()
		opt.StartTime = 100
		opt.EndTime = 100
		opt.ArchiveSearch = true
		opt.FuzzySearch = false
		opt.CaptionSearch = caption
		opt.Title = "特別番組"
		if caption {
			opt.Title = "世界遺産"
		}
		task := SearchTask{ID: ^uint64(0), option: opt, com: make(chan SearchJobCom)}
		task.Exec()
		if len(task.result) != 1 {
			t.Fatalf("task caption=%v, hits=%d", caption, len(task.result))
		}
	}
}

func TestArchiveCaptionBackfillAndResume(t *testing.T) {
	im, err := newTvProgramBleveMapping()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-caption index, including its persisted mapping.
	delete(im.TypeMapping["tvprogram"].Properties, "caption")
	path := filepath.Join(t.TempDir(), "index")
	index, err := bleve.New(path, im)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { index.Close() })
	previousDB, previousIndex := rockDB, bleveProgram.index
	previousEnabled, previousLimit := configure.Bleve, configure.ArchiverSetting.BleveInsertMax
	t.Cleanup(func() {
		rockDB, bleveProgram.index = previousDB, previousIndex
		configure.Bleve, configure.ArchiverSetting.BleveInsertMax = previousEnabled, previousLimit
	})
	rockDB = RockDB{Program: []*sql.DB{nil, newSearchTestDB(t, false)}, Caption: []*sql.DB{nil, newSearchTestDB(t, true)}}
	bleveProgram.index, configure.Bleve, configure.ArchiverSetting.BleveInsertMax = index, true, 1
	for _, start := range []int64{100, 200} {
		if _, err := rockDB.Program[1].Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (101,2,?,?, '番組','','',0,0)`, start, start+100); err != nil {
			t.Fatal(err)
		}
		if err := rockDB.InsertCaption(1, 101, 2, start, &PacketCaption{Text: "京都"}); err != nil {
			t.Fatal(err)
		}
		prg := &TvProgram{Sid: 101, Eid: 2, Start: start, End: start + 100, Title: "番組"}
		if err := index.Index(tvProgramBleveID(prg), newBleveProgramDocument(prg, "")); err != nil {
			t.Fatal(err)
		}
	}
	if err := addCaptionBleveMapping(index); err != nil {
		t.Fatal(err)
	}
	if err := IndexingArchiveProgram(); err != nil {
		t.Fatal(err)
	}
	start, _, _, err := bleveProgram.lastCaptionProgramKey()
	if err != nil || start != 100 {
		t.Fatalf("first backfill cursor=%d, err=%v", start, err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	index, err = bleve.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	bleveProgram.index = index
	if err := addCaptionBleveMapping(index); err != nil {
		t.Fatal(err)
	}
	if err := IndexingArchiveProgram(); err != nil {
		t.Fatal(err)
	}
	start, _, _, err = bleveProgram.lastCaptionProgramKey()
	if err != nil || start != 200 {
		t.Fatalf("resumed cursor=%d, err=%v", start, err)
	}
	if err := IndexingArchiveProgram(); err != nil {
		t.Fatal(err)
	}
	count, err := index.DocCount()
	if err != nil || count != 2 {
		t.Fatalf("documents=%d, err=%v", count, err)
	}
	opt := NewSearch()
	opt.Title = "京都"
	opt.CaptionSearch = true
	opt.StartTime = 1
	opt.EndTime = 300
	res, err := index.Search(bleve.NewSearchRequest(buildTvProgramBleveQuery(opt)))
	if err != nil || res.Total != 2 {
		t.Fatalf("caption backfill results=%+v, err=%v", res, err)
	}
}
