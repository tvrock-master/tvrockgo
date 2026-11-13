//go:build windows

package tvrock

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

type SleepEvent int

const (
	EventSuspending SleepEvent = iota // スリープ移行
	EventResumed                      // スリープ復帰
)

const (
	WM_QUIT           = 0x0012
	WM_POWERBROADCAST = 0x0218

	// 電源イベント定数 (wParam)
	PBT_APMSUSPEND         = 0x0004 // スリープ移行時
	PBT_APMRESUMESUSPEND   = 0x0007 // スリープから復帰時
	PBT_APMRESUMEAUTOMATIC = 0x0012 // 自動タイマー等での復帰時

	ES_CONTINUOUS        = 0x80000000 // 要求を解除するまで維持
	ES_SYSTEM_REQUIRED   = 0x00000001 // システム（本体）の自動スリープを阻止
	ES_DISPLAY_REQUIRED  = 0x00000002 // 画面（ディスプレイ）の自動オフを阻止
	ES_AWAYMODE_REQUIRED = 0x00000040 // 画面オフは許可しつつ、バックグラウンド実行を継続させたい場合（モダンスタンバイ/Windows 8以降）
)

var (
	user32                      = syscall.NewLazyDLL("user32.dll")
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW      = user32.NewProc("PostThreadMessageW")
	procGetModuleHandleW        = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadId      = kernel32.NewProc("GetCurrentThreadId")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procSetSuspendState         = syscall.NewLazyDLL("powrprof.dll").NewProc("SetSuspendState")
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var globalEventChan chan<- SleepEvent
var wakeUpSuppressUntil atomic.Int64
var wakeTask wakeTaskManager
var sleepTimer sleepTimerManager

type wakeTaskManager struct {
	mux      sync.Mutex
	wakeTime time.Time
	known    bool
	synced   bool
}

type sleepTimerManager struct {
	mux             sync.Mutex
	noScheduleSince time.Time
	requested       bool
}

type taskXML struct {
	XMLName          xml.Name         `xml:"Task"`
	Xmlns            string           `xml:"xmlns,attr"`
	Version          string           `xml:"version,attr"`
	RegistrationInfo registrationInfo `xml:"RegistrationInfo"`
	Triggers         triggers         `xml:"Triggers"`
	Settings         taskSettings     `xml:"Settings"`
	Actions          actions          `xml:"Actions"`
}

type registrationInfo struct {
	Date        string `xml:"Date"`
	Author      string `xml:"Author"`
	Description string `xml:"Description"`
}

type triggers struct {
	TimeTrigger timeTrigger `xml:"TimeTrigger"`
}

type timeTrigger struct {
	StartBoundary string `xml:"StartBoundary"`
	Enabled       bool   `xml:"Enabled"`
}

type taskSettings struct {
	MultipleInstancesPolicy    string `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool   `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool   `xml:"StopIfGoingOnBatteries"`
	AllowHardTerminate         bool   `xml:"AllowHardTerminate"`
	StartWhenAvailable         bool   `xml:"StartWhenAvailable"`
	RunOnlyIfNetworkAvailable  bool   `xml:"RunOnlyIfNetworkAvailable"`
	IdleSettings               struct {
		StopOnIdleEnd bool `xml:"StopOnIdleEnd"`
		RestartOnIdle bool `xml:"RestartOnIdle"`
	} `xml:"IdleSettings"`
	AllowStartOnDemand bool   `xml:"AllowStartOnDemand"`
	Enabled            bool   `xml:"Enabled"`
	Hidden             bool   `xml:"Hidden"`
	WakeToRun          bool   `xml:"WakeToRun"`
	ExecutionTimeLimit string `xml:"ExecutionTimeLimit"`
	Priority           int    `xml:"Priority"`
}

type actions struct {
	Exec taskExec `xml:"Exec"`
}

type taskExec struct {
	Command   string `xml:"Command"`
	Arguments string `xml:"Arguments"`
}

func notifySleepEvent(event SleepEvent) {
	if globalEventChan == nil {
		return
	}

	select {
	case globalEventChan <- event:
	default:
	}
}

func suppressSleepAfterWake() {
	wakeUpSuppressUntil.Store(Now().Add(time.Duration(CurrentPowerManagerSetting().WakeUpSuppressDuration) * time.Second).Unix())
}

func isSuppressingAfterWake(now time.Time) bool {
	return now.Unix() < wakeUpSuppressUntil.Load()
}

func setSleepPrevention(enabled bool) {
	state := uintptr(ES_CONTINUOUS)
	if enabled && CurrentPowerManagerSetting().PreventSleep {
		state = uintptr(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)
		if CurrentPowerManagerSetting().PreventDisplayOff {
			state |= uintptr(ES_DISPLAY_REQUIRED)
		}
	}

	ret, _, err := procSetThreadExecutionState.Call(state)
	if ret == 0 {
		if enabled {
			log.Printf("error: スリープ抑止有効化の失敗: %v\n", err)
			return
		}
		log.Printf("error: スリープ抑止解除の失敗: %v\n", err)
	}
}

func syncWakeTask(schedules []*RecordingSchedule, now time.Time) {
	wakeTask.mux.Lock()
	defer wakeTask.mux.Unlock()

	if len(schedules) == 0 {
		wakeTask.delete()
		return
	}

	wakeTime := time.Unix(schedules[0].StartTime, 0).Add(-(time.Duration(CurrentPowerManagerSetting().WakeBeforeSchedule) * time.Minute))
	if !wakeTime.After(now.Add(time.Minute)) {
		wakeTask.delete()
		return
	}

	if wakeTask.known && wakeTask.wakeTime.Equal(wakeTime) {
		return
	}

	if err := wakeTask.create(wakeTime); err != nil {
		log.Printf("error: スリープ解除タスクの作成に失敗: %v", err)
		return
	}

	wakeTask.wakeTime = wakeTime
	wakeTask.known = true
	log.Printf("info: 次の予約に向けたスリープ解除タスクを設定: %s", wakeTime.Format(time.RFC3339))
}

func (p *wakeTaskManager) create(wakeTime time.Time) error {
	xmlPath, err := writeWakeTaskXML(wakeTime)
	if err != nil {
		return err
	}
	defer os.Remove(xmlPath)

	cmd := exec.CommandContext(serviceContext(), "schtasks.exe", "/Create", "/TN", CurrentPowerManagerSetting().WakeTaskName, "/XML", xmlPath, "/F")
	configureBackgroundCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return commandError(err, out)
	}
	p.synced = true
	return nil
}

func (p *wakeTaskManager) delete() {
	if !p.known && p.synced {
		return
	}

	cmd := exec.CommandContext(serviceContext(), "schtasks.exe", "/Delete", "/TN", CurrentPowerManagerSetting().WakeTaskName, "/F")
	configureBackgroundCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil && p.known {
		log.Printf("error: スリープ解除タスクの削除に失敗: %v", commandError(err, out))
		return
	}
	if err != nil {
		p.wakeTime = time.Time{}
		p.known = false
		p.synced = true
		return
	}

	p.wakeTime = time.Time{}
	p.known = false
	p.synced = true
	log.Println("info: スリープ解除タスクを削除")
}

func writeWakeTaskXML(wakeTime time.Time) (string, error) {
	task := taskXML{
		Xmlns:   "http://schemas.microsoft.com/windows/2004/02/mit/task",
		Version: "1.4",
		RegistrationInfo: registrationInfo{
			Date:        Now().Format("2006-01-02T15:04:05"),
			Author:      "tvrockgo",
			Description: "Wake the computer before the next tvrockgo schedule.",
		},
		Triggers: triggers{
			TimeTrigger: timeTrigger{
				StartBoundary: wakeTime.Format("2006-01-02T15:04:05"),
				Enabled:       true,
			},
		},
		Actions: actions{
			Exec: taskExec{
				Command:   wakeTaskCommand(),
				Arguments: "/c exit",
			},
		},
	}
	task.Settings.MultipleInstancesPolicy = "IgnoreNew"
	task.Settings.DisallowStartIfOnBatteries = false
	task.Settings.StopIfGoingOnBatteries = false
	task.Settings.AllowHardTerminate = true
	task.Settings.StartWhenAvailable = true
	task.Settings.RunOnlyIfNetworkAvailable = false
	task.Settings.IdleSettings.StopOnIdleEnd = false
	task.Settings.IdleSettings.RestartOnIdle = false
	task.Settings.AllowStartOnDemand = true
	task.Settings.Enabled = true
	task.Settings.Hidden = true
	task.Settings.WakeToRun = true
	task.Settings.ExecutionTimeLimit = "PT5M"
	task.Settings.Priority = 7

	buf, err := xml.MarshalIndent(task, "", "  ")
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp("", "tvrockgo-wake-task-*.xml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	xmlText := `<?xml version="1.0" encoding="UTF-16"?>` + "\r\n" + string(buf)
	if _, err := f.Write(utf16LEBytes(xmlText)); err != nil {
		return "", err
	}

	return f.Name(), nil
}

func utf16LEBytes(s string) []byte {
	encoded := utf16.Encode([]rune(s))
	buf := make([]byte, 2, len(encoded)*2+2)
	buf[0] = 0xff
	buf[1] = 0xfe
	for _, r := range encoded {
		buf = append(buf, byte(r), byte(r>>8))
	}
	return buf
}

func wakeTaskCommand() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", "cmd.exe")
}

func commandError(err error, out []byte) error {
	if len(out) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
}

func syncSleepTimer(now time.Time, busyCount int) {
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
	setSleepPrevention(false)

	ret, _, err := procSetSuspendState.Call(0, 0, 0)
	if ret == 0 {
		log.Printf("error: スリープへの移行に失敗: %v", err)
		return false
	}
	return true
}

func updatePowerState(reason string) {
	nowTime := Now()
	now := nowTime.Unix()

	sch, err := scheduler.GetNextSchedules()
	if err != nil {
		log.Println("error: ", err)
		setSleepPrevention(true)
		log.Printf("debug: スリープ抑止: %s", reason)
		return
	}

	syncWakeTask(sch, nowTime)
	busyCount := customCommandManager.GetBusyCount()
	syncSleepTimer(nowTime, busyCount)

	log.Printf("debug: sch=%t, busyCount=%d, isSuppressingAfterWake=%t\n",
		(len(sch) > 0 && sch[0].StartTime < now+CurrentPowerManagerSetting().PreventSleepTime),
		busyCount,
		isSuppressingAfterWake(nowTime))

	shouldPreventSleep := (len(sch) > 0 && sch[0].StartTime < now+CurrentPowerManagerSetting().PreventSleepTime) ||
		(CurrentPowerManagerSetting().PreventIfDeviceRunning && slot.IsAnyRunning()) ||
		playbackActive() || tsAnalysisActive() ||
		busyCount > 0 || // 実行中のコマンドがあれば抑止
		isSuppressingAfterWake(nowTime)

	if shouldPreventSleep {
		// スリープ抑止を有効化
		setSleepPrevention(true)
		log.Printf("debug: スリープ抑止: %s", reason)
		return
	}

	setSleepPrevention(false)
	log.Printf("debug: スリープ抑止解除")
}

// ウィンドウプロシージャ（イベントハンドラ）
func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == WM_POWERBROADCAST {
		switch wParam {
		case PBT_APMSUSPEND:
			log.Println("info: システムがスタンバイ/スリープに移行")
			notifySleepEvent(EventSuspending)
		case PBT_APMRESUMESUSPEND:
			log.Println("info: スリープからユーザー操作で復帰")
			suppressSleepAfterWake()
			notifySleepEvent(EventResumed)
		case PBT_APMRESUMEAUTOMATIC:
			log.Println("info: スリープから自動復帰")
			suppressSleepAfterWake()
			notifySleepEvent(EventResumed)
		}
		return 1 // TRUE
	}

	ret, _, _ := procDefWindowProcW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)

	return ret
}

// スリープ監視スレッド
func sleepMonitor(ctx context.Context, eventChan chan<- SleepEvent) {
	// ウィンドウの作成、メッセージのディスパッチ、および破棄は、同一のOSスレッドで行う必要がある。
	// このスレッドは、トレイのメッセージポンプとは独立している。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	globalEventChan = eventChan

	threadId, _, _ := procGetCurrentThreadId.Call()
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("SleepMonitorClass")

	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     syscall.Handle(hInstance),
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, 0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		log.Println("error: ウィンドウの作成に失敗しました")
		return
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), hInstance)
	defer user32.NewProc("DestroyWindow").Call(hwnd)

	log.Println("info: スリープ監視スレッドの開始")

	go func() {
		<-ctx.Done()
		procPostThreadMessageW.Call(threadId, WM_QUIT, 0, 0)
	}()

	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	log.Println("info: スリープ監視スレッドの終了")
}

// スリープ抑止スレッド
func preventSleep(ctx context.Context, reason string) {
	// SetThreadExecutionState は、それを呼び出すスレッドに対して適用されます。
	// どの起動モードにおいても、すべての更新処理および最終的な ES_CONTINUOUS のリセットは
	// 必ず同一スレッド上で行うようにしてください。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

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

	setSleepPrevention(false)
	log.Println("info: スリープ抑止スレッドの終了")
}

func InitializePowerManager() error {

	if !CurrentPowerManagerSetting().PowerManager {
		return nil
	}

	eventChan := make(chan SleepEvent, 10)
	wakeUpSuppressUntil.Store(0)

	serviceWorkers.start(func() { sleepMonitor(globalContext, eventChan) })

	serviceWorkers.start(func() { preventSleep(globalContext, "バックグラウンドタスク実行中") })

	return nil
}
