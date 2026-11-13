package tvrock

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestStartupDeviceIDs(t *testing.T) {
	schedules := []*RecordingSchedule{
		{DeviceID: "recording", Validate: true, StartTime: 90, EndTime: 110},
		{DeviceID: "recording", Validate: true, StartTime: 95, EndTime: 120},
		{DeviceID: "idle", Validate: true, StartTime: 110, IdleTime: 10, EndTime: 120},
		{DeviceID: "soon", Validate: true, StartTime: 103, EndTime: 120},
		{DeviceID: "tail", Validate: true, StartTime: 80, EndTime: 99, TailTime: 2},
		{DeviceID: "disabled", StartTime: 90, EndTime: 110},
		{DeviceID: "complete", Validate: true, Complete: true, StartTime: 90, EndTime: 110},
		{DeviceID: "ended", Validate: true, StartTime: 90, EndTime: 100},
		{DeviceID: "future", Validate: true, StartTime: 104, EndTime: 120},
		{Validate: true, StartTime: 90, EndTime: 110},
	}
	want := []string{"recording", "idle", "soon", "tail", ""}
	if got := startupDeviceIDs(schedules, 100, 103); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWaitForStartupDevices(t *testing.T) {
	t.Run("wait for all actual connections", func(t *testing.T) {
		var s Slot
		a := &RockDevice{ID: "a"}
		b := &RockDevice{ID: "b", State: StateBooting}
		s.setConnectionState(a, StateRecording)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.waitForDevices(ctx, []string{"a", "b"}) }()
		select {
		case err := <-done:
			t.Fatalf("returned before b reconnected: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		s.setConnectionState(b, StateReady)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		// A new waiter must observe an already completed reconnection.
		if err := s.waitForDevices(ctx, []string{"a", "b"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, state := range []DeviceState{StateOFF, StateBooting, StateShutdown} {
		t.Run("not ready "+string(rune('0'+state)), func(t *testing.T) {
			var s Slot
			dev := &RockDevice{ID: "a"}
			s.setConnectionState(dev, StateReady)
			s.setConnectionState(dev, state)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := s.waitForDevices(ctx, []string{"a"}); err != context.DeadlineExceeded {
				t.Fatalf("got %v, want deadline exceeded", err)
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		var s Slot
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := s.waitForDevices(ctx, []string{"missing"}); err != context.Canceled {
			t.Fatalf("got %v, want canceled", err)
		}
	})
	t.Run("no reservations", func(t *testing.T) {
		var s Slot
		if err := s.waitForDevices(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	})
}
