//go:build linux || darwin

package tvrock

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type sleepInhibitor struct {
	mux     sync.Mutex
	cmd     *exec.Cmd
	enabled bool
}

type wakeTaskManager struct {
	mux      sync.Mutex
	wakeTime time.Time
	known    bool
}

type sleepTimerManager struct {
	mux             sync.Mutex
	noScheduleSince time.Time
	requested       bool
}

var (
	inhibitor  sleepInhibitor
	wakeTask   wakeTaskManager
	sleepTimer sleepTimerManager
)

func (p *sleepInhibitor) set(enabled bool, reason string) {
	p.mux.Lock()
	defer p.mux.Unlock()

	if enabled && CurrentPowerManagerSetting().PreventSleep {
		if p.enabled {
			return
		}
		cmd, err := startSleepInhibitor(reason)
		if err != nil {
			log.Printf("error: スリープ抑止の開始に失敗: %v", err)
			return
		}
		p.cmd = cmd
		p.enabled = true
		log.Printf("debug: スリープ抑止: %s", reason)
		return
	}

	if !p.enabled {
		return
	}
	if err := stopCommand(p.cmd); err != nil {
		log.Printf("error: スリープ抑止の解除に失敗: %v", err)
		return
	}
	p.cmd = nil
	p.enabled = false
	log.Printf("debug: スリープ抑止解除")
}

func startSleepInhibitor(reason string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemd-inhibit"); err == nil {
			cmd := exec.Command("systemd-inhibit", "--what=sleep", "--mode=block", "--why="+reason, "sleep", "infinity")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return cmd, cmd.Start()
		}
		return nil, errors.New("systemd-inhibit が見つかりません")

	case "darwin":
		if _, err := exec.LookPath("caffeinate"); err == nil {
			cmd := exec.Command("caffeinate", "-i", "-s")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return cmd, cmd.Start()
		}
		return nil, errors.New("caffeinate が見つかりません")
	}

	return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

func stopCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return err
	}
	_, _ = cmd.Process.Wait()
	return nil
}

func syncWakeTask(schedules []*RecordingSchedule, now time.Time) {
	wakeTask.mux.Lock()
	defer wakeTask.mux.Unlock()

	if len(schedules) == 0 {
		wakeTask.clear()
		return
	}

	wakeTime := time.Unix(schedules[0].StartTime, 0).Add(-(time.Duration(CurrentPowerManagerSetting().WakeBeforeSchedule) * time.Minute))
	if !wakeTime.After(now.Add(time.Minute)) {
		wakeTask.clear()
		return
	}

	if wakeTask.known && wakeTask.wakeTime.Equal(wakeTime) {
		return
	}

	if err := wakeTask.set(wakeTime); err != nil {
		log.Printf("error: スリープ解除予約の設定に失敗: %v", err)
		return
	}

	wakeTask.wakeTime = wakeTime
	wakeTask.known = true
	log.Printf("info: 次の予約に向けたスリープ解除予約を設定: %s", wakeTime.Format(time.RFC3339))
}

func (p *wakeTaskManager) set(wakeTime time.Time) error {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("rtcwake"); err != nil {
			return errors.New("rtcwake が見つかりません")
		}
		out, err := exec.Command("rtcwake", "-m", "no", "-t", fmt.Sprintf("%d", wakeTime.Unix())).CombinedOutput()
		if err != nil {
			return commandError(err, out)
		}
		return nil

	case "darwin":
		return errors.New("macOS の wake 予約は未対応です")
	}

	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

func (p *wakeTaskManager) clear() {
	if p.known {
		if err := p.disable(); err != nil {
			log.Printf("error: スリープ解除予約の解除に失敗: %v", err)
			return
		}
		log.Println("info: スリープ解除予約を解除")
	}
	p.wakeTime = time.Time{}
	p.known = false
}

func (p *wakeTaskManager) disable() error {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("rtcwake"); err != nil {
			return errors.New("rtcwake が見つかりません")
		}
		out, err := exec.Command("rtcwake", "-m", "disable").CombinedOutput()
		if err != nil {
			return commandError(err, out)
		}
		return nil

	case "darwin":
		return nil
	}

	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

func commandError(err error, out []byte) error {
	if len(out) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, string(out))
}

func syncSleepTimer(schedules []*RecordingSchedule, now time.Time, busyCount int) {
	sleepTimer.mux.Lock()
	defer sleepTimer.mux.Unlock()

	if CurrentPowerManagerSetting().SleepTimer <= 0 ||
		CurrentPowerManagerSetting().SleepTimerPause ||
		scheduler.GetCurrentRecordings() > 0 ||
		(CurrentPowerManagerSetting().PreventIfDeviceRunning && slot.IsAnyRunning()) ||
		playbackActive() || tsAnalysisActive() ||
		busyCount > 0 {
		sleepTimer.noScheduleSince = time.Time{}
		sleepTimer.requested = false
		return
	}

	if sleepTimer.noScheduleSince.IsZero() {
		sleepTimer.noScheduleSince = now
		return
	}

	sleepAfter := time.Duration(CurrentPowerManagerSetting().SleepTimer) * time.Minute
	if sleepTimer.requested || now.Sub(sleepTimer.noScheduleSince) < sleepAfter {
		return
	}

	log.Printf("info: 予約が無い状態が %s 続いたためスリープへ移行します", sleepAfter)
	sleepTimer.requested = requestSystemSleep()
}

func requestSystemSleep() bool {
	inhibitor.set(false, "")

	cmd, err := sleepCommand()
	if err != nil {
		log.Printf("error: スリープへの移行に失敗: %v", err)
		return false
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error: スリープへの移行に失敗: %v", commandError(err, out))
		return false
	}
	return true
}

func sleepCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return nil, errors.New("systemctl が見つかりません")
		}
		return exec.Command("systemctl", "suspend"), nil

	case "darwin":
		if _, err := exec.LookPath("pmset"); err != nil {
			return nil, errors.New("pmset が見つかりません")
		}
		return exec.Command("pmset", "sleepnow"), nil
	}

	return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

func updatePowerState(reason string) {
	nowTime := Now()
	now := nowTime.Unix()

	sch, err := scheduler.GetNextSchedules()
	if err != nil {
		log.Println("error: ", err)
		inhibitor.set(true, reason)
		return
	}

	syncWakeTask(sch, nowTime)
	busyCount := customCommandManager.GetBusyCount()
	syncSleepTimer(sch, nowTime, busyCount)

	shouldPreventSleep := (len(sch) > 0 && sch[0].StartTime < now+CurrentPowerManagerSetting().PreventSleepTime) ||
		(CurrentPowerManagerSetting().PreventIfDeviceRunning && slot.IsAnyRunning()) ||
		playbackActive() || tsAnalysisActive() ||
		busyCount > 0

	inhibitor.set(shouldPreventSleep, reason)
}

func preventSleep(ctx context.Context, reason string) {
	log.Println("info: スリープ抑止スレッドを開始")
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	updatePowerState(reason)

L:
	for {
		select {
		case <-ctx.Done():
			break L

		case <-ticker.C:
			updatePowerState(reason)
		}
	}

	inhibitor.set(false, reason)
	log.Println("info: スリープ抑止スレッドの終了")
}

func InitializePowerManager() error {

	if !CurrentPowerManagerSetting().PowerManager {
		return nil
	}

	serviceWorkers.start(func() { preventSleep(globalContext, "バックグラウンドタスク実行中") })
	return nil
}
