package tvrock

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newDeviceAssignmentTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	db := newSearchTestDB(t, false)
	if err := CreateTable(db); err != nil {
		t.Fatal(err)
	}
	store := &RockDB{Data: []*sql.DB{db}, Program: []*sql.DB{db}}
	return &Scheduler{db: store, tvp: &TvProgramStore{db: store}, CurrentRecording: map[string]*RecordingSchedule{}}
}

func deviceAssignmentTestSchedule(start int64) *RecordingSchedule {
	return &RecordingSchedule{DeviceID: "REC01", Title: "hotel", Sid: 1, Eid: 1,
		Validate: true, AutoSelect: true, RecordOnly: true,
		StartTime: start, EndTime: start + 1800, IdleTime: 60, ReadyTime: 30}
}

func deviceAssignmentTestJob(sch *RecordingSchedule) *RecordingJob {
	job := &RecordingJob{ID: sch.ID, DeviceID: sch.DeviceID, Sid: sch.Sid, Eid: sch.Eid,
		Title: sch.Title, Validate: sch.Validate, BootType: GetBootType(sch), Com: make(chan ScheduleJobCom, 16)}
	job.SetOffsetTimer(sch)
	return job
}

func insertDeviceAssignmentTestSchedule(t *testing.T, s *Scheduler, sch *RecordingSchedule) {
	t.Helper()
	if _, err := s.Insert(CurrentSlot, sch); err != nil {
		t.Fatal(err)
	}
	if err := s.db.GetData(CurrentSlot).QueryRow(`SELECT MAX(id) FROM schedule`).Scan(&sch.ID); err != nil {
		t.Fatal(err)
	}
}

func assertDeviceAssignment(t *testing.T, s *Scheduler, id uint64, want string) {
	t.Helper()
	sch, err := s.GetRecordingSchedule(CurrentSlot, id)
	if err != nil || sch == nil {
		t.Fatalf("get schedule: %v, %v", sch, err)
	}
	if sch.DeviceID != want {
		t.Fatalf("stored device=%s, want %s", sch.DeviceID, want)
	}
}

func TestDeviceAssignmentUpdatesPendingTaskAfterWatchingReservation(t *testing.T) {
	setupSchedulerAssistTest(t)
	configure.RecordingDevice.Assist = false
	s := newDeviceAssignmentTestScheduler(t)
	start := Now().Add(2 * time.Hour).Unix()
	hotel := deviceAssignmentTestSchedule(start)
	insertDeviceAssignmentTestSchedule(t, s, hotel)
	job := deviceAssignmentTestJob(hotel)
	ta := &TaskArray{Job: map[uint64]*RecordingJob{hotel.ID: job}}

	// The recording task already exists before the overlapping watching reserve.
	watch := deviceAssignmentTestSchedule(start - 900)
	watch.Title, watch.AutoSelect, watch.RecordOnly, watch.WatchOnly = "watch", false, false, true
	watch.Eid = 2
	insertDeviceAssignmentTestSchedule(t, s, watch)
	if _, err := s.ScheduleUpdateAndOrder([]*RecordingSchedule{watch, hotel}); err != nil {
		t.Fatal(err)
	}
	if hotel.DeviceID != "REC02" {
		t.Fatalf("recording assignment=%s, want REC02", hotel.DeviceID)
	}
	if job.CompareSchedule(hotel)&schChangeDevice == 0 {
		t.Fatal("device-only change was not detected")
	}
	ta.Modify(hotel)
	if job.DeviceID != "REC02" || job.getStatus() != JobNone {
		t.Fatalf("pending task device=%s state=%v", job.DeviceID, job.getStatus())
	}
	select {
	case command := <-job.Com:
		if command != JobUpdate {
			t.Fatalf("unexpected command: %v", command)
		}
	default:
		t.Fatal("device-only change did not notify the task")
	}
	assertDeviceAssignment(t, s, hotel.ID, "REC02")
}

func TestDeviceAssignmentDoesNotMoveStartedTasks(t *testing.T) {
	for _, status := range []ScheduleJobCom{JobBoot, JobStreamChange, JobRecordStart, JobRecordStop, JobExit, JobForceExit} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
			sch.ID = 123
			job := deviceAssignmentTestJob(sch)
			job.setStatus(status)
			ta := &TaskArray{Job: map[uint64]*RecordingJob{sch.ID: job}}
			sch.DeviceID, sch.Title, sch.EndTime = "REC02", "extended", sch.EndTime+300
			ta.Modify(sch)
			if job.DeviceID != "REC01" || job.Title != "extended" || job.RecordStopTime.Unix() != sch.EndTime {
				t.Fatalf("device=%s title=%s stop=%s", job.DeviceID, job.Title, job.RecordStopTime)
			}
		})
	}
	// An EPG postponement may return an acquired job to JobNone. It still owns
	// its original device until the task exits.
	sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
	sch.ID = 124
	job := deviceAssignmentTestJob(sch)
	job.deviceLocked = true
	ta := &TaskArray{Job: map[uint64]*RecordingJob{sch.ID: job}}
	sch.DeviceID = "REC02"
	ta.Modify(sch)
	if job.DeviceID != "REC01" {
		t.Fatal("acquired JobNone task moved")
	}
}

func TestDeviceAssignmentPreservesStartedReservationWrites(t *testing.T) {
	for _, writer := range []string{"single", "batch"} {
		for _, requested := range []string{"", "REC02"} {
			t.Run(writer+"/"+requested, func(t *testing.T) {
				s := newDeviceAssignmentTestScheduler(t)
				sch := deviceAssignmentTestSchedule(Now().Unix() - 60)
				insertDeviceAssignmentTestSchedule(t, s, sch)
				sch.DeviceID, sch.Title = requested, "updated"
				var err error
				if writer == "single" {
					err = s.Update(CurrentSlot, sch)
				} else {
					err = s.UpdateSchedules(CurrentSlot, sch)
				}
				if err != nil {
					t.Fatal(err)
				}
				if sch.DeviceID != "REC01" {
					t.Fatalf("updated object device=%s", sch.DeviceID)
				}
				assertDeviceAssignment(t, s, sch.ID, "REC01")
			})
		}
	}
}

func TestDeviceAssignmentKeywordRefreshKeepsRecordingAndWatchingDevices(t *testing.T) {
	for _, watch := range []bool{false, true} {
		for _, requested := range []string{"", "REC02"} {
			t.Run(fmt.Sprint(watch)+"/"+requested, func(t *testing.T) {
				s := newDeviceAssignmentTestScheduler(t)
				start := Now().Unix() - 60
				if _, err := s.db.GetData(CurrentSlot).Exec(`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b) VALUES (1,1,?,?,?,'','',0,0)`, start, start+1800, "hotel"); err != nil {
					t.Fatal(err)
				}
				sch := deviceAssignmentTestSchedule(start)
				sch.SearchID, sch.WatchOnly, sch.RecordOnly = 9001, watch, !watch
				insertDeviceAssignmentTestSchedule(t, s, sch)
				kr := NewKeywordReserve()
				kr.ID, kr.Title, kr.Sid, kr.DeviceID = 9001, "hotel", []uint{1}, requested
				kr.WatchOnly, kr.RecordOnly = watch, !watch
				count, err := s.addKeywordReserveSchedulesNoNotify(kr)
				if err != nil || count != 1 {
					t.Fatalf("keyword refresh count=%d err=%v", count, err)
				}
				assertDeviceAssignment(t, s, sch.ID, "REC01")
			})
		}
	}
}

func TestDeviceAssignmentAcquisitionUsesLatestReservationAndProtectsOwnership(t *testing.T) {
	s := newDeviceAssignmentTestScheduler(t)
	sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
	insertDeviceAssignmentTestSchedule(t, s, sch)
	job := deviceAssignmentTestJob(sch)
	sch.DeviceID = "REC02"
	if err := s.Update(CurrentSlot, sch); err != nil {
		t.Fatal(err)
	}
	// Start before the queued task modification has been processed.
	job.Lock()
	_, err := s.acquireDevice(job)
	job.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if job.DeviceID != "REC02" || !job.deviceLocked {
		t.Fatalf("acquired device=%s locked=%v", job.DeviceID, job.deviceLocked)
	}
	// Even with a future start time / JobNone, ownership freezes the device.
	sch.DeviceID = "REC01"
	if err := s.UpdateSchedules(CurrentSlot, sch); err != nil {
		t.Fatal(err)
	}
	assertDeviceAssignment(t, s, sch.ID, "REC02")
	configureSnapshot := configure.RecordingDevice
	t.Cleanup(func() { configure.RecordingDevice = configureSnapshot })
	configure.RecordingDevice.Assist = false
	s.DistributeOrder([]*RecordingSchedule{sch}, Now().Unix())
	if sch.DeviceID != "REC02" {
		t.Fatal("redistribution moved an acquired future reservation")
	}
	s.releaseDevice(job)
	s.DeleteCurrentRecording(job.DeviceID, job.ID)
	sch.DeviceID = "REC01"
	if err := s.Update(CurrentSlot, sch); err != nil {
		t.Fatal(err)
	}
	assertDeviceAssignment(t, s, sch.ID, "REC01")
}

func TestDeviceAssignmentAcquisitionAndUpdateAreSerialized(t *testing.T) {
	s := newDeviceAssignmentTestScheduler(t)
	for i := 0; i < 25; i++ {
		sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
		insertDeviceAssignmentTestSchedule(t, s, sch)
		job := deviceAssignmentTestJob(sch)
		sch.DeviceID = "REC02"
		var wg sync.WaitGroup
		var acquireErr, updateErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			job.Lock()
			defer job.Unlock()
			_, acquireErr = s.acquireDevice(job)
		}()
		go func() {
			defer wg.Done()
			updateErr = s.Update(CurrentSlot, sch)
		}()
		wg.Wait()
		if acquireErr != nil || updateErr != nil {
			t.Fatalf("acquire=%v update=%v", acquireErr, updateErr)
		}
		assertDeviceAssignment(t, s, sch.ID, job.DeviceID)
		if deviceID, fixed := s.fixedDevice(job.ID); !fixed || deviceID != job.DeviceID {
			t.Fatalf("ownership=%s fixed=%v task=%s", deviceID, fixed, job.DeviceID)
		}
		s.DeleteCurrentRecording(job.DeviceID, job.ID)
		s.releaseDevice(job)
	}
}

func TestDeviceAssignmentRecordingDoesNotTakeWatchingDevice(t *testing.T) {
	s := newDeviceAssignmentTestScheduler(t)
	oldDB, oldCurrent, oldAssignments := scheduler.db, scheduler.CurrentRecording, scheduler.deviceAssignments
	oldDevices, oldStations := slot.Device, station.Cache
	scheduler.db, scheduler.CurrentRecording, scheduler.deviceAssignments = s.db, map[string]*RecordingSchedule{}, nil
	t.Cleanup(func() {
		scheduler.db, scheduler.CurrentRecording, scheduler.deviceAssignments = oldDB, oldCurrent, oldAssignments
		slot.Device, station.Cache = oldDevices, oldStations
	})
	sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
	insertDeviceAssignmentTestSchedule(t, s, sch)
	job := deviceAssignmentTestJob(sch)
	sch.DeviceID = "REC02"
	if err := scheduler.Update(CurrentSlot, sch); err != nil {
		t.Fatal(err)
	}
	watch := &RecordingSchedule{ID: sch.ID + 100, DeviceID: "REC01", WatchOnly: true}
	scheduler.CurrentRecording["REC01"] = watch
	slot.Device = map[string]*RockDevice{
		"REC01": {ID: "REC01", State: StateReady, Com: make(chan RequestMessage, 8)},
		"REC02": {ID: "REC02", State: StateReady, Com: make(chan RequestMessage, 8), RecordPath: "record"},
	}
	station.Cache = map[uint]*TvStation{}
	job.Lock()
	err := job.AcquireDeviceControl(nil)
	job.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { scheduler.releaseDevice(job) })
	if err := job.Boot(); err != nil {
		t.Fatal(err)
	}
	if err := job.StreamChange(); err != nil {
		t.Fatal(err)
	}
	if err := job.RecordStart(); err != nil {
		t.Fatal(err)
	}
	if err := job.RecordStop(); err != nil {
		t.Fatal(err)
	}
	if current, ok := scheduler.GetCurrentRecording("REC01"); !ok || current != watch {
		t.Fatal("recording replaced the watching reservation")
	}
	if len(slot.Device["REC01"].Com) != 0 {
		t.Fatal("watching device received recording commands")
	}
	for _, want := range []string{"StreamChange", "RecordStart", "RecordStop"} {
		select {
		case command := <-slot.Device["REC02"].Com:
			if command.ID != "REC02" || command.Command != want {
				t.Fatalf("received %+v, want REC02 %s", command, want)
			}
		default:
			t.Fatalf("missing command %s", want)
		}
	}
}

func TestDeviceAssignmentRefusesMissingDisabledAndUnassignedReservations(t *testing.T) {
	for _, kind := range []string{"missing", "disabled", "complete", "unassigned"} {
		t.Run(kind, func(t *testing.T) {
			s := newDeviceAssignmentTestScheduler(t)
			sch := deviceAssignmentTestSchedule(Now().Add(time.Hour).Unix())
			switch kind {
			case "disabled":
				sch.Validate = false
			case "complete":
				sch.Complete = true
			case "unassigned":
				sch.DeviceID = ""
			}
			if kind != "missing" {
				insertDeviceAssignmentTestSchedule(t, s, sch)
			}
			job := deviceAssignmentTestJob(sch)
			job.Lock()
			_, err := s.acquireDevice(job)
			job.Unlock()
			if err == nil || job.deviceLocked || len(s.CurrentRecording) != 0 {
				t.Fatalf("invalid reservation acquired control: err=%v locked=%v", err, job.deviceLocked)
			}
		})
	}
}
