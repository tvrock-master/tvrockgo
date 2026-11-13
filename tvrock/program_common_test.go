package tvrock

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func uintRef(value uint) *uint { return &value }

func setupCommonProgramTest(t *testing.T) (uint, uint, int64) {
	t.Helper()
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		if err := CreateTable(db); err != nil {
			t.Fatal(err)
		}
		if err := CreateProgramTable(db); err != nil {
			t.Fatal(err)
		}
		return db
	}
	current, archive := open(), open()
	oldCache, oldStore, oldDB, oldQueue := station.Cache, tvprogram, scheduler.db, scheduler.chUpdate
	t.Cleanup(func() {
		station.Cache, tvprogram, scheduler.db, scheduler.chUpdate = oldCache, oldStore, oldDB, oldQueue
	})
	parent, child := uint(2<<16|101), uint(2<<16|102)
	station.Cache = map[uint]*TvStation{parent: {Sid: parent, Tsp: 2, Tsid: 16625}, child: {Sid: child, Tsp: 2, Tsid: 16625, Parent: parent}}
	scheduler.db = &RockDB{Data: []*sql.DB{current, archive}, Program: []*sql.DB{current, archive}}
	scheduler.chUpdate = make(chan SchedulerRequest, 16)
	tvprogram = TvProgramStore{db: scheduler.db, FirstStartTime: []int64{0, 0}, LastEndTime: []int64{0, 1 << 62}}
	return parent, child, Now().Add(time.Hour).Truncate(time.Minute).Unix()
}

func TestCommonProgramKeepsChildIdentityAndTime(t *testing.T) {
	parent, child, start := setupCommonProgramTest(t)
	source := &TvProgram{Sid: parent, Eid: 10, Start: start, End: start + 3600, Title: "shared title", Subtitle: "description", Content: "detail", CnibA: 7, CnibB: 1}
	sharing := &TvProgram{Sid: child, Eid: 20, Start: start + 600, End: start + 2400, CommonSid: uintRef(parent), CommonEid: uintRef(10)}
	for _, program := range []*TvProgram{source, sharing} {
		if err := tvprogram.Insert(0, program); err != nil {
			t.Fatal(err)
		}
	}
	check := func(program *TvProgram) {
		t.Helper()
		if program.Sid != child || program.Eid != 20 || program.Start != sharing.Start || program.End != sharing.End || program.Title != source.Title || program.Content != source.Content || program.CnibA != 7 {
			t.Fatalf("resolved=%+v", program)
		}
	}
	for _, get := range []func(int, uint, uint) ([]*TvProgram, error){tvprogram.Get, tvprogram.GetWithEID} {
		got, err := get(0, child, 20)
		if err != nil || len(got) != 1 {
			t.Fatalf("get=%+v err=%v", got, err)
		}
		check(got[0])
	}
	guide, err := GetTvGuide([]uint{child}, time.Unix(start, 0), time.Unix(start+3600, 0), 0)
	if err != nil || len(guide.Tvp[child]) != 1 {
		t.Fatalf("guide=%+v err=%v", guide, err)
	}
	check(guide.Tvp[child][0])
	week, err := GetTvGuideWeek([]uint{child}, time.Unix(start, 0), time.Unix(start+3600, 0), 0)
	if err != nil || len(week.Tvp[0]) != 1 {
		t.Fatalf("week=%+v err=%v", week, err)
	}
	check(week.Tvp[0][0])
	count := 0
	if err := tvprogram.GetFromSID(0, []uint{child}, time.Unix(start, 0), time.Unix(start+3600, 0), 0, func(program *TvProgram) bool { check(program); count++; return true }); err != nil || count != 1 {
		t.Fatalf("search count=%d err=%v", count, err)
	}
	stored, err := tvprogram.getStoredWithEID(0, child, 20)
	if err != nil || stored[0].Title != "" || !sameOptionalUint(stored[0].CommonEid, uintRef(10)) {
		t.Fatal("raw sharing event was overwritten", stored, err)
	}
	device := &RockDevice{CurrentProgram: stored[0]}
	displayed, err := tvprogram.resolveDevicePrograms(device)
	if err != nil || displayed.CurrentProgram.Title != source.Title || device.CurrentProgram.Title != "" {
		t.Fatal("device snapshot changed the raw PF or failed to resolve", displayed, err)
	}
	// 親子設定だけでは、子に存在しないイベントを作らない。
	missing, err := tvprogram.Get(0, child, 10)
	if err != nil || len(missing) != 0 {
		t.Fatal(missing, err)
	}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/schedule", strings.NewReader(fmt.Sprintf(`{"sid":%d,"eid":20,"start":%d,"end":%d}`, child, sharing.Start, sharing.End)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	APIAddSchedule(ctx)
	if rec.Code != 200 || len(scheduler.chUpdate) != 1 {
		t.Fatalf("reserve=%d %s", rec.Code, rec.Body.String())
	}
	rs := (<-scheduler.chUpdate).Schedule[0]
	if rs.Sid != child || rs.Eid != 20 || rs.StartTime != sharing.Start || rs.EndTime != sharing.End || rs.Title != source.Title {
		t.Fatal(rs)
	}
	if _, err := scheduler.Insert(0, rs); err != nil {
		t.Fatal(err)
	}
	source.Title = "updated title"
	source.End += 600
	if err := tvprogram.Insert(0, source); err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.EpgFollow([]*RecordingSchedule{rs}); err != nil {
		t.Fatal(err)
	}
	if rs.Title != source.Title || rs.EndTime != sharing.End || rs.Eid != 20 {
		t.Fatal("follow borrowed parent identity/time", rs)
	}
}

func TestCommonProgramLateSourceInvalidReferenceAndClear(t *testing.T) {
	parent, child, start := setupCommonProgramTest(t)
	sharing := &TvProgram{Sid: child, Eid: 20, Start: start, End: start + 1800, CommonSid: uintRef(parent), CommonEid: uintRef(10)}
	if err := tvprogram.Insert(0, sharing); err != nil {
		t.Fatal(err)
	}
	got, err := tvprogram.Get(0, child, 20)
	if err != nil || got[0].Title != "" {
		t.Fatal(got, err)
	}
	source := &TvProgram{Sid: parent, Eid: 10, Start: start, End: start + 1800, Title: "late source"}
	if err := tvprogram.Insert(0, source); err != nil {
		t.Fatal(err)
	}
	got, err = tvprogram.Get(0, child, 20)
	if err != nil || got[0].Title != "late source" {
		t.Fatal(got, err)
	}
	source.CommonSid, source.CommonEid = uintRef(child), uintRef(20)
	if err := tvprogram.Insert(0, source); err != nil {
		t.Fatal(err)
	}
	got, err = tvprogram.Get(0, child, 20)
	if err != nil || got[0].Title != "" {
		t.Fatal("cyclic reference borrowed data", got, err)
	}
	sharing.CommonSid, sharing.CommonEid = nil, nil
	sharing.Title = "independent"
	if err := tvprogram.ReplaceBatchContext(context.Background(), 0, []TvProgram{*sharing}); err != nil {
		t.Fatal(err)
	}
	got, err = tvprogram.Get(0, child, 20)
	if err != nil || got[0].Title != "independent" || got[0].CommonSid != nil {
		t.Fatal(got, err)
	}
}

func TestCommonProgramReceiveAndArchive(t *testing.T) {
	parent, child, start := setupCommonProgramTest(t)
	var received []TvProgram
	text := fmt.Sprintf(`[{"Sid":102,"Eid":20,"Start":%d,"End":%d,"CommonSid":101,"CommonEid":10}]`, start, start+1800)
	if err := json.Unmarshal([]byte(text), &received); err != nil {
		t.Fatal(err)
	}
	dev := &RockDevice{CurrentService: child}
	dev.UpdateTVProgram(received)
	got, err := tvprogram.getStoredWithEID(0, child, 20)
	if err != nil || len(got) != 1 || !sameOptionalUint(got[0].CommonSid, uintRef(parent)) {
		t.Fatal(got, err)
	}
	pf := &TvProgram{Sid: child, Eid: 20, Start: start, End: start + 1800, CommonSid: uintRef(parent), CommonEid: uintRef(11)}
	if _, err := dev.updatePFProgram(pf, start-1, start+3600); err != nil {
		t.Fatal(err)
	}
	got, err = tvprogram.getStoredWithEID(0, child, 20)
	if err != nil || !sameOptionalUint(got[0].CommonEid, uintRef(11)) {
		t.Fatal("PF lost shared target", got, err)
	}
	old := Now().Unix() - configure.CurrentProgramThreshold - 3600
	for _, program := range []TvProgram{{Sid: parent, Eid: 30, Start: old, End: old + 1800, Title: "archived source"}, {Sid: child, Eid: 40, Start: old, End: old + 1800, CommonSid: uintRef(parent), CommonEid: uintRef(30)}} {
		if err := tvprogram.Replace(0, &program); err != nil {
			t.Fatal(err)
		}
	}
	previous := rockDB
	rockDB = *tvprogram.db
	t.Cleanup(func() { rockDB = previous })
	if err := archiveProgramContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = tvprogram.GetWithEID(1, child, 40)
	if err != nil || len(got) != 1 || got[0].Title != "archived source" || !sameOptionalUint(got[0].CommonEid, uintRef(30)) {
		t.Fatal("archive lost reference", got, err)
	}
}
