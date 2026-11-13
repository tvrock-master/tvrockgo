package tvrock

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/gin-gonic/gin"
)

func TestArchiveSearchPeriod(t *testing.T) {
	loc := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, loc)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)
	for _, tt := range []struct {
		name, start, end   string
		wantStart, wantEnd int64
		invalid            bool
	}{
		{"same-day", "2026-09-01", "2026-09-01", day.Unix(), day.AddDate(0, 0, 1).Unix(), false},
		{"unspecified", "", "", 100, now.Unix(), false},
		{"start-only", "2026-09-01", "", day.Unix(), now.Unix(), false},
		{"end-only", "", "2026-09-01", 100, day.AddDate(0, 0, 1).Unix(), false},
		{"datetime", "2026-09-01T00:00", "2026-09-01T01:00:00+09:00", day.Unix(), day.Add(time.Hour).Unix(), false},
		{"invalid-day", "2026-02-30", "", 0, 0, true},
		{"reversed", "2026-09-03", "2026-09-01", 0, 0, true},
		{"invalid-end", "", "invalid", 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := archiveSearchPeriod(tt.start, tt.end, 100, now)
			if (err != nil) != tt.invalid || (!tt.invalid && (start != tt.wantStart || end != tt.wantEnd)) {
				t.Fatalf("period = (%d, %d, %v), want (%d, %d), invalid=%v", start, end, err, tt.wantStart, tt.wantEnd, tt.invalid)
			}
		})
	}
	start, end, err := archiveSearchPeriod("", "2026-09-01", now.Unix(), now)
	if err != nil || start != end {
		t.Fatalf("expected empty period before stored data, got %d, %d, %v", start, end, err)
	}
}

func TestAPIDoSearchRejectsInvalidArchivePeriod(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"sid":[101],"archivesearch":true,"starttime":"invalid"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	APIDoSearch(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func newSearchTestDB(t *testing.T, caption bool) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if caption {
		err = CreateCaptionTable(db)
	} else {
		err = CreateProgramTable(db)
	}
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSearchCaptionMatching(t *testing.T) {
	store := RockDB{Caption: []*sql.DB{newSearchTestDB(t, true), newSearchTestDB(t, true)}}
	for _, row := range []struct {
		no       int
		sid, eid uint
		start    int64
		text     string
	}{
		{0, 101, 2, 100, "どうして？教えて"},
		{1, 101, 2, 100, "世界遺産"},
		{1, 101, 2, 100, "歴史特集"},
		{1, 999, 2, 100, "他局限定"},
		{1, 101, 999, 100, "別番号限定"},
		{1, 101, 2, 999, "別日時限定"},
	} {
		if err := store.InsertCaption(row.no, row.sid, row.eid, row.start, &PacketCaption{Text: row.text}); err != nil {
			t.Fatal(err)
		}
	}
	prg := &TvProgram{Sid: 101, Eid: 2, Start: 100, Title: "番組タイトル"}
	for _, tt := range []struct {
		name, title, content string
		and, fuzzy, want     bool
	}{
		{"current-title", "どうして？教えて", "", false, true, true},
		{"archive-content", "", "世界遺産", false, false, true},
		{"metadata-and-caption", "番組タイトル", "世界遺産", true, false, true},
		{"different-caption-rows", "世界遺産", "歴史特集", true, false, true},
		{"and-missing", "不存在", "世界遺産", true, false, false},
		{"wrong-sid", "他局限定", "", false, false, false},
		{"wrong-eid", "別番号限定", "", false, false, false},
		{"wrong-start", "別日時限定", "", false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			compile := func(s string) *regexp.Regexp {
				if s == "" {
					return nil
				}
				if tt.fuzzy {
					s = fuzzySearchPattern(s)
				}
				return regexp.MustCompile(s)
			}
			got, err := store.matchProgramCaption(context.Background(), tt.and, tt.fuzzy, compile(tt.title), compile(tt.content), prg)
			if err != nil || got != tt.want {
				t.Fatalf("match = %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestSearchExecArchivePeriodAndCaption(t *testing.T) {
	previousDB, previousPrograms := rockDB, tvprogram
	previousBleve, previousLimit := configure.Bleve, configure.SearchLimit
	t.Cleanup(func() {
		rockDB, tvprogram = previousDB, previousPrograms
		configure.Bleve, configure.SearchLimit = previousBleve, previousLimit
	})
	rockDB = RockDB{Program: []*sql.DB{newSearchTestDB(t, false), newSearchTestDB(t, false)}, Caption: []*sql.DB{newSearchTestDB(t, true), newSearchTestDB(t, true)}}
	tvprogram = TvProgramStore{db: &rockDB, FirstStartTime: []int64{100, 1}, LastEndTime: []int64{400, 400}}
	for no := range 2 {
		for _, row := range []struct {
			start, end int
			title      string
		}{{1, 100, "一致"}, {100, 110, "対象外"}, {110, 150, "字幕番組"}, {150, 250, "一致"}, {250, 300, "一致"}, {300, 400, "一致"}} {
			if _, err := rockDB.Program[no].Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (101,2,?,?,?,'','',0,0)`, row.start, row.end, row.title); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := rockDB.InsertCaption(1, 101, 2, 110, &PacketCaption{Text: "一致"}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name                   string
		captions, bleve, fuzzy bool
		limit, want            int
	}{
		{"metadata", false, false, false, 0, 2},
		{"caption", true, false, false, 0, 3},
		{"fuzzy-caption-with-bleve-enabled", true, true, true, 0, 3},
		{"result-limit-after-filtering", true, false, false, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configure.Bleve, configure.SearchLimit = tt.bleve, tt.limit
			opt := NewSearch()
			opt.StartTime = 100
			opt.EndTime = 200
			opt.ArchiveSearch = true
			opt.CaptionSearch = tt.captions
			opt.FuzzySearch = tt.fuzzy
			task := SearchTask{ID: ^uint64(0), option: opt, title: regexp.MustCompile("一致"), current: 100, leng: 100, sid: []uint{101}, com: make(chan SearchJobCom)}
			task.Exec()
			if task.state != SearchJobFinish || len(task.result) != tt.want {
				t.Fatalf("state=%v hits=%d, want %d", task.state, len(task.result), tt.want)
			}
			for _, prg := range task.result {
				if prg.Start < 100 || prg.Start >= 300 {
					t.Fatalf("outside period: %+v", prg)
				}
			}
		})
	}
}

func TestBleveSearchPeriodQuery(t *testing.T) {
	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	for _, doc := range []struct {
		id         string
		start, end int64
	}{{"before", 1, 100}, {"inside", 100, 200}, {"overlap", 90, 110}, {"after", 200, 300}} {
		if err := index.Index(doc.id, map[string]interface{}{"title": "example", "start": doc.start, "end": doc.end}); err != nil {
			t.Fatal(err)
		}
	}
	opt := NewSearch()
	opt.Title = "example"
	opt.StartTime = 100
	opt.EndTime = 100
	res, err := index.Search(bleve.NewSearchRequest(buildTvProgramBleveQuery(opt)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("hits = %d, want 2", res.Total)
	}
	for _, hit := range res.Hits {
		if hit.ID != "inside" && hit.ID != "overlap" {
			t.Fatalf("outside period: %s", hit.ID)
		}
	}
}
