package tvrock

import (
	"context"

	"testing"
	"time"
)

func TestLauncherReserveBoot(t *testing.T) {
	l := Launcher{}
	if err := l.Initialize(); err != nil {
		t.Fatal(err)
	}

	if !l.reserveBoot("RID1") {
		t.Fatal("first reserveBoot returned false")
	}
	if l.reserveBoot("RID1") {
		t.Fatal("second reserveBoot returned true")
	}

	l.releaseBoot("RID1")
	if !l.reserveBoot("RID1") {
		t.Fatal("reserveBoot after release returned false")
	}
}

func TestLauncherUnknownDeviceStatus(t *testing.T) {
	known := &RockDevice{ID: "known-device", RegistryCode: "known-registry", State: StateRecording}
	slot.Lock()
	previous := slot.Device
	slot.Device = map[string]*RockDevice{known.ID: known}
	slot.Unlock()
	t.Cleanup(func() {
		slot.Lock()
		slot.Device = previous
		slot.Unlock()
	})
	l := &Launcher{}
	for _, tc := range []struct{ name, message string }{
		{"unknown-on", LauncherComDeviceOn + "unknown-registry"},
		{"unknown-off", LauncherComDeviceOff + "unknown-registry"},
		{"empty-on", LauncherComDeviceOn},
		{"empty-off", LauncherComDeviceOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 存在しないデバイスを参照してパニックせず、登録済みデバイスにも影響しない。
			l.applyStatus(tc.message)
			if known.State != StateRecording {
				t.Fatal("unknown notification changed an existing device's state")
			}
		})
	}
}

func TestLauncherClientHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "empty", host: "", want: "localhost"},
		{name: "localhost", host: "localhost:80", want: "localhost:80"},
		{name: "unspecified ipv4", host: "0.0.0.0:80", want: "localhost:80"},
		{name: "port only", host: ":80", want: "localhost:80"},
		{name: "unspecified ipv6", host: "[::]:80", want: "localhost:80"},
		{name: "http scheme", host: "http://0.0.0.0:80", want: "localhost:80"},
		{name: "https scheme", host: "https://0.0.0.0:443", want: "localhost:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := launcherClientHost(tt.host); got != tt.want {
				t.Fatalf("launcherClientHost(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestLauncherWaitForConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("already connected", func(t *testing.T) {
		l := Launcher{}
		_ = l.Initialize()
		l.Pool["test"] = &LauncherConnection{HostName: "test"}

		if !l.WaitForConnection(ctx, 100*time.Millisecond) {
			t.Fatal("expected WaitForConnection to return true for existing connection")
		}
	})

	t.Run("connected while waiting", func(t *testing.T) {
		l := Launcher{}
		_ = l.Initialize()

		go func() {
			time.Sleep(20 * time.Millisecond)
			l.Lock()
			l.Pool["test"] = &LauncherConnection{HostName: "test"}
			l.Unlock()
			l.notifyConnected()
		}()

		if !l.WaitForConnection(ctx, 500*time.Millisecond) {
			t.Fatal("expected WaitForConnection to return true after notifyConnected")
		}
	})

	t.Run("timeout when not connected", func(t *testing.T) {
		l := Launcher{}
		_ = l.Initialize()

		if l.WaitForConnection(ctx, 50*time.Millisecond) {
			t.Fatal("expected WaitForConnection to return false on timeout")
		}
	})
}
