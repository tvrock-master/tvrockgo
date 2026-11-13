package tvrock

import (
	"os"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestTraySleepTimerTogglePersistsAndNotifies(t *testing.T) {
	previous := CurrentPowerManagerSetting()
	previousHandlers := deviceController.Handler
	t.Cleanup(func() {
		configure.PowerManagerSetting = previous
		deviceController.Handler = previousHandlers
	})
	t.Chdir(t.TempDir())
	if err := os.Mkdir("conf", 0700); err != nil {
		t.Fatal(err)
	}
	queue := &DevConQueue{Com: make(chan string, 2), Done: make(chan struct{})}
	deviceController.Handler = map[string]*DevConQueue{"client": queue}
	configure.PowerManagerSetting = PowerManagerSetting{PowerManager: true, SleepTimer: 15, PreventSleep: true}
	for _, wantPause := range []bool{true, false} {
		setting, err := ToggleSleepTimerPause()
		if err != nil {
			t.Fatal(err)
		}
		if setting.SleepTimerPause != wantPause || setting.SleepTimer != 15 || !setting.PreventSleep {
			t.Fatalf("setting = %+v", setting)
		}
		data, err := os.ReadFile(TvRockConfigureFile)
		if err != nil {
			t.Fatal(err)
		}
		var saved TvRockConf
		if err := yaml.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.PowerManagerSetting != setting {
			t.Fatal("saved setting differs from live setting")
		}
		select {
		case message := <-queue.Com:
			if message != ControllerUpdatePowerManagerSetting {
				t.Fatalf("notification = %q", message)
			}
		default:
			t.Fatal("missing client notification")
		}
	}
}

func TestTraySleepTimerToggleSaveFailureAndDisabled(t *testing.T) {
	previous := CurrentPowerManagerSetting()
	previousHandlers := deviceController.Handler
	t.Cleanup(func() {
		configure.PowerManagerSetting = previous
		deviceController.Handler = previousHandlers
	})
	t.Chdir(t.TempDir()) // No conf directory: saving must fail without touching real settings.
	queue := &DevConQueue{Com: make(chan string, 1), Done: make(chan struct{})}
	deviceController.Handler = map[string]*DevConQueue{"client": queue}
	configure.PowerManagerSetting = PowerManagerSetting{PowerManager: true, SleepTimerPause: true}
	before := CurrentPowerManagerSetting()
	if _, err := ToggleSleepTimerPause(); err == nil {
		t.Fatal("expected save error")
	}
	if CurrentPowerManagerSetting() != before {
		t.Fatal("failed save changed live settings")
	}
	select {
	case <-queue.Com:
		t.Fatal("failed save notified clients")
	default:
	}
	configure.PowerManagerSetting.PowerManager = false
	before = CurrentPowerManagerSetting()
	if got, err := ToggleSleepTimerPause(); err != nil || got != before {
		t.Fatalf("disabled timer changed: %+v, %v", got, err)
	}
}
