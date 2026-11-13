package tvrock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"
)

func shutdownTestDevice(t *testing.T) *RockDevice {
	t.Helper()
	oldDevices, oldCurrent := slot.Device, scheduler.CurrentRecording
	oldTimers, oldDelay := shutdownTimer.shutdownDevice, configure.DeviceShutdownTime
	oldReady, oldChanged := slot.connectedReady, slot.connectionChanged
	slot.connectedReady = map[string]bool{}
	slot.connectionChanged = make(chan struct{})
	shutdownTimer.shutdownDevice = map[string]*time.Timer{}
	configure.DeviceShutdownTime = 3600 // テストはコールバックを明示的に進める。
	scheduler.CurrentRecording = map[string]*RecordingSchedule{}
	d := &RockDevice{ID: "shutdown-regression", RegistryCode: "shutdown-regression-code", State: StateOFF}
	slot.Device = map[string]*RockDevice{d.ID: d}
	t.Cleanup(func() {
		shutdownTimer.Lock()
		for _, timer := range shutdownTimer.shutdownDevice {
			timer.Stop()
		}
		shutdownTimer.shutdownDevice = oldTimers
		shutdownTimer.Unlock()
		serviceWorkers.wg.Wait()
		slot.Device, scheduler.CurrentRecording = oldDevices, oldCurrent
		slot.connectedReady, slot.connectionChanged = oldReady, oldChanged
		configure.DeviceShutdownTime = oldDelay
	})
	return d
}

func pendingTestShutdown(t *testing.T, d *RockDevice) *deviceShutdownRequest {
	t.Helper()
	r := &deviceShutdownRequest{deviceID: d.ID, jobID: 990001, deadline: time.Now().Add(time.Minute)}
	r.timer = time.AfterFunc(time.Hour, r.run)
	shutdownTimer.Lock()
	shutdownTimer.shutdownDevice[d.ID] = r.timer
	shutdownTimer.Unlock()
	return r
}

func TestDeviceConnectionRegistrationIsExclusive(t *testing.T) {
	var s Slot
	d := &RockDevice{ID: "recorder"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := make(chan RequestMessage, 4), make(chan RequestMessage, 4)
	if _, accepted := s.attachConnection(d, first, ctx.Done()); !accepted {
		t.Fatal("first connection rejected")
	}
	if d.State != StateBooting {
		t.Fatal("connection remains OFF before its first status report")
	}
	if _, accepted := s.attachConnection(d, second, context.Background().Done()); accepted {
		t.Fatal("duplicate connection replaced the command queue before Ready")
	}
	cancel()
	if _, accepted := s.attachConnection(d, second, nil); accepted {
		t.Fatal("new connection accepted before the old reader finished")
	}
	s.detachConnection(d, first)
	if reconnect, accepted := s.attachConnection(d, second, nil); !accepted || !reconnect {
		t.Fatal("reconnection rejected after cleanup")
	}
	s.setConnectionState(d, StateReady)
	s.detachConnection(d, first)
	if d.State != StateReady || d.Com != second {
		t.Fatal("stale connection cleanup corrupted the new connection")
	}
}

func TestDeviceShutdownRetriesAfterReconnect(t *testing.T) {
	d := shutdownTestDevice(t)
	r := pendingTestShutdown(t, d)
	r.run() // 終了要求の瞬間は切断中。
	if shutdownTimer.shutdownDevice[d.ID] != r.timer {
		t.Fatal("disconnected device lost its pending shutdown")
	}
	for attempt := range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		queue := make(chan RequestMessage, 4)
		if _, ok := slot.attachConnection(d, queue, ctx.Done()); !ok {
			t.Fatal("test connection rejected")
		}
		slot.setConnectionState(d, StateReady)
		r.run()
		var message RequestMessage
		select {
		case message = <-queue:
		default:
			t.Fatal("PowerOff was not retried on the new connection")
		}
		if message.Command != "PowerOff" || message.shutdown != r {
			t.Fatalf("unexpected request: %+v", message)
		}
		sends := 0
		err := message.shutdown.dispatch(d, queue, func(RequestMessage) error {
			sends++
			if attempt == 0 {
				return errors.New("simulated broken socket")
			}
			return nil
		})
		if sends != 1 || (attempt == 0 && err == nil) || (attempt == 1 && err != nil) {
			t.Fatalf("dispatch: sends=%d error=%v", sends, err)
		}
		r.run()
		if len(queue) != 0 {
			t.Fatal("duplicate shutdown was queued on the same connection")
		}
		cancel()
		slot.detachConnection(d, queue)
	}
	d.processExited = true // ランチャーによる実際のプロセス終了通知。
	r.run()
	if shutdownTimer.shutdownDevice[d.ID] != nil {
		t.Fatal("confirmed process exit did not clear shutdown")
	}
}

func TestDeviceShutdownDoesNotBlockOnFullQueue(t *testing.T) {
	d := shutdownTestDevice(t)
	d.State, d.Com = StateReady, make(chan RequestMessage, 1)
	d.Com <- RequestMessage{Command: "RecordStop"}
	r := pendingTestShutdown(t, d)
	r.run()
	if r.queued != nil {
		t.Fatal("full queue was treated as successful delivery")
	}
	<-d.Com
	r.run()
	if message := <-d.Com; message.Command != "PowerOff" {
		t.Fatal("shutdown was not retried after queue space became available")
	}
}

func TestDeviceShutdownDiscardsQueuedRequestForNextOwner(t *testing.T) {
	for _, owner := range []string{"recording", "watching", "cancelled", "recording state"} {
		t.Run(owner, func(t *testing.T) {
			d := shutdownTestDevice(t)
			d.State, d.Com = StateReady, make(chan RequestMessage, 4)
			r := pendingTestShutdown(t, d)
			r.run()
			message := <-d.Com // 送信待ちの古い終了要求。
			switch owner {
			case "recording":
				scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: 990002, DeviceID: d.ID}
			case "watching":
				remoteWatching.Lock()
				previous := remoteWatching.sessions
				remoteWatching.sessions = map[string]*watchSession{"viewer": {DeviceID: d.ID}}
				remoteWatching.Unlock()
				t.Cleanup(func() { remoteWatching.Lock(); remoteWatching.sessions = previous; remoteWatching.Unlock() })
			case "cancelled":
				(&RecordingJob{DeviceID: d.ID}).ShutdownCancel()
			case "recording state":
				d.State = StateRecording
			}
			if err := message.shutdown.dispatch(d, d.Com, func(RequestMessage) error {
				t.Error("old PowerOff reached a subsequent recording/viewer")
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if shutdownTimer.shutdownDevice[d.ID] != nil {
				t.Fatal("retired shutdown can still retry")
			}
		})
	}
}

func TestDeviceShutdownDeadline(t *testing.T) {
	d := shutdownTestDevice(t)
	r := pendingTestShutdown(t, d)
	r.deadline = time.Now().Add(-time.Second)
	r.run()
	if shutdownTimer.shutdownDevice[d.ID] != nil {
		t.Fatal("shutdown retry did not expire")
	}
}

func TestDeviceShutdownRetiresLateRequestAfterHandover(t *testing.T) {
	for _, state := range []DeviceState{StateReady, StateRecording, StateOFF} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			d := shutdownTestDevice(t)
			d.State, d.Com = state, make(chan RequestMessage, 4)
			// 後続タスクが取得時に古い監視を取り消した後で、終了元が監視を登録する順序。
			scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: 990002, DeviceID: d.ID}
			r := pendingTestShutdown(t, d)
			r.deadline = time.Now().Add(-time.Second) // 期限切れ警告より引き継ぎを優先。
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(previous)
			r.run()
			if shutdownTimer.shutdownDevice[d.ID] != nil || len(d.Com) != 0 {
				t.Fatal("late shutdown was not retired before it could affect the next owner")
			}
			if strings.Contains(output.String(), "warn:") || !strings.Contains(output.String(), "デバイス終了を中止") {
				t.Fatalf("handover should be informational: %s", output.String())
			}
			// 後続タスク自身の終了要求は通常どおり送る。
			delete(scheduler.CurrentRecording, d.ID)
			d.State = StateReady
			next := pendingTestShutdown(t, d)
			next.jobID = 990002
			next.run()
			select {
			case message := <-d.Com:
				if message.shutdown != next {
					t.Fatal("old shutdown survived handover")
				}
			default:
				t.Fatal("the final recording lost its shutdown request")
			}
		})
	}
}

func TestDeviceShutdownRetiresLateRequestForViewer(t *testing.T) {
	d := shutdownTestDevice(t)
	d.State = StateRecording
	remoteWatching.Lock()
	previous := remoteWatching.sessions
	remoteWatching.sessions = map[string]*watchSession{"viewer": {DeviceID: d.ID}}
	remoteWatching.Unlock()
	t.Cleanup(func() { remoteWatching.Lock(); remoteWatching.sessions = previous; remoteWatching.Unlock() })
	r := pendingTestShutdown(t, d)
	r.run()
	if shutdownTimer.shutdownDevice[d.ID] != nil {
		t.Fatal("active viewer did not retire the shutdown while the device was not Ready")
	}
}

func TestDeviceShutdownKeepsUnconfirmedStopWithoutNextOwner(t *testing.T) {
	d := shutdownTestDevice(t)
	d.State = StateRecording
	r := pendingTestShutdown(t, d)
	scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: r.jobID, DeviceID: d.ID}
	r.run()
	if shutdownTimer.shutdownDevice[d.ID] != r.timer {
		t.Fatal("the finishing recording was mistaken for a subsequent owner")
	}
	r.deadline = time.Now().Add(-time.Second)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	r.run()
	if shutdownTimer.shutdownDevice[d.ID] != nil || !strings.Contains(output.String(), "warn: デバイスの終了を確認できず") {
		t.Fatalf("a real unconfirmed stop must still expire with a warning: %s", output.String())
	}
}

func TestDeviceShutdownWaitsForFinishingTaskToReleaseControl(t *testing.T) {
	d := shutdownTestDevice(t)
	d.State, d.Com = StateReady, make(chan RequestMessage, 4)
	r := pendingTestShutdown(t, d)
	scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: r.jobID, DeviceID: d.ID}
	r.run()
	message := <-d.Com
	if err := message.shutdown.dispatch(d, d.Com, func(RequestMessage) error {
		t.Error("PowerOff was sent before the finishing task released control")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if shutdownTimer.shutdownDevice[d.ID] != r.timer {
		t.Fatal("zero-delay shutdown was lost while the finishing task still owned the device")
	}
	delete(scheduler.CurrentRecording, d.ID)
	r.run()
	message = <-d.Com
	sent := false
	if err := message.shutdown.dispatch(d, d.Com, func(RequestMessage) error { sent = true; return nil }); err != nil || !sent {
		t.Fatalf("shutdown did not proceed after control was released: %v", err)
	}
}

func TestDeviceShutdownProtectsUpcomingScheduleAndAcquisition(t *testing.T) {
	d := shutdownTestDevice(t)
	d.State, d.Com = StateReady, make(chan RequestMessage, 4)
	s := newDeviceAssignmentTestScheduler(t)
	oldDB := scheduler.db
	scheduler.db = s.db
	t.Cleanup(func() { scheduler.db = oldDB })
	sch := deviceAssignmentTestSchedule(Now().Add(2 * time.Minute).Unix())
	sch.DeviceID = d.ID
	insertDeviceAssignmentTestSchedule(t, s, sch)
	r := pendingTestShutdown(t, d)
	r.run()
	message := <-d.Com
	if err := message.shutdown.dispatch(d, d.Com, func(RequestMessage) error {
		t.Error("PowerOff reached the device immediately before its next reservation")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if shutdownTimer.shutdownDevice[d.ID] != nil {
		t.Fatal("shutdown for upcoming reservation was not cancelled")
	}
	r = pendingTestShutdown(t, d)
	job := deviceAssignmentTestJob(sch)
	job.Lock()
	_, err := s.acquireDevice(job)
	job.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if shutdownTimer.shutdownDevice[d.ID] == r.timer {
		t.Fatal("new task acquired the recorder without cancelling old shutdown")
	}
}
