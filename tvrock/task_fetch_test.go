package tvrock

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"
)

func TestFetchProgramTargets(t *testing.T) {
	stations := map[uint]*TvStation{
		131233: {Sid: 131233, Tsp: uint(TuningSpaceBS), No: 2, FetchProgram: true, Name: "BS星の港"},
		131308: {Sid: 131308, Tsp: uint(TuningSpaceBS), No: 3, FetchProgram: true, Name: "BS雲の庭"},
		131173: {Sid: 131173, Tsp: uint(TuningSpaceBS), No: 1, FetchProgram: false},
		66560:  {Sid: 66560, Tsp: uint(TuningSpaceTS), No: 1, FetchProgram: true},
	}
	targets := fetchProgramTargets(stations, TuningSpaceBS)
	if len(targets) != 2 || targets[0].Name != "BS星の港" || targets[1].Name != "BS雲の庭" {
		t.Fatalf("unexpected BS targets: %+v", targets)
	}
	// 同じ表示順でもmapの走査順で起動局が変わらない。
	stations[131233].No = 3
	for range 20 {
		targets = fetchProgramTargets(stations, TuningSpaceBS)
		if targets[0].Sid != 131233 || targets[1].Sid != 131308 {
			t.Fatal("target order is unstable")
		}
	}
}

func TestFetchProgramStartupWaitsForTuningNotification(t *testing.T) {
	dev := &RockDevice{ID: "fetch-startup", State: StateReady}
	s := &Slot{Device: map[string]*RockDevice{dev.ID: dev}}
	// 接続完了や、SID未取得のBS通知だけで選局要求を開始しない。
	for _, sid := range []uint{0, uint(TuningSpaceBS) << 16} {
		dev.CurrentService = sid
		if s.isFetchProgramTuningReady(dev.ID) {
			t.Fatalf("startup accepted incomplete tuning notification: %d", sid)
		}
	}
	dev.CurrentService = 66560
	if !s.isFetchProgramTuningReady(dev.ID) {
		t.Fatal("startup did not accept the initial tuned service")
	}
	dev.State = StateOFF
	if s.isFetchProgramTuningReady(dev.ID) || s.isFetchProgramTuningReady("missing") {
		t.Fatal("startup accepted a disconnected or missing device")
	}
}

func TestFetchProgramWaitsForEachSelectedService(t *testing.T) {
	for _, services := range [][]uint{{131233, 131308}, {66560, 66568, 66608, 66576, 66584, 66592, 66616}} {
		t.Run(fmt.Sprint(services), func(t *testing.T) {
			var logs bytes.Buffer
			oldWriter := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(oldWriter) })
			oldDevices, oldCurrent, oldStations := slot.Device, scheduler.CurrentRecording, station.Cache
			t.Cleanup(func() {
				slot.Device, scheduler.CurrentRecording, station.Cache = oldDevices, oldCurrent, oldStations
			})
			queue := make(chan RequestMessage, 1)
			dev := &RockDevice{ID: "fetch-test", State: StateReady, Com: queue, requestDone: make(chan struct{}),
				DeviceInfo: DeviceInfo{Events: 123}}
			slot.Device = map[string]*RockDevice{dev.ID: dev}
			station.Cache = map[uint]*TvStation{}
			for _, sid := range services {
				station.Cache[sid] = &TvStation{Sid: sid, Name: fmt.Sprintf("test station %d", sid)}
			}
			job := &RecordingJob{ID: 900002, DeviceID: dev.ID}
			scheduler.CurrentRecording = map[string]*RecordingSchedule{dev.ID: {ID: job.ID}}
			now := time.Unix(1000, 0)
			for _, sid := range services {
				job.Sid = sid
				wait := newFetchProgramStationWait(30)
				// 選局待ち中の時間を、前の局のイベント数の安定時間に含めない。
				for range 4 {
					if reason := job.pollFetchProgramStation(now, &wait); reason != "" {
						t.Fatalf("service %d skipped without tuning: %s", sid, reason)
					}
					request := <-queue
					if request.Command != "StreamChange" || request.Message != fmt.Sprint(sid) {
						t.Fatalf("wrong tuning request: %+v", request)
					}
					now = now.Add(5 * time.Second)
				}
				dev.CurrentService = sid
				if reason := job.pollFetchProgramStation(now, &wait); reason != "" {
					t.Fatal("finished immediately after tuning")
				}
				if reason := job.pollFetchProgramStation(now.Add(29*time.Second), &wait); reason != "" {
					t.Fatal("quiet timeout started before tuning")
				}
				if reason := job.pollFetchProgramStation(now.Add(30*time.Second), &wait); reason != fetchProgramStationDone {
					t.Fatalf("did not finish tuned service successfully: %s", reason)
				}
				if len(queue) != 0 {
					t.Fatal("retuned an already selected service")
				}
				for _, action := range []string{"選局要求", "選局確認"} {
					want := fmt.Sprintf("番組情報取得%s id:%d device:%s sid:%d 局名:test station %d", action, job.ID, dev.ID, sid, sid)
					if strings.Count(logs.String(), want) != 1 {
						t.Fatalf("missing or duplicate tuning log: %s\n%s", want, logs.String())
					}
				}
				now = now.Add(time.Minute)
			}
		})
	}
}

func TestFetchProgramReportsNoEventsAndTuningFailure(t *testing.T) {
	for _, ready := range []bool{false, true} {
		wait := newFetchProgramStationWait(30)
		now := time.Unix(1000, 0)
		if result := wait.observe(now, ready, 0); result != fetchProgramWaiting {
			t.Fatal("failed without waiting for the timeout")
		}
		if result := wait.observe(now.Add(29*time.Second), ready, 0); result != fetchProgramWaiting {
			t.Fatal("failed before the timeout")
		}
		want := fetchProgramNoEvents
		if !ready {
			want = fetchProgramTuningFailed
		}
		if result := wait.observe(now.Add(30*time.Second), ready, 0); result != want {
			t.Fatalf("ready=%t: result=%s, want %s", ready, result, want)
		}
	}
}

func TestFetchProgramWaitResetsOnEventChangeAndDisconnection(t *testing.T) {
	wait := newFetchProgramStationWait(30)
	base := time.Unix(1000, 0)
	steps := []struct {
		sec    int
		ready  bool
		events uint
		done   bool
	}{
		{0, true, 0, false},
		{20, true, 10, false},
		{40, true, 5, false},
		{60, false, 5, false},
		{120, true, 5, false},
		{149, true, 5, false},
		{150, true, 5, true},
	}
	for _, step := range steps {
		if done := wait.observe(base.Add(time.Duration(step.sec)*time.Second), step.ready, step.events) != ""; done != step.done {
			t.Fatalf("at %ds: done=%t, want %t", step.sec, done, step.done)
		}
	}
}
