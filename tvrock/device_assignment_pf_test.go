package tvrock

import (
	"testing"
	"time"
)

// Exercise the handler used by OnDevMsgPF through the real schedule refresh,
// while using an isolated DB and in-memory device queues.
func TestDeviceAssignmentPFTimingFollow(t *testing.T) {
	for _, phase := range []string{"startup-next-event", "recording-extension"} {
		t.Run(phase, func(t *testing.T) {
			setupSchedulerAssistTest(t)
			configure.RecordingDevice.Assist = false
			s := newDeviceAssignmentTestScheduler(t)
			oldDB, oldStore, oldTVP := scheduler.db, tvprogram, scheduler.tvp
			oldCurrent, oldAssignments, oldQueue := scheduler.CurrentRecording, scheduler.deviceAssignments, scheduler.chUpdate
			scheduler.db, tvprogram, scheduler.tvp = s.db, *s.tvp, s.tvp
			scheduler.CurrentRecording = map[string]*RecordingSchedule{}
			scheduler.deviceAssignments = nil
			scheduler.chUpdate = make(chan SchedulerRequest, 16)
			t.Cleanup(func() {
				scheduler.db, tvprogram, scheduler.tvp = oldDB, oldStore, oldTVP
				scheduler.CurrentRecording, scheduler.deviceAssignments, scheduler.chUpdate = oldCurrent, oldAssignments, oldQueue
			})

			now := Now().Truncate(time.Second)
			start := now.Unix() - 300
			if phase == "startup-next-event" {
				start = now.Unix() + 30
			}
			sch := deviceAssignmentTestSchedule(start)
			sch.EpgFollow, sch.TailTime = true, 10
			if phase == "recording-extension" {
				sch.EndTime = now.Unix() + 300
			}
			insertDeviceAssignmentTestSchedule(t, &scheduler, sch)
			job := deviceAssignmentTestJob(sch)
			job.Lock()
			_, err := scheduler.acquireDevice(job)
			job.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { scheduler.releaseDevice(job) })
			job.setStatus(JobBoot)
			dev := &RockDevice{ID: "REC01", State: StateReady, CurrentService: sch.Sid, Com: make(chan RequestMessage, 8)}
			slot.Device = map[string]*RockDevice{"REC01": dev}
			ta := &TaskArray{Job: map[uint64]*RecordingJob{job.ID: job}}
			originalStop := job.RecordStopTime
			pf := &TvProgram{Sid: sch.Sid, Eid: sch.Eid, Title: sch.Title, Start: sch.StartTime, End: sch.EndTime}
			if err := tvprogram.Insert(CurrentSlot, pf); err != nil {
				t.Fatal(err)
			}
			// Keep the original event in the device so this is a changed PF,
			// rather than a first-time program insertion.
			originalPF := *pf
			current, next := &TvProgram{}, pf
			if phase == "startup-next-event" {
				dev.NextProgram = &originalPF
				pf.Start += 60
				pf.End += 60
			} else {
				dev.CurrentProgram = &originalPF
				current, next = pf, &TvProgram{}
				job.setStatus(JobRecordStart)
				dev.State = StateRecording
				pf.End += 1800
			}
			changed, err := dev.UpdatePFRecordings(current, next)
			if err != nil || !changed {
				t.Fatalf("PF change=%v err=%v", changed, err)
			}
			select {
			case request := <-scheduler.chUpdate:
				if request.Com != SchdulerComUpdate {
					t.Fatalf("unexpected scheduler request: %+v", request)
				}
			default:
				t.Fatal("PF did not request a schedule refresh")
			}
			// Execute the same two steps as ExecScheduler's update branch.
			updated, err := scheduler.ScheduleUpdate()
			if err != nil {
				t.Fatal(err)
			}
			if err := ta.Update(updated); err != nil {
				t.Fatal(err)
			}
			stored, err := scheduler.GetRecordingSchedule(CurrentSlot, job.ID)
			if err != nil || stored == nil {
				t.Fatalf("updated schedule=%v err=%v", stored, err)
			}
			if stored.StartTime != pf.Start || stored.EndTime != pf.End || stored.DeviceID != "REC01" {
				t.Fatalf("PF timing/device not preserved in DB: %+v", stored)
			}
			if job.DeviceID != "REC01" || job.BootTime.Unix() != pf.Start-sch.IdleTime ||
				job.StreamChangeTime.Unix() != pf.Start-sch.IdleTime || job.RecordStartTime.Unix() != pf.Start-sch.ReadyTime ||
				job.RecordStopTime.Unix() != pf.End+sch.TailTime || job.ExitTime.Unix() != pf.End+sch.TailTime {
				t.Fatalf("PF timers not reflected in task: device=%s boot=%s record=%s stop=%s exit=%s",
					job.DeviceID, job.BootTime, job.RecordStartTime, job.RecordStopTime, job.ExitTime)
			}
			select {
			case command := <-job.Com:
				if command != JobUpdate {
					t.Fatalf("unexpected task command: %v", command)
				}
			default:
				t.Fatal("PF timer update was not delivered to the task")
			}
			if phase == "recording-extension" {
				job.RunDueActions(originalStop, ta)
				if job.getStatus() != JobRecordStart || len(dev.Com) != 0 {
					t.Fatal("recording stopped at the original deadline after PF extension")
				}
				job.RunDueActions(job.RecordStopTime, ta)
				if job.getStatus() != JobRecordStop {
					t.Fatal("recording did not stop at the extended deadline")
				}
				select {
				case command := <-dev.Com:
					if command.ID != "REC01" || command.Command != "RecordStop" {
						t.Fatalf("wrong stop command: %+v", command)
					}
				default:
					t.Fatal("missing stop command at extended deadline")
				}
			} else {
				job.RunDueActions(now, ta)
				if job.getStatus() != JobBoot || len(dev.Com) != 0 {
					t.Fatal("startup actions ignored the corrected start time")
				}
			}
		})
	}
}
