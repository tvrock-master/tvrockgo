package tvrock

import (
	"testing"
	"time"
)

func setupSchedulerAssistTest(t *testing.T) {
	t.Helper()
	oldRecording, oldMargin := configure.RecordingDevice, configure.IsAfterWithMargin
	oldDevices, oldStations := slot.Device, station.Cache
	t.Cleanup(func() {
		configure.RecordingDevice, configure.IsAfterWithMargin = oldRecording, oldMargin
		slot.Device, station.Cache = oldDevices, oldStations
	})
	configure.RecordingDevice = RecordingDevice{
		Assist: true, OrderTS: []string{"REC01", "REC02"},
		DeviceAssistance: map[string]string{"REC01": "SUB01", "REC02": "SUB02"},
	}
	configure.IsAfterWithMargin = true
	slot.Device = map[string]*RockDevice{
		"REC01": {ID: "REC01"}, "SUB01": {ID: "SUB01"},
		"REC02": {ID: "REC02"}, "SUB02": {ID: "SUB02"},
	}
	station.Cache = map[uint]*TvStation{1: {Sid: 1, Tsp: uint(TuningSpaceTS)}}
}

func assistTestSchedule(id uint64, device string, sid uint, start, end int64) *RecordingSchedule {
	return &RecordingSchedule{ID: id, DeviceID: device, Sid: sid, Validate: true, StartTime: start, EndTime: end}
}

func TestSelectAssistDevice(t *testing.T) {
	tests := []struct {
		name         string
		reservations []*RecordingSchedule
		want         string
	}{
		{"same service on recorder", []*RecordingSchedule{assistTestSchedule(1, "REC01", 1, 1000, 2000)}, "SUB01"},
		{"same service on assist", []*RecordingSchedule{assistTestSchedule(1, "SUB01", 1, 1000, 2000)}, "REC01"},
		{"different service", []*RecordingSchedule{assistTestSchedule(1, "REC01", 2, 1000, 2000)}, ""},
		{"both occupied", []*RecordingSchedule{assistTestSchedule(1, "REC01", 1, 1000, 2000), assistTestSchedule(2, "SUB01", 1, 1000, 2000)}, ""},
		{"both occupied reversed", []*RecordingSchedule{assistTestSchedule(2, "SUB01", 1, 1000, 2000), assistTestSchedule(1, "REC01", 1, 1000, 2000)}, ""},
		{"mixed services", []*RecordingSchedule{assistTestSchedule(1, "REC01", 1, 1000, 2000), assistTestSchedule(2, "REC01", 2, 1000, 2000)}, ""},
		{"mixed services reversed", []*RecordingSchedule{assistTestSchedule(2, "REC01", 2, 1000, 2000), assistTestSchedule(1, "REC01", 1, 1000, 2000)}, ""},
		{"unrelated recorder", []*RecordingSchedule{assistTestSchedule(1, "REC02", 1, 1000, 2000)}, ""},
		{"nonoverlapping assist reservation", []*RecordingSchedule{assistTestSchedule(1, "REC01", 1, 1000, 2000), assistTestSchedule(2, "SUB01", 2, 2000, 3000)}, "SUB01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupSchedulerAssistTest(t)
			sch := assistTestSchedule(3, "REC01", 1, 1000, 2000)
			if got := (&Scheduler{}).SelectAssistDevice([]string{"REC01"}, sch, tt.reservations, 0); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if sch.DeviceID != "REC01" {
				t.Fatal("selection mutated the schedule")
			}
		})
	}
}

func TestSelectAssistDeviceMarginsAndInactiveReservations(t *testing.T) {
	setupSchedulerAssistTest(t)
	p := &Scheduler{}
	sch := assistTestSchedule(3, "REC01", 1, 2000, 3000)
	sch.IdleTime = 60
	rec := assistTestSchedule(1, "REC01", 1, 1000, 2000)
	for _, margin := range []bool{true, false} {
		configure.IsAfterWithMargin = margin
		want := ""
		if margin {
			want = "SUB01"
		}
		if got := p.SelectAssistDevice([]string{"REC01"}, sch, []*RecordingSchedule{rec}, 0); got != want {
			t.Fatalf("margin=%v: got %q, want %q", margin, got, want)
		}
	}
	configure.IsAfterWithMargin = true
	for _, kind := range []string{"disabled", "complete", "ended", "self"} {
		v := *rec
		now := int64(0)
		switch kind {
		case "disabled":
			v.Validate = false
		case "complete":
			v.Complete = true
		case "ended":
			now = 2001
		case "self":
			v.ID = sch.ID
		}
		if got := p.SelectAssistDevice([]string{"REC01"}, sch, []*RecordingSchedule{&v}, now); got != "" {
			t.Fatalf("%s reservation selected %q", kind, got)
		}
	}
}

func TestSelectAssistDeviceRejectsInvalidPairs(t *testing.T) {
	for _, kind := range []string{"missing mapping", "missing device", "self mapping", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			setupSchedulerAssistTest(t)
			switch kind {
			case "missing mapping":
				delete(configure.RecordingDevice.DeviceAssistance, "REC01")
			case "missing device":
				delete(slot.Device, "SUB01")
			case "self mapping":
				configure.RecordingDevice.DeviceAssistance["REC01"] = "REC01"
			case "disabled":
				configure.RecordingDevice.Assist = false
			}
			sch := assistTestSchedule(2, "REC01", 1, 1000, 2000)
			if got := (&Scheduler{}).SelectAssistDevice([]string{"REC01"}, sch, []*RecordingSchedule{assistTestSchedule(1, "REC01", 1, 1000, 2000)}, 0); got != "" {
				t.Fatalf("got %q, want no assist", got)
			}
		})
	}
}

func TestDistributeOrderAssist(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			setupSchedulerAssistTest(t)
			configure.RecordingDevice.Assist = enabled
			start := time.Now().Add(time.Hour).Unix()
			fixed := assistTestSchedule(1, "REC01", 1, start, start+1000)
			sch := assistTestSchedule(2, "", 1, start, start+1000)
			sch.AutoSelect = true
			(&Scheduler{}).DistributeOrder([]*RecordingSchedule{fixed, sch}, start-100)
			want := "REC02"
			if enabled {
				want = "SUB01"
			}
			if sch.DeviceID != want {
				t.Fatalf("got %q, want %q", sch.DeviceID, want)
			}
		})
	}
	t.Run("do not borrow another pair", func(t *testing.T) {
		setupSchedulerAssistTest(t)
		delete(configure.RecordingDevice.DeviceAssistance, "REC01")
		start := time.Now().Add(time.Hour).Unix()
		sch := assistTestSchedule(3, "", 1, start, start+1000)
		sch.AutoSelect = true
		(&Scheduler{}).DistributeOrder([]*RecordingSchedule{
			assistTestSchedule(1, "REC01", 1, start, start+100),
			assistTestSchedule(2, "REC02", 2, start, start+1000), sch,
		}, start-100)
		if sch.DeviceID != "REC01" {
			t.Fatalf("got %q, want fallback REC01", sch.DeviceID)
		}
	})
}

func TestDistributeOrderAssistAlternatesContinuousReservations(t *testing.T) {
	setupSchedulerAssistTest(t)
	start := time.Now().Add(time.Hour).Unix()
	var rs []*RecordingSchedule
	for i := 0; i < 4; i++ {
		s := assistTestSchedule(uint64(i+1), "", 1, start+int64(i)*600, start+int64(i+1)*600)
		s.AutoSelect = true
		s.IdleTime, s.TailTime = 30, 10
		rs = append(rs, s)
	}
	p := &Scheduler{}
	// 余裕時間で隣接予約だけが重複する。REC02が空いていても同じペアを交互に使う。
	for pass := 0; pass < 2; pass++ {
		p.DistributeOrder(rs, start-100)
		for i, want := range []string{"REC01", "SUB01", "REC01", "SUB01"} {
			if rs[i].DeviceID != want {
				t.Fatalf("pass %d reservation %d: got %q, want %q", pass, i+1, rs[i].DeviceID, want)
			}
		}
	}
}

func TestSelectMinimalOverlapAssistAndLegacy(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, duration := range []int64{1000, 20000} {
			setupSchedulerAssistTest(t)
			configure.RecordingDevice.Assist = enabled
			sch := assistTestSchedule(3, "", 1, 1000, 1000+duration)
			rs := []*RecordingSchedule{assistTestSchedule(1, "REC01", 2, 1000, 1000+duration/2), assistTestSchedule(2, "REC02", 2, 1000, 1000+duration)}
			if err := (&Scheduler{}).SelectMinimalOverlap([]string{"REC01", "REC02"}, sch, rs, 0); err != nil {
				t.Fatal(err)
			}
			want := "REC01"
			// Assist=falseでは既存の選択動作を維持する。
			if !enabled {
				want = "REC02"
				if duration == 20000 {
					want = ""
				}
			}
			if sch.DeviceID != want {
				t.Fatalf("assist=%v duration=%d: got %q, want %q", enabled, duration, sch.DeviceID, want)
			}
		}
	}
}

func TestCollisionLengthAssistReverseAndLegacy(t *testing.T) {
	setupSchedulerAssistTest(t)
	p := &Scheduler{}
	rec, sub := assistTestSchedule(1, "REC01", 1, 1000, 2000), assistTestSchedule(2, "SUB01", 2, 1000, 2000)
	for _, enabled := range []bool{true, false} {
		configure.RecordingDevice.Assist = enabled
		want := int64(0)
		if enabled {
			want = 1000
		}
		if got := p.CollisionLength(sub, rec, 0); got != want {
			t.Fatalf("assist=%v reverse: got %d, want %d", enabled, got, want)
		}
		if got := p.CollisionLength(rec, sub, 0); got != 1000 {
			t.Fatalf("forward: got %d, want 1000", got)
		}
	}
}
