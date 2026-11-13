//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/tvrock-master/tvrockgo/tvrock"
	"golang.org/x/sys/windows"
)

func TestWindowsConsoleInterrupt(t *testing.T) {
	if os.Getenv("TVROCKGO_CONSOLE_INTERRUPT_TEST") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsConsoleInterrupt$")
		cmd.Env = append(os.Environ(), "TVROCKGO_CONSOLE_INTERRUPT_TEST=1")
		// Send events only to this helper's private, hidden console.
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_CONSOLE}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("console interrupt helper: %v\n%s", err, output)
		}
		return
	}
	if err := prepareConsole(true); err != nil {
		t.Fatal(err)
	}
	input, err := windows.CreateFile(windows.StringToUTF16Ptr("CONIN$"), windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(input)
	var originalMode uint32
	if err := windows.GetConsoleMode(input, &originalMode); err != nil {
		t.Fatal(err)
	}
	rawMode := originalMode &^ (windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
	if err := windows.SetConsoleMode(input, rawMode); err != nil {
		t.Fatal(err)
	}
	proc := kernel32.NewProc("SetConsoleCtrlHandler")
	if ok, _, err := proc.Call(0, 1); ok == 0 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	cleanup, err := installPlatformStop(cancel, stopped)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()
	defer close(stopped)
	var mode uint32
	if err := windows.GetConsoleMode(input, &mode); err != nil || mode&windows.ENABLE_PROCESSED_INPUT == 0 {
		t.Fatalf("Ctrl+C input remains disabled: mode=%#x, error=%v", mode, err)
	}
	if ok, _, err := kernel32.NewProc("GenerateConsoleCtrlEvent").Call(0, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl+C did not cancel the context")
	}
	cleanup()
	cleanup = nil
	if err := windows.GetConsoleMode(input, &mode); err != nil || mode != rawMode {
		t.Fatalf("input mode was not restored: mode=%#x, want=%#x, error=%v", mode, rawMode, err)
	}
}

func TestNormalizeConsoleInputMode(t *testing.T) {
	for _, tt := range []struct{ mode, want uint32 }{
		{484, 480}, // PowerShell's observed raw mode with an inactive echo flag.
		{485, 481}, // Enable Ctrl+C without introducing an invalid echo/line combination.
		{windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT, windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT},
	} {
		if got := normalizeConsoleInputMode(tt.mode); got != tt.want {
			t.Errorf("mode=%#x: got %#x, want %#x", tt.mode, got, tt.want)
		}
	}
}

func TestWindowsNativeLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("Windows x64 distribution")
	}
	if got := unsafe.Sizeof(windowClass{}); got != 80 {
		t.Fatalf("WNDCLASSEXW size = %d", got)
	}
	if got := unsafe.Sizeof(winMessage{}); got != 48 {
		t.Fatalf("MSG size = %d", got)
	}
	if got := unsafe.Sizeof(trayIconData{}); got != 976 {
		t.Fatalf("NOTIFYICONDATAW size = %d", got)
	}
}

func TestTrayRightClickOpensMenuOnce(t *testing.T) {
	for _, version4 := range []bool{false, true} {
		count := 0
		for _, event := range []uint32{0x0204, wmRightButtonUp, wmContextMenu} {
			if trayOpensMenu(event, version4) {
				count++
			}
		}
		if count != 1 {
			t.Errorf("version4=%t: one right click opened %d menus", version4, count)
		}
		for _, event := range []uint32{wmLeftButtonUp, ninSelect, ninKeySelect, 0x0402} {
			if trayOpensMenu(event, version4) {
				t.Errorf("version4=%t: unrelated event %#x opens menu", version4, event)
			}
		}
	}
	if !trayOpensMenu(wmContextMenu, true) {
		t.Error("version 4 keyboard context menu must remain available")
	}
}

func TestTrayIconFromEmbeddedPNG(t *testing.T) {
	for name, data := range map[string][]byte{"normal": trayPNG, "sleep": traySleepPNG} {
		t.Run(name, func(t *testing.T) {
			icon, err := createTrayIconFromPNG(data)
			if err != nil {
				t.Fatal(err)
			}
			if icon == 0 {
				t.Fatal("empty icon handle")
			}
			user32.NewProc("DestroyIcon").Call(icon)
		})
	}
}

func TestSleepTimerTrayState(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, paused := range []bool{false, true} {
			setting := tvrock.PowerManagerSetting{PowerManager: enabled, SleepTimerPause: paused}
			if got := sleepTimerActive(setting); got != (enabled && !paused) {
				t.Errorf("enabled=%t paused=%t: sleep icon=%t", enabled, paused, got)
			}
			items := trayMenuItems(setting)
			found := false
			for _, item := range items {
				if item.ID == traySleepTimer {
					found = true
					if item.Label != "スリープタイマー" || item.Checked != !paused {
						t.Errorf("unexpected sleep menu: %+v", item)
					}
				}
			}
			if found != enabled {
				t.Errorf("enabled=%t paused=%t: sleep menu visible=%t", enabled, paused, found)
			}
			if items[len(items)-1].ID != 3 {
				t.Error("exit command ID changed")
			}
		}
	}
}

func TestTrayLeftClickDoesNotToggleTwice(t *testing.T) {
	// Version 4 can report both raw mouse messages and NIN_SELECT for one click.
	for _, version4 := range []bool{false, true} {
		count := 0
		for _, event := range []uint32{0x0201, wmLeftButtonUp, ninSelect} {
			if trayActivatesSleepTimer(event, version4) {
				count++
			}
		}
		if count != 1 {
			t.Errorf("version4=%t: one click triggered %d toggles", version4, count)
		}
		for _, event := range []uint32{wmContextMenu, 0x0205, 0x0203, 0x0402} {
			if trayActivatesSleepTimer(event, version4) {
				t.Errorf("version4=%t: unrelated event %#x toggles", version4, event)
			}
		}
	}
}
