package tvrock

import (
	"strconv"
	"testing"
	"time"
)

func TestDisconnectedDeviceDoesNotReusePreviousService(t *testing.T) {
	var s Slot
	dev := &RockDevice{
		ID: "test-recorder", CurrentService: 66592,
		CurrentProgram: &TvProgram{Sid: 66592, Eid: 1},
		NextProgram:    &TvProgram{Sid: 66592, Eid: 2},
	}
	s.setConnectionState(dev, StateRecording)
	if dev.CurrentService != 66592 || dev.CurrentProgram == nil || dev.NextProgram == nil {
		t.Fatal("recording state discarded valid service information")
	}
	s.setConnectionState(dev, StateOFF)
	s.setConnectionState(dev, StateReady)
	if dev.CurrentService != 0 || dev.CurrentProgram != nil || dev.NextProgram != nil {
		t.Fatal("reconnected device reused previous service/programs before a new tuning notification")
	}
}

func TestRecordingRestartWaitsForReservedService(t *testing.T) {
	for _, initialService := range []uint{0, 66560, 66592} {
		t.Run(strconv.FormatUint(uint64(initialService), 10), func(t *testing.T) {
			oldDevices, oldCurrent, oldStations := slot.Device, scheduler.CurrentRecording, station.Cache
			t.Cleanup(func() {
				slot.Device, scheduler.CurrentRecording, station.Cache = oldDevices, oldCurrent, oldStations
			})
			queue := make(chan RequestMessage, 8)
			dev := &RockDevice{ID: "test-recorder", State: StateReady, CurrentService: initialService,
				Com: queue, requestDone: make(chan struct{}), RecordPath: "record"}
			slot.Device = map[string]*RockDevice{dev.ID: dev}
			station.Cache = map[uint]*TvStation{}
			scheduler.CurrentRecording = map[string]*RecordingSchedule{dev.ID: {ID: 900001, DeviceID: dev.ID}}
			now := time.Unix(1000, 0)
			job := &RecordingJob{ID: 900001, DeviceID: dev.ID, Sid: 66592, Title: "test recording",
				BootType: DeviceBootTypeRecord, Status: JobRecordStart,
				RecordStartTime: now.Add(-time.Minute), RecordStopTime: now.Add(time.Hour)}
			job.EventRecordStart(now, false)
			if initialService != job.Sid {
				if job.getStatus() != JobStreamChange || len(queue) != 0 {
					t.Fatal("recording restarted before the reserved service was selected")
				}
				job.EventStreamChange(now, false)
				select {
				case request := <-queue:
					if request.Command != "StreamChange" || request.Message != "66592" {
						t.Fatalf("expected tuning request for reservation, got %+v", request)
					}
				default:
					t.Fatal("reserved service was not requested")
				}
				if len(queue) != 0 || job.getStatus() != JobStreamChange {
					t.Fatal("recording started without a tuning completion notification")
				}
				// Simulate the new connection reporting that the reserved service is tuned.
				dev.CurrentService = job.Sid
				job.EventStreamChange(now, false)
			}
			select {
			case request := <-queue:
				if request.Command != "RecordStart" {
					t.Fatalf("expected recording after tuning confirmation, got %+v", request)
				}
			default:
				t.Fatal("recording did not start after the reserved service was confirmed")
			}
			if job.getStatus() != JobRecordStart || len(queue) != 0 {
				t.Fatal("unexpected state or duplicate recording request")
			}
			// A different service must not delay stopping an expired recording.
			dev.CurrentService = 66560
			job.RecordStopTime = now
			job.EventRecordStart(now, false)
			select {
			case request := <-queue:
				if request.Command != "RecordStop" || job.getStatus() != JobRecordStop {
					t.Fatalf("expired recording should stop without retuning, got %+v", request)
				}
			default:
				t.Fatal("expired recording did not stop")
			}
		})
	}
}
