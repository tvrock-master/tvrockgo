package tvrock

import (
	"testing"
	"time"
)

func TestRecordingShutdownDoesNotStopAnotherWatchingDevice(t *testing.T) {
	oldDevices := slot.Device
	oldCurrent := scheduler.CurrentRecording
	oldDelay := configure.DeviceShutdownTime
	oldTimers := shutdownTimer.shutdownDevice
	t.Cleanup(func() {
		// Wait for the timer callback before restoring its shared state.
		shutdownTimer.Lock()
		for _, timer := range shutdownTimer.shutdownDevice {
			timer.Stop()
		}
		shutdownTimer.shutdownDevice = oldTimers
		shutdownTimer.Unlock()
		slot.Device = oldDevices
		scheduler.CurrentRecording = oldCurrent
		configure.DeviceShutdownTime = oldDelay
	})

	recorder := &RockDevice{ID: "REC01", RegistryCode: "recorder", State: StateReady, Com: make(chan RequestMessage, 4)}
	watcher := &RockDevice{ID: "WATCH01", RegistryCode: "watcher", State: StateReady, Com: make(chan RequestMessage, 4)}
	slot.Device = map[string]*RockDevice{recorder.ID: recorder, watcher.ID: watcher}
	scheduler.CurrentRecording = map[string]*RecordingSchedule{}
	shutdownTimer.shutdownDevice = map[string]*time.Timer{}
	configure.DeviceShutdownTime = 0
	job := &RecordingJob{ID: 1, DeviceID: recorder.ID, Status: JobRecordStop, ExitMode: ExitModeShutDown, ExitTime: time.Now()}

	// Use the real stop-command routing and delayed shutdown callback, without
	// starting a recording application or connecting to a physical device.
	if err := job.sendRecordStop(); err != nil {
		t.Fatal(err)
	}
	job.EventRecordStop(job.ExitTime, false)
	for _, want := range []string{"RecordStop", "PowerOff"} {
		select {
		case msg := <-recorder.Com:
			if msg.ID != recorder.ID || msg.Command != want {
				t.Fatalf("recorder received %+v, want %s", msg, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("recorder did not receive %s", want)
		}
	}
	select {
	case msg := <-watcher.Com:
		t.Fatalf("unrelated watching device received %+v", msg)
	default:
	}
	if watcher.State != StateReady {
		t.Fatalf("watching device state changed: %v", watcher.State)
	}
}

func TestDeletingFutureTaskDoesNotShutdownWatchingDevice(t *testing.T) {
	oldDevices := slot.Device
	oldCurrent := scheduler.CurrentRecording
	oldDelay := configure.DeviceShutdownTime
	oldTimers := shutdownTimer.shutdownDevice
	t.Cleanup(func() {
		shutdownTimer.Lock()
		for _, timer := range shutdownTimer.shutdownDevice {
			timer.Stop()
		}
		shutdownTimer.shutdownDevice = oldTimers
		shutdownTimer.Unlock()
		slot.Device = oldDevices
		scheduler.CurrentRecording = oldCurrent
		configure.DeviceShutdownTime = oldDelay
	})
	configure.DeviceShutdownTime = 3600
	shutdownTimer.shutdownDevice = map[string]*time.Timer{}
	scheduler.CurrentRecording = map[string]*RecordingSchedule{}
	watcher := &RockDevice{ID: "REC01", State: StateReady, Com: make(chan RequestMessage, 4)}
	slot.Device = map[string]*RockDevice{watcher.ID: watcher}

	for _, action := range []string{"delete", "delete all", "force exit"} {
		t.Run(action, func(t *testing.T) {
			now := Now()
			job := &RecordingJob{
				ID: 1, DeviceID: watcher.ID, Validate: true, ChLock: true,
				ExitMode: ExitModeShutDown, BootType: DeviceBootTypeRecord,
				BootTime: now.Add(time.Hour), ExitTime: now.Add(2 * time.Hour),
				Com: make(chan ScheduleJobCom, 16),
			}
			ta := &TaskArray{Job: map[uint64]*RecordingJob{job.ID: job}}
			switch action {
			case "delete":
				ta.Delete(job)
			case "delete all":
				ta.DeleteAll()
			case "force exit":
				job.Com <- JobForceExit
			}
			// Execute the real queued deletion while the task is still waiting
			// for its boot time. No application or physical device is started.
			job.Exec(ta)
			shutdownTimer.Lock()
			timer := shutdownTimer.shutdownDevice[watcher.ID]
			if timer != nil {
				timer.Stop()
				delete(shutdownTimer.shutdownDevice, watcher.ID)
			}
			shutdownTimer.Unlock()
			if timer != nil {
				t.Error("deleting an unstarted task scheduled device shutdown")
			}
			select {
			case msg := <-watcher.Com:
				t.Fatalf("watching device received %+v", msg)
			default:
			}
			if watcher.State != StateReady {
				t.Fatalf("watching device state changed: %v", watcher.State)
			}
		})
	}
}
