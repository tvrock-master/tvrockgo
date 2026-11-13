package tvrock

import (
	"database/sql"
	"sync"
	"testing"
)

// These tests isolate relay processing from real devices and persistent databases.
func setupEventRelayTest(t *testing.T) PacketEventRelay {
	t.Helper()
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
	oldDB, oldQueue, oldStore := scheduler.db, scheduler.chUpdate, tvprogram
	t.Cleanup(func() { scheduler.db, scheduler.chUpdate, tvprogram = oldDB, oldQueue, oldStore })
	scheduler.db = &RockDB{Data: []*sql.DB{db}, Program: []*sql.DB{db}}
	scheduler.chUpdate = make(chan SchedulerRequest, 16)
	tvprogram = TvProgramStore{db: scheduler.db}
	source := scheduler.New()
	source.Sid, source.Eid = uint(TuningSpaceTS)<<16|101, 11
	source.EndTime = source.StartTime + 3600
	if _, err := scheduler.Insert(0, source); err != nil {
		t.Fatal(err)
	}
	return PacketEventRelay{GroupType: 2, Sid: 101, Eid: 11, RelaySid: 102, RelayEid: 12}
}

func relayTargetProgram() *TvProgram {
	now := Now().Unix()
	return &TvProgram{Sid: uint(TuningSpaceTS)<<16 | 102, Eid: 12, Title: "relay target", Start: now, End: now + 3600}
}

func TestEventRelayKnownAndUnknownProgram(t *testing.T) {
	for _, known := range []bool{true, false} {
		name := "unknown"
		if known {
			name = "known"
		}
		t.Run(name, func(t *testing.T) {
			evr := setupEventRelayTest(t)
			prg := relayTargetProgram()
			if known {
				if err := tvprogram.Insert(0, prg); err != nil {
					t.Fatal(err)
				}
			}
			dev := &RockDevice{}
			if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
				t.Fatal(err)
			}
			if len(scheduler.chUpdate) != 1 {
				t.Fatalf("expected one add request, got %d", len(scheduler.chUpdate))
			}
			<-scheduler.chUpdate
			rs, err := GetSelectedSchedule(prg.Sid, prg.Eid)
			if err != nil || rs == nil {
				t.Fatalf("relay not persisted: %v", err)
			}
			if rs.Sid != prg.Sid || rs.Eid != prg.Eid || !rs.EpgFollow || !rs.Validate {
				t.Fatalf("invalid relay schedule: %+v", rs)
			}
			if known && (rs.Title != prg.Title || rs.StartTime != prg.Start || rs.EndTime != prg.End) {
				t.Fatalf("program not copied: %+v", rs)
			}
			if !known && (rs.Title != "未定" || rs.EndTime-rs.StartTime != 600) {
				t.Fatalf("unexpected placeholder: %+v", rs)
			}
			if !known && rs.StartTime < prg.Start+3600 {
				t.Fatal("placeholder starts before the source ends")
			}
			if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
				t.Fatal(err)
			}
			if len(scheduler.chUpdate) != 0 {
				t.Fatal("already persisted target was duplicated")
			}
			if !known {
				if err := tvprogram.Insert(0, prg); err != nil {
					t.Fatal(err)
				}
				all, err := scheduler.GetScheduleAll()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := scheduler.EpgFollow(all); err != nil {
					t.Fatal(err)
				}
				got, err := GetSelectedSchedule(prg.Sid, prg.Eid)
				if err != nil {
					t.Fatal(err)
				}
				if got.Title != prg.Title || got.EndTime != prg.End {
					t.Fatalf("explicit EPG follow failed: %+v", got)
				}
			}
		})
	}
}

func TestEventRelayDoesNotDuplicateTargets(t *testing.T) {
	evr := setupEventRelayTest(t)
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr, evr}); err != nil {
		t.Fatal(err)
	}
	if got := len(scheduler.chUpdate); got != 1 {
		t.Fatalf("got %d update notifications; want 1", got)
	}
	all, err := scheduler.GetScheduleAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want source and one relay, got %d", len(all))
	}
}

func TestEventRelayFirstPFNotifiesScheduler(t *testing.T) {
	evr := setupEventRelayTest(t)
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	<-scheduler.chUpdate
	prg := relayTargetProgram()
	placeholder, err := GetSelectedSchedule(prg.Sid, prg.Eid)
	if err != nil || placeholder == nil {
		t.Fatalf("missing placeholder: %v", err)
	}
	job := (&RecordingJob{ID: placeholder.ID, Validate: true, Title: placeholder.Title}).SetOffsetTimer(placeholder)
	activeTasks := &TaskArray{Job: map[uint64]*RecordingJob{job.ID: job}}
	if _, err := dev.UpdatePFRecordings(prg, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	got, err := tvprogram.Get(0, prg.Sid, prg.Eid)
	if err != nil || len(got) != 1 {
		t.Fatalf("PF not stored: %v, %v", got, err)
	}
	if len(scheduler.chUpdate) == 0 {
		t.Fatal("first PF is stored but scheduler receives no update; placeholder timers remain unchanged")
	}
	all, err := scheduler.GetScheduleAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.EpgFollow(all); err != nil {
		t.Fatal(err)
	}
	rs, err := GetSelectedSchedule(prg.Sid, prg.Eid)
	if err != nil || rs == nil {
		t.Fatalf("missing relay: %v", err)
	}
	if rs.Title != prg.Title || rs.StartTime != prg.Start || rs.EndTime != prg.End {
		t.Fatalf("placeholder not resolved: %+v", rs)
	}
	activeTasks.Modify(rs)
	if job.RecordStopTime.Unix() != prg.End || job.Title != prg.Title {
		t.Fatal("recording still uses placeholder deadline")
	}
}

func TestEventRelayOtherNetwork(t *testing.T) {
	evr := setupEventRelayTest(t)
	evr.GroupType, evr.Onid, evr.RelayOnid = 4, 0x7e80, 4
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.chUpdate) != 1 {
		t.Fatalf("expected one cross-network relay, got %d", len(scheduler.chUpdate))
	}
	got, err := GetSelectedSchedule(uint(TuningSpaceBS)<<16|evr.RelaySid, evr.RelayEid)
	if err != nil || got == nil {
		t.Fatalf("missing relay: %v", err)
	}
	if got.Sid != uint(TuningSpaceBS)<<16|evr.RelaySid || got.Eid != evr.RelayEid {
		t.Fatalf("incorrect cross-network target: %+v", got)
	}
}

func TestEventRelayWithoutSourceReservation(t *testing.T) {
	evr := setupEventRelayTest(t)
	evr.Eid++
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.chUpdate) != 0 {
		t.Fatal("relay created without a source reservation")
	}
}

func TestEventRelayCurrentPFUpdateSurvivesNewNextPF(t *testing.T) {
	setupEventRelayTest(t)
	prg := relayTargetProgram()
	if err := tvprogram.Insert(0, prg); err != nil {
		t.Fatal(err)
	}
	next := *prg
	next.Eid++
	next.Start, next.End = prg.End, prg.End+1800
	dev := &RockDevice{}
	if _, err := dev.UpdatePFRecordings(prg, &next); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.chUpdate) == 0 {
		t.Fatal("current PF requests update, but newly inserted next PF overwrites update flag with false")
	}
}

func TestEventRelayCurrentPFUpdateSurvivesEmptyNextPF(t *testing.T) {
	setupEventRelayTest(t)
	prg := relayTargetProgram()
	if err := tvprogram.Insert(0, prg); err != nil {
		t.Fatal(err)
	}
	dev := &RockDevice{}
	if _, err := dev.UpdatePFRecordings(prg, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.chUpdate) != 1 {
		t.Fatal("empty next PF discarded current update")
	}
}

func TestEventRelayConcurrentDevices(t *testing.T) {
	evr := setupEventRelayTest(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := (&RockDevice{}).UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	all, err := scheduler.GetScheduleAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want source and one relay, got %d schedules", len(all))
	}
	if len(scheduler.chUpdate) != 1 {
		t.Fatal("duplicate scheduler notifications")
	}
}

func TestEventRelayLateNotification(t *testing.T) {
	evr := setupEventRelayTest(t)
	source, err := GetSelectedSchedule(uint(TuningSpaceTS)<<16|evr.Sid, evr.Eid)
	if err != nil {
		t.Fatal(err)
	}
	source.StartTime, source.EndTime = Now().Unix()-3600, Now().Unix()-60
	if err := scheduler.Update(0, source); err != nil {
		t.Fatal(err)
	}
	before := Now().Unix()
	if err := (&RockDevice{}).UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	rs, err := GetSelectedSchedule(uint(TuningSpaceTS)<<16|evr.RelaySid, evr.RelayEid)
	if err != nil || rs == nil {
		t.Fatalf("missing relay: %v", err)
	}
	if rs.StartTime < before || rs.EndTime < before+600 {
		t.Fatalf("late relay already expired: %+v", rs)
	}
}

func TestEventRelayEITNotifiesScheduler(t *testing.T) {
	evr := setupEventRelayTest(t)
	prg := relayTargetProgram()
	oldCache := station.Cache
	station.Cache = map[uint]*TvStation{prg.Sid: {Sid: prg.Sid}}
	t.Cleanup(func() { station.Cache = oldCache })
	dev := &RockDevice{CurrentService: prg.Sid}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	<-scheduler.chUpdate
	dev.UpdateTVProgram([]TvProgram{*prg})
	if len(scheduler.chUpdate) != 1 {
		t.Fatal("EIT did not notify scheduler")
	}
	<-scheduler.chUpdate
	dev.UpdateTVProgram([]TvProgram{*prg})
	if len(scheduler.chUpdate) != 0 {
		t.Fatal("unchanged EIT notified scheduler again")
	}
}

func TestEventRelayInsertFailureCanRetry(t *testing.T) {
	evr := setupEventRelayTest(t)
	db := scheduler.db.GetData(0)
	if _, err := db.Exec(`CREATE TRIGGER fail_relay BEFORE INSERT ON schedule BEGIN SELECT RAISE(FAIL, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err == nil {
		t.Fatal("insert failure was hidden")
	}
	if len(scheduler.chUpdate) != 0 {
		t.Fatal("failed insert notified scheduler")
	}
	if _, err := db.Exec(`DROP TRIGGER fail_relay`); err != nil {
		t.Fatal(err)
	}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	got, err := GetSelectedSchedule(uint(TuningSpaceTS)<<16|evr.RelaySid, evr.RelayEid)
	if err != nil || got == nil {
		t.Fatalf("retry did not create relay: %v", err)
	}
}

func TestEventRelayUnknownPFTimesKeepPlaceholder(t *testing.T) {
	evr := setupEventRelayTest(t)
	dev := &RockDevice{}
	if err := dev.UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
		t.Fatal(err)
	}
	<-scheduler.chUpdate
	prg := relayTargetProgram()
	prg.End = -1
	if _, err := dev.UpdatePFRecordings(prg, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	got, err := tvprogram.Get(0, prg.Sid, prg.Eid)
	if err != nil || len(got) != 0 {
		t.Fatalf("invalid PF persisted: %+v, %v", got, err)
	}
	if dev.CurrentProgram != prg {
		t.Fatal("unknown times not available to device")
	}
	resolved := *prg
	resolved.End = resolved.Start + 3600
	if _, err := dev.UpdatePFRecordings(&resolved, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.chUpdate) != 1 {
		t.Fatal("resolved PF did not notify scheduler")
	}
}
