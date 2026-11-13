package tvrock

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestScheduledWatchObservesTasksWithoutChangingThem(t *testing.T) {
	_, d := watchTestManager(t)
	previous := tasks.Job
	defer func() { tasks.Job = previous }()
	job := &RecordingJob{ID: 42, DeviceID: d.ID, Sid: d.CurrentService, Validate: true, BootType: DeviceBootTypeWatchOnly}
	tasks.Job = map[uint64]*RecordingJob{job.ID: job}
	scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: job.ID}
	for _, boot := range []uint{DeviceBootTypeWatchOnly, DeviceBootTypeWatch, DeviceBootTypeOnesegWatchOnly, DeviceBootTypeOnesegWatch, DeviceBootTypeRecord, DeviceBootTypeOnesegRecord, DeviceBootTypeFetchProgramTS} {
		job.BootType = boot
		job.setStatus(JobStreamChange)
		got := scheduledWatchJobs()
		if watchBootType(boot) {
			if len(got) != 1 || !got[0].ready {
				t.Fatalf("watch type %d: %+v", boot, got)
			}
		} else if len(got) != 0 {
			t.Fatalf("record/EPG type %d launched a viewer", boot)
		}
		if job.getStatus() != JobStreamChange || d.State != StateReady {
			t.Fatal("observer modified task/device")
		}
	}
	job.BootType = DeviceBootTypeWatchOnly
	d.CurrentService++
	if got := scheduledWatchJobs(); len(got) != 1 || got[0].ready {
		t.Fatal("announced before tuning", got)
	}
	d.CurrentService = job.Sid
	job.Lock()
	got := scheduledWatchJobs()
	job.Unlock()
	if len(got) != 1 || !got[0].preserve {
		t.Fatal("locked job should be preserved without waiting", got)
	}
	for _, status := range []ScheduleJobCom{JobNone, JobRecordStop, JobExit, JobForceExit} {
		job.setStatus(status)
		if len(scheduledWatchJobs()) != 0 {
			t.Fatal("inactive task announced", status)
		}
	}
	job.setStatus(JobRecordStart)
	job.Validate = false
	if len(scheduledWatchJobs()) != 0 {
		t.Fatal("disabled task announced")
	}
}

func TestScheduledWatchBroadcastsToAllAndStopsOnlyOwnedViews(t *testing.T) {
	m, d := watchTestManager(t)
	old := deviceController.Handler
	defer func() { deviceController.Handler = old }()
	deviceController.Handler = map[string]*DevConQueue{}
	for _, uid := range []string{"pc1", "pc2"} {
		deviceController.Handler[uid] = &DevConQueue{Com: make(chan string, 20), Done: make(chan struct{})}
	}
	jobs := []scheduledWatchJob{{id: 1, parent: d.RegistryCode, ready: true, stopOnEnd: true}}
	m.updateScheduled(jobs)
	request := m.scheduled[1].Request
	for _, uid := range []string{"pc1", "pc2"} {
		raw := <-deviceController.Handler[uid].Com
		var notice scheduledWatch
		if json.Unmarshal([]byte(strings.TrimPrefix(raw, "wat:")), &notice) != nil || notice.Request != request {
			t.Fatal(raw)
		}
	}
	m.updateScheduled(jobs)
	if m.scheduled[1].Request != request {
		t.Fatal("poll replaced schedule identity")
	}
	for i, uid := range []string{"pc1", "pc2"} {
		watchTestCommand(m, uid, []string{"192.0.2.2", "192.0.2.3"}[i], WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234, Schedule: request})
	}
	if len(m.sessions) != 2 {
		t.Fatal("not all PCs acquired independent views", len(m.sessions))
	}
	// A manual viewer is never claimed or stopped by a reservation.
	watchTestCommand(m, "manual", "192.0.2.4", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	m.updateScheduled(nil)
	for _, s := range m.sessions {
		if s.Stopping != (s.Schedule == request) {
			t.Fatal("schedule ended wrong view", s.Owner)
		}
	}
	if d.State != StateReady {
		t.Fatal("schedule notification changed recorder state")
	}
}
func TestScheduledWatchAdjacentReservationsAndManualClose(t *testing.T) {
	m, d := watchTestManager(t)
	m.updateScheduled([]scheduledWatchJob{{id: 1, parent: d.RegistryCode, ready: true, stopOnEnd: true}})
	old := m.scheduled[1].Request
	watchTestCommand(m, "pc", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234, Schedule: old})
	s := m.sessions[m.byParent[d.RegistryCode]]
	m.updateScheduled([]scheduledWatchJob{{id: 2, parent: d.RegistryCode, ready: false, stopOnEnd: false}})
	if s.Stopping || s.Schedule != m.scheduled[2].Request {
		t.Fatal("adjacent reservation interrupted view")
	}
	watchTestCommand(m, "pc", "192.0.2.2", WatchCommand{Action: "adopt", Session: s.Token, Schedule: old})
	if s.Schedule == old {
		t.Fatal("stale reservation reclaimed view")
	}
	m.updateScheduled(nil)
	if s.Stopping {
		t.Fatal("keep-on exit mode stopped viewer")
	}
	watchTestCommand(m, "pc", "192.0.2.2", WatchCommand{Action: "stop", Session: s.Token})
	if !s.Stopping {
		t.Fatal("manual power off failed")
	}
}
func TestRemoteWatchMultiplePeersIsolateStatusAndStop(t *testing.T) {
	m, d := watchTestManager(t)
	for i, uid := range []string{"pc1", "pc2"} {
		watchTestCommand(m, uid, []string{"192.0.2.2", "192.0.2.3"}[i], WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	}
	if len(m.sessions) != 2 {
		t.Fatal("fanout missing")
	}
	var first, second *watchSession
	for _, s := range m.sessions {
		if s.Owner == "pc1" {
			first = s
		} else {
			second = s
		}
	}
	now := time.Now()
	for _, s := range []*watchSession{first, second} {
		b, _ := json.Marshal(SourceStreamStatus{Request: s.Token, Session: s.Token, Supported: true, OptionsSupported: true, MultiSupported: true, Success: true, Enabled: true, Address: s.Address, Port: s.Port, Flags: 7})
		m.status(d.RegistryCode, b)
		s.ReceiverSeen = now
		s.ReceiverReady = true
		s.ReceiverQueue = make(chan RequestMessage, 10)
	}
	if first.Source.Address == second.Source.Address {
		t.Fatal("source status crossed peers")
	}
	watchTestCommand(m, "pc1", first.Address, WatchCommand{Action: "stop", Session: first.Token})
	first.StopAck = true
	m.tick(now)
	if len(m.sessions) != 1 || m.byParent[d.RegistryCode] != second.Token || second.Stopping {
		t.Fatal("one stop affected other viewer")
	}
	if m.sendRetirement(d.RegistryCode) {
		t.Fatal("active source was retired")
	}
	watchTestCommand(m, "pc1", first.Address, WatchCommand{Action: "stop", Session: second.Token})
	if second.Stopping {
		t.Fatal("other peer stopped a session")
	}
	// Retired replies must not replace the surviving view's status.
	b, _ := json.Marshal(SourceStreamStatus{Request: first.Token, Session: first.Token})
	m.status(d.RegistryCode, b)
	if second.Source.Request != second.Token {
		t.Fatal("stale status affected survivor")
	}
}

func TestScheduledWatchReservationFlagsAndBusyCollection(t *testing.T) {
	for _, c := range []struct{ watch, record, want bool }{{true, false, true}, {false, false, true}, {false, true, false}, {true, true, false}} {
		schedule := &RecordingSchedule{WatchOnly: c.watch, RecordOnly: c.record}
		if watchBootType(GetBootType(schedule)) != c.want {
			t.Fatalf("flags watch=%v record=%v", c.watch, c.record)
		}
		schedule.Sid = uint(TuningSpaceOS)<<16 | 1
		if watchBootType(GetBootType(schedule)) != c.want {
			t.Fatal("one-seg reservation mode differs")
		}
	}
	tasks.Lock()
	result := scheduledWatchJobs()
	tasks.Unlock()
	if result != nil {
		t.Fatal("busy task collection should defer monitoring")
	}
}
