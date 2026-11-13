package tvrock

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEventRelayExecutionWithDeviceAssignmentProtection(t *testing.T) {
	for _, crossNetwork := range []bool{false, true} {
		for _, mode := range []string{"known", "PF-after-boot", "PF-after-record-start", "EIT-after-record-start", "unknown-PF-then-resolved"} {
			t.Run(fmt.Sprintf("cross-network=%v/%s", crossNetwork, mode), func(t *testing.T) {
				evr := setupEventRelayTest(t)
				oldConfig, oldMargin := configure.RecordingDevice, configure.IsAfterWithMargin
				oldDevices, oldStations := slot.Device, station.Cache
				oldCurrent, oldAssignments := scheduler.CurrentRecording, scheduler.deviceAssignments
				oldTimers := dbdInterval.dbd
				dbdInterval.dbd = map[string]*time.Timer{}
				t.Cleanup(func() {
					dbdInterval.Lock()
					for _, timer := range dbdInterval.dbd {
						timer.Stop()
					}
					dbdInterval.dbd = oldTimers
					dbdInterval.Unlock()
					configure.RecordingDevice, configure.IsAfterWithMargin = oldConfig, oldMargin
					slot.Device, station.Cache = oldDevices, oldStations
					scheduler.CurrentRecording, scheduler.deviceAssignments = oldCurrent, oldAssignments
				})
				configure.IsAfterWithMargin = true
				configure.RecordingDevice = RecordingDevice{OrderTS: []string{"REC01", "REC02"}, OrderBS: []string{"REC03"}}
				fromSid := uint(TuningSpaceTS)<<16 | evr.Sid
				toSid, wantDevice := uint(TuningSpaceTS)<<16|evr.RelaySid, "REC02"
				if crossNetwork {
					evr.GroupType, evr.Onid, evr.RelayOnid = 4, 0x7e80, 4
					toSid, wantDevice = uint(TuningSpaceBS)<<16|evr.RelaySid, "REC03"
				}
				now := Now().Truncate(time.Second)
				source, err := GetSelectedSchedule(fromSid, evr.Eid)
				if err != nil || source == nil {
					t.Fatalf("source=%v err=%v", source, err)
				}
				source.DeviceID, source.AutoSelect, source.EpgFollow = "REC01", false, false
				source.StartTime, source.EndTime = now.Unix()-600, now.Unix()+60
				if err := scheduler.Update(CurrentSlot, source); err != nil {
					t.Fatal(err)
				}
				scheduler.CurrentRecording = map[string]*RecordingSchedule{"REC01": source}
				scheduler.deviceAssignments = nil
				station.Cache = map[uint]*TvStation{
					fromSid: {Sid: fromSid, Tsp: uint(TuningSpaceTS)},
					toSid:   {Sid: toSid, Tsp: toSid >> 16},
				}
				slot.Device = map[string]*RockDevice{}
				for _, id := range []string{"REC01", "REC02", "REC03"} {
					slot.Device[id] = &RockDevice{ID: id, State: StateReady, Com: make(chan RequestMessage, 16), RecordPath: "record"}
				}
				formal := &TvProgram{Sid: toSid, Eid: evr.RelayEid, Title: "正式なリレー先番組", Start: source.EndTime, End: source.EndTime + 3600}
				if mode == "known" {
					if err := tvprogram.Insert(CurrentSlot, formal); err != nil {
						t.Fatal(err)
					}
				}
				if err := slot.Device["REC01"].UpdateEventRelay(TuningSpaceTS, []PacketEventRelay{evr}); err != nil {
					t.Fatal(err)
				}
				if len(scheduler.chUpdate) != 1 {
					t.Fatal("relay did not request schedule refresh")
				}
				<-scheduler.chUpdate
				if _, err := scheduler.ScheduleUpdate(); err != nil {
					t.Fatal(err)
				}
				reservation, err := GetSelectedSchedule(toSid, evr.RelayEid)
				if err != nil || reservation == nil {
					t.Fatalf("target=%v err=%v", reservation, err)
				}
				if reservation.DeviceID != wantDevice || reservation.Sid != toSid || reservation.Eid != evr.RelayEid {
					t.Fatalf("incorrect target assignment: %+v", reservation)
				}
				if mode != "known" && (reservation.Title != "未定" || reservation.EndTime-reservation.StartTime != 600) {
					t.Fatalf("incorrect placeholder: %+v", reservation)
				}
				job := deviceAssignmentTestJob(reservation)
				sourceJob := deviceAssignmentTestJob(source)
				sourceJob.setStatus(JobRecordStart)
				ta := &TaskArray{Job: map[uint64]*RecordingJob{job.ID: job, sourceJob.ID: sourceJob}}
				dev := slot.Device[wantDevice]
				t.Cleanup(func() { scheduler.releaseDevice(job) })
				originalStop := job.RecordStopTime
				// Real startup state transitions. Ready devices and queues ensure
				// that no application, server or physical tuner is started.
				job.RunDueActions(job.BootTime, ta)
				job.RunDueActions(job.BootTime, ta)
				job.RunDueActions(job.BootTime, ta)
				select {
				case command := <-dev.Com:
					if command.ID != wantDevice || command.Command != "StreamChange" || command.Message != fmt.Sprint(toSid) {
						t.Fatalf("incorrect relay tuning: %+v", command)
					}
				default:
					t.Fatal("relay target did not receive tuning command")
				}
				dev.CurrentService = toSid // Simulate successful tuning notification.
				refreshFromQueue := func() {
					t.Helper()
					if len(scheduler.chUpdate) != 1 {
						t.Fatalf("expected one refresh request, got %d", len(scheduler.chUpdate))
					}
					<-scheduler.chUpdate
					all, err := scheduler.ScheduleUpdate()
					if err != nil {
						t.Fatal(err)
					}
					if err := ta.Update(all); err != nil {
						t.Fatal(err)
					}
					if existing, ok := ta.Get(job.ID); !ok || existing != job {
						t.Fatal("placeholder task was replaced instead of updated")
					}
				}
				if mode == "PF-after-boot" {
					if _, err := dev.UpdatePFRecordings(formal, &TvProgram{}); err != nil {
						t.Fatal(err)
					}
					refreshFromQueue()
				}
				job.RunDueActions(job.RecordStartTime, ta)
				var initialFilename string
				select {
				case command := <-dev.Com:
					if command.ID != wantDevice || command.Command != "RecordStart" {
						t.Fatalf("incorrect relay record start: %+v", command)
					}
					initialFilename = command.Message
				default:
					t.Fatal("relay recording did not start")
				}
				dev.State = StateRecording
				if mode == "known" || mode == "PF-after-boot" {
					if !strings.Contains(initialFilename, formal.Title) {
						t.Fatalf("recording did not use formal title: %s", initialFilename)
					}
				} else {
					if !strings.Contains(initialFilename, "未定") {
						t.Fatalf("placeholder recording did not start: %s", initialFilename)
					}
					if mode == "unknown-PF-then-resolved" {
						unknown := *formal
						unknown.End = -1
						if _, err := dev.UpdatePFRecordings(&unknown, &TvProgram{}); err != nil {
							t.Fatal(err)
						}
						job.RunDueActions(originalStop, ta)
						if job.getStatus() != JobRecordStart || len(dev.Com) != 0 {
							t.Fatal("unknown end PF did not keep placeholder recording running")
						}
						if len(scheduler.chUpdate) != 0 {
							t.Fatal("invalid PF timing replaced valid placeholder timers")
						}
					}
					if mode == "EIT-after-record-start" {
						dev.UpdateTVProgram([]TvProgram{*formal})
					} else if _, err := dev.UpdatePFRecordings(formal, &TvProgram{}); err != nil {
						t.Fatal(err)
					}
					refreshFromQueue()
				}
				stored, err := scheduler.GetRecordingSchedule(CurrentSlot, job.ID)
				if err != nil || stored == nil {
					t.Fatalf("updated reservation=%v err=%v", stored, err)
				}
				if stored.DeviceID != wantDevice || stored.Title != formal.Title || stored.StartTime != formal.Start || stored.EndTime != formal.End ||
					job.DeviceID != wantDevice || job.Title != formal.Title || job.RecordStopTime.Unix() != formal.End+reservation.TailTime {
					t.Fatalf("formal metadata not applied: reservation=%+v task-title=%s device=%s stop=%s", stored, job.Title, job.DeviceID, job.RecordStopTime)
				}
				if current, ok := scheduler.GetCurrentRecording(wantDevice); !ok || current.ID != job.ID {
					t.Fatal("formal information displaced the executing relay task")
				}
				if len(dev.Com) != 0 {
					t.Fatal("formal information interrupted/restarted placeholder recording")
				}
				if mode != "known" {
					job.RunDueActions(originalStop, ta)
					if job.getStatus() != JobRecordStart || len(dev.Com) != 0 {
						t.Fatal("relay stopped at the original 10-minute placeholder deadline")
					}
				}
				job.RunDueActions(job.RecordStopTime, ta)
				select {
				case command := <-dev.Com:
					if command.ID != wantDevice || command.Command != "RecordStop" {
						t.Fatalf("incorrect relay stop: %+v", command)
					}
				default:
					t.Fatal("relay did not stop at the formal end time")
				}
				if len(slot.Device["REC01"].Com) != 0 {
					t.Fatal("relay target commands were sent to source device")
				}
			})
		}
	}
}
