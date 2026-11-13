package tvrock

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUpdateCaptionIncludesSourceDevice(t *testing.T) {
	originalHandlers := deviceController.Handler
	originalRecord := configure.CaptionRecord
	t.Cleanup(func() {
		deviceController.Handler = originalHandlers
		configure.CaptionRecord = originalRecord
	})
	queue := &DevConQueue{Com: make(chan string, 2), Done: make(chan struct{})}
	deviceController.Handler = map[string]*DevConQueue{"rockbar": queue}
	configure.CaptionRecord = false
	program := &TvProgram{Sid: 101, Eid: 2, Start: 100, End: 200}
	for _, id := range []string{"REC01", "REC02"} {
		device := RockDevice{ID: id, CurrentProgram: program}
		if err := device.UpdateCaption(&PacketCaption{Text: "字幕", HistoryID: 123}); err != nil {
			t.Fatal(err)
		}
		message := <-queue.Com
		if !strings.HasPrefix(message, ControllerSendCaption) {
			t.Fatalf("unexpected notification: %s", message)
		}
		var caption CaptionInfo
		if err := json.Unmarshal([]byte(strings.TrimPrefix(message, ControllerSendCaption)), &caption); err != nil {
			t.Fatal(err)
		}
		if caption.DeviceID != id || caption.Sid != program.Sid || caption.Text != "字幕" || caption.HistoryID != 0 {
			t.Fatalf("incorrect caption source or metadata: %+v", caption)
		}
	}
}

func TestCaptionHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateCaptionTable(db); err != nil {
		t.Fatal(err)
	}
	store := RockDB{Caption: []*sql.DB{db}}
	for _, packet := range []PacketCaption{
		{Pts: 8589934590, Text: "最", Color: 3, ColorName: "yellow"}, {Pts: 8589934590, Text: "初", Color: 6, ColorName: "cyan"}, {Pts: 0, Clear: true},
		{Pts: 10, Text: "次"}, {Pts: -1, Clear: true, Text: "消去付き本文"},
	} {
		if err := store.InsertCaption(0, 101, 2, 100, &packet); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.InsertCaption(0, 101, 2, 200, &PacketCaption{Text: "別番組"}); err != nil {
		t.Fatal(err)
	}
	items, err := store.CaptionHistory(context.Background(), 101, 2, 100, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Text != "最初" || items[1].Text != "次" {
		t.Fatalf("unexpected history: %+v", items)
	}
	if len(items[0].Segments) != 2 || items[0].Segments[0].Text != "最" || items[0].Segments[0].Color != 3 || items[0].Segments[1].Text != "初" || items[0].Segments[1].ColorName != "cyan" {
		t.Fatalf("lost fragment colors: %+v", items[0].Segments)
	}
	next, err := store.CaptionHistory(context.Background(), 101, 2, 100, items[1].ID, 2)
	if err != nil || len(next) != 1 || next[0].Text != "消去付き本文" {
		t.Fatalf("unexpected page: %+v, %v", next, err)
	}
	missing, err := store.CaptionHistory(context.Background(), 102, 2, 100, 0, 2)
	if err != nil || len(missing) != 0 {
		t.Fatalf("unexpected missing result: %+v, %v", missing, err)
	}
	archive := RockDB{Caption: []*sql.DB{nil, db}}
	items, err = archive.CaptionHistory(context.Background(), 101, 2, 100, 0, 2)
	if err != nil || len(items) != 2 {
		t.Fatalf("archive: %+v, %v", items, err)
	}
}

func TestCaptionHistoryJoinsFragmentsAcrossPageBoundary(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateCaptionTable(db); err != nil {
		t.Fatal(err)
	}
	store := RockDB{Caption: []*sql.DB{db}}
	for _, packet := range []PacketCaption{
		{Pts: 100, Text: "「"}, {Pts: 100, Text: "えっ"},
		{Pts: 100, Text: "今の方って"}, {Pts: 100, Text: "あの"}, {Pts: 100, Text: "」"},
		{Pts: 100, Clear: true}, {Pts: 100, Text: "次の文章"},
		{Pts: 100, Lang: 1, Text: "別言語"},
	} {
		if err := store.InsertCaption(0, 101, 2, 100, &packet); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.CaptionHistory(context.Background(), 101, 2, 100, 0, 1)
	if err != nil || len(items) != 1 || items[0].Text != "「えっ今の方ってあの」" || items[0].ID != 5 {
		t.Fatalf("fragment grouping: %+v, %v", items, err)
	}
	items, err = store.CaptionHistory(context.Background(), 101, 2, 100, items[0].ID, 2)
	if err != nil || len(items) != 2 || items[0].Text != "次の文章" || items[1].Text != "別言語" {
		t.Fatalf("statement boundaries: %+v, %v", items, err)
	}
	snapshot, err := store.CaptionSnapshot(context.Background(), 101, 2, 100)
	if err != nil || snapshot != 8 {
		t.Fatalf("snapshot: %d, %v", snapshot, err)
	}
	packet := PacketCaption{Pts: 100, Lang: 1, Text: "追加"}
	if err := store.InsertCaption(0, 101, 2, 100, &packet); err != nil {
		t.Fatal(err)
	}
	if packet.HistoryID != 9 {
		t.Fatalf("missing transport ID: %+v", packet)
	}
	items, err = store.CaptionHistory(context.Background(), 101, 2, 100, 7, 2, snapshot)
	if err != nil || len(items) != 1 || items[0].Text != "別言語" || items[0].Pts != 100 {
		t.Fatalf("snapshot changed while paging: %+v, %v", items, err)
	}
}

func TestCaptionHistoryRejectsInvalidQuery(t *testing.T) {
	for _, query := range []string{"", "sid=1&eid=2", "sid=-1&eid=2&start=100", "sid=1&eid=2&start=100&after=-1", "sid=1&eid=2&start=no"} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest("GET", "/caption?"+query, nil)
		APIGetCaptionHistory(c)
		if response.Code != 400 {
			t.Fatalf("query %q: status %d", query, response.Code)
		}
	}
}

func TestCaptionTimingAndLayoutSurviveTransport(t *testing.T) {
	var packet PacketCaption
	err := json.Unmarshal([]byte(`{"Pid":278,"Pts":-1,"Lang":1,"Timing":1,"DelayMs":4200,"BgColor":65,"Size":"small","Text":"よみ","Layout":{"X":170,"Y":439,"CellWidth":20,"CellHeight":30,"GlyphWidth":18,"GlyphHeight":18,"Cells":2}}`), &packet)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(CaptionInfo{Sid: 101, PacketCaption: packet})
	if err != nil {
		t.Fatal(err)
	}
	var decoded CaptionInfo
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Pts != -1 || decoded.DelayMs != 4200 || decoded.Timing != 1 || decoded.Layout == nil || decoded.Layout.GlyphHeight != 18 || decoded.BgColor != 65 {
		t.Fatalf("caption metadata lost: %s", encoded)
	}
}

func TestArchiveCaption(t *testing.T) {
	db0, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db0.Close()
	db0.SetMaxOpenConns(1)
	if err := CreateCaptionTable(db0); err != nil {
		t.Fatal(err)
	}

	db1, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db1.Close()
	db1.SetMaxOpenConns(1)
	if err := CreateCaptionTable(db1); err != nil {
		t.Fatal(err)
	}

	origCaption := rockDB.Caption
	origThreshold := configure.CurrentCaptionThreshold
	defer func() {
		rockDB.Caption = origCaption
		configure.CurrentCaptionThreshold = origThreshold
	}()

	rockDB.Caption = []*sql.DB{db0, db1}
	configure.CurrentCaptionThreshold = 100

	now := Now().Unix()
	oldTime := now - 200
	for _, p := range []PacketCaption{
		{Pts: 10, Text: "古い字幕1", Clear: false, ColorName: "white", Size: "normal"},
		{Pts: 20, Text: "古い字幕2", Clear: true, ColorName: "white", Size: "normal"},
	} {
		if err := rockDB.InsertCaption(0, 101, 1, oldTime, &p); err != nil {
			t.Fatal(err)
		}
	}

	recentTime := now - 50
	if err := rockDB.InsertCaption(0, 101, 2, recentTime, &PacketCaption{Pts: 30, Text: "新しい字幕", ColorName: "white", Size: "normal"}); err != nil {
		t.Fatal(err)
	}

	if err := ArchiveCaption(); err != nil {
		t.Fatalf("ArchiveCaption failed: %v", err)
	}

	var count0 int
	if err := db0.QueryRow("SELECT COUNT(*) FROM caption").Scan(&count0); err != nil {
		t.Fatal(err)
	}
	if count0 != 1 {
		t.Fatalf("expected 1 record in db0, got %d", count0)
	}

	var count1 int
	if err := db1.QueryRow("SELECT COUNT(*) FROM caption").Scan(&count1); err != nil {
		t.Fatal(err)
	}
	if count1 != 2 {
		t.Fatalf("expected 2 records in db1, got %d", count1)
	}
}

func TestCaptionHistoryOmitsRuby(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := CreateCaptionTable(db); err != nil {
		t.Fatal(err)
	}
	store := RockDB{Caption: []*sql.DB{db}}
	for _, packet := range []PacketCaption{
		{Pts: 100, Size: "small", Text: "かんじ"},
		{Pts: 100, Size: "normal", Text: "漢字"},
		{Pts: 100, Size: "micro", Text: "よみ"},
		{Pts: 100, Size: "medium", Text: "の本文"},
		{Pts: 100, Size: "small", Text: "ほんぶん"},
		{Pts: 100, Size: "small", Clear: true, Text: "つぎ"},
		{Pts: 100, Text: "次"},
	} {
		if err := store.InsertCaption(0, 101, 2, 100, &packet); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.CaptionHistory(context.Background(), 101, 2, 100, 0, 1)
	if err != nil || len(items) != 1 || items[0].Text != "漢字の本文" || items[0].ID != 5 {
		t.Fatalf("ruby in paginated history: %+v, %v", items, err)
	}
	next, err := store.CaptionHistory(context.Background(), 101, 2, 100, items[0].ID, 1)
	if err != nil || len(next) != 1 || next[0].Text != "次" {
		t.Fatalf("clear on ruby lost: %+v, %v", next, err)
	}
	items, err = store.CaptionHistory(context.Background(), 101, 2, 100, 0, 10, 5)
	if err != nil || len(items) != 1 || items[0].ID != 5 || items[0].Text != "漢字の本文" {
		t.Fatalf("snapshot ending in ruby: %+v, %v", items, err)
	}
	items, err = store.CaptionHistory(context.Background(), 101, 2, 100, 0, 10, 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("ruby-only snapshot: %+v, %v", items, err)
	}
}
