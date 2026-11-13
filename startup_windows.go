//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"unsafe"

	"github.com/tvrock-master/tvrockgo/tvrock"
	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	shell32         = windows.NewLazySystemDLL("shell32.dll")
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	registerClass   = user32.NewProc("RegisterClassExW")
	createWindow    = user32.NewProc("CreateWindowExW")
	defWindowProc   = user32.NewProc("DefWindowProcW")
	destroyWindow   = user32.NewProc("DestroyWindow")
	getMessage      = user32.NewProc("GetMessageW")
	dispatchMessage = user32.NewProc("DispatchMessageW")
	postMessage     = user32.NewProc("PostMessageW")
	notifyIcon      = shell32.NewProc("Shell_NotifyIconW")
)

// 初期化処理およびトレイのメッセージポンプは最初のWindowsスレッド上で実行し続けること。
// 電源管理については tvrock/powermanager_windows.go 内で独自のロックされたスレッドが維持される。
func init() { runtime.LockOSThread() }

func normalizeConsoleInputMode(mode uint32) uint32 {
	// LINE_INPUT が指定されていない場合 ECHO_INPUT は何の効果も持たず SetConsoleMode はこれを拒否します。
	if mode&windows.ENABLE_LINE_INPUT == 0 {
		mode &^= windows.ENABLE_ECHO_INPUT
	}
	return mode
}

func installPlatformStop(cancel context.CancelFunc, stopped <-chan struct{}) (func(), error) {
	// PowerShellではGUI形式の実行ファイル（EXE）を起動した後、共有入力バッファが「RAWモード」のまま残ってしまうことがある。
	// 標準入力がパイプ経由でリダイレクトされている可能性があるためCONIN$を明示的に開く必要がある。
	input, err := windows.CreateFile(windows.StringToUTF16Ptr("CONIN$"), windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("コンソール入力を開く: %w", err)
	}
	var inputMode uint32
	if err := windows.GetConsoleMode(input, &inputMode); err != nil {
		windows.CloseHandle(input)
		return nil, fmt.Errorf("コンソール入力モード取得: %w", err)
	}
	restoreInput := func() {
		windows.SetConsoleMode(input, normalizeConsoleInputMode(inputMode))
		windows.CloseHandle(input)
	}
	// AttachConsoleやAllocConsoleを呼び出すとmain関数が実行される前にGoランタイムが設定したハンドラがリセットされてしまう。
	// そのためアタッチ後に改めてハンドラを登録し、Ctrl+Cで適切に終了できるようにしておく。
	callback := windows.NewCallback(func(event uint32) uintptr {
		switch event {
		case 0, 1: // CTRL_C_EVENT, CTRL_BREAK_EVENT
			cancel()
			return 1
		case 2, 5, 6: // close, logoff, shutdown: Windows waits for the handler
			cancel()
			<-stopped
			return 1
		}
		return 0
	})
	proc := kernel32.NewProc("SetConsoleCtrlHandler")
	if ok, _, err := proc.Call(callback, 1); ok == 0 {
		restoreInput()
		return nil, fmt.Errorf("終了ハンドラー登録: %w", err)
	}
	cleanup := func() {
		proc.Call(callback, 0)
		restoreInput()
	}
	processedMode := normalizeConsoleInputMode(inputMode | windows.ENABLE_PROCESSED_INPUT)
	if err := windows.SetConsoleMode(input, processedMode); err != nil {
		cleanup()
		return nil, fmt.Errorf("Ctrl+C入力の有効化: %w", err)
	}
	// CREATE_NEW_PROCESS_GROUP を指定した場合、親プロセスは Ctrl+C を個別に無効化できます
	if ok, _, err := proc.Call(0, 0); ok == 0 {
		cleanup()
		return nil, fmt.Errorf("Ctrl+C通知の有効化: %w", err)
	}
	return cleanup, nil
}

func prepareConsole(enabled bool) error {
	if !enabled {
		// 開発用コンソールビルドを実行する際、親のターミナルを隠さないようにする。
		kernel32.NewProc("FreeConsole").Call()
		return nil
	}

	attached, _, attachErr := kernel32.NewProc("AttachConsole").Call(0xffffffff)
	if attached == 0 && attachErr != windows.ERROR_ACCESS_DENIED {
		allocated, _, err := kernel32.NewProc("AllocConsole").Call()
		if allocated == 0 {
			return fmt.Errorf("コンソール作成: %w", err)
		}
	}

	// パイプやリダイレクトされたファイルは維持し、欠落している標準ハンドルのみを置き換える。
	for _, stream := range []struct {
		id     uint32
		name   string
		target **os.File
		flag   int
	}{
		{windows.STD_INPUT_HANDLE, "CONIN$", &os.Stdin, os.O_RDONLY},
		{windows.STD_OUTPUT_HANDLE, "CONOUT$", &os.Stdout, os.O_WRONLY},
		{windows.STD_ERROR_HANDLE, "CONOUT$", &os.Stderr, os.O_WRONLY},
	} {
		handle, err := windows.GetStdHandle(stream.id)
		if err == nil && handle != 0 && handle != windows.InvalidHandle {
			if _, err := windows.GetFileType(handle); err == nil {
				*stream.target = os.NewFile(uintptr(handle), stream.name)
				continue
			}
		}
		file, err := os.OpenFile(stream.name, stream.flag, 0)
		if err != nil {
			return err
		}
		*stream.target = file
		if err := windows.SetStdHandle(stream.id, windows.Handle(file.Fd())); err != nil {
			return err
		}
	}
	return nil
}

func serviceStandardOutput() bool { return false }
func reportStartupError(err error, console bool) {
	if console {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	showTrayError(err)
}
func showTrayError(err error) {
	log.Printf("error: %v", err)
	text, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString("TvRockGO")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}

func confirmTrayExit(hwnd uintptr) bool {
	text, _ := windows.UTF16PtrFromString("TvRockGOを終了しますか？")
	title, _ := windows.UTF16PtrFromString("TvRockGO")
	// MB_YESNO | MB_ICONQUESTION | MB_DEFBUTTON2 | MB_SETFOREGROUND | MB_TOPMOST
	// デフォルトを「いいえ」とし、ダイアログを通知アイコンパネルよりも前面に表示する
	result, _, _ := user32.NewProc("MessageBoxW").Call(hwnd, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x4|0x20|0x100|0x10000|0x40000)
	return result == 6 // IDYES、「いいえ」、Escキー、あるいはダイアログのエラーによって終了してはならない
}

// これらのレイアウトは、Windows SDKのWNDCLASSEXW、MSG、およびNOTIFYICONDATAWに準拠しています。
// uintptr型のフィールドを使用することで、Goは各アーキテクチャにおいて適切なポインタアライメントを適用できるようになります。
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type winPoint struct{ X, Y int32 }
type winMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          winPoint
	Private        uint32
}
type trayIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

const (
	trayCallback      = 0x8001
	wmClose           = 0x0010
	wmDestroy         = 0x0002
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmContextMenu     = 0x007b
	wmTimer           = 0x0113
	trayStateTimer    = 1
	traySleepTimer    = 4
	ninSelect         = 0x0400
	ninKeySelect      = 0x0401
	wmLeftButtonUp    = 0x0202
	wmRightButtonUp   = 0x0205
)

// バージョン4では、マウスメッセージに加えて選択通知も送信します。
// 1回のクリックで2回切り替わってしまうことがないよう、イベントの系統を1つに絞って処理します。
func trayActivatesSleepTimer(event uint32, version4 bool) bool {
	if version4 {
		return event == ninSelect || event == ninKeySelect
	}
	return event == wmLeftButtonUp
}

func trayOpensMenu(event uint32, version4 bool) bool {
	if version4 {
		return event == wmContextMenu
	}
	return event == wmRightButtonUp
}

type trayMenuItem struct {
	ID      uintptr
	Label   string
	Checked bool
}

func sleepTimerActive(setting tvrock.PowerManagerSetting) bool {
	return setting.PowerManager && !setting.SleepTimerPause
}

func trayMenuItems(setting tvrock.PowerManagerSetting) []trayMenuItem {
	items := []trayMenuItem{{ID: 1, Label: "番組表を開く"}, {ID: 2, Label: "Rockbarを開く"}}
	if setting.PowerManager {
		items = append(items, trayMenuItem{ID: traySleepTimer, Label: "スリープタイマー", Checked: sleepTimerActive(setting)})
	}
	return append(items, trayMenuItem{ID: 3, Label: "終了"})
}

func runPlatform(ctx context.Context, cancel context.CancelFunc, opts startupOptions) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ctx.Err() != nil {
		return nil
	}
	baseURL, err := localWebURL(tvrock.LocalWebHost())
	if err != nil {
		return err
	}
	icon, err := createTrayIcon()
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(icon)
	sleepIcon, err := createTrayIconFromPNG(traySleepPNG)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(sleepIcon)
	className, _ := windows.UTF16PtrFromString("TvRockGO.Tray")
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	taskbarName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	taskbarCreated, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(taskbarName)))
	var data trayIconData
	var stopping bool
	var iconVersion4 bool
	var menuOpen bool
	selectedIcon := func() uintptr {
		if sleepTimerActive(tvrock.CurrentPowerManagerSetting()) {
			return sleepIcon
		}
		return icon
	}
	updateIcon := func() {
		if stopping || data.Window == 0 {
			return
		}
		desired := selectedIcon()
		if data.Icon == desired {
			return
		}
		previous := data.Icon
		data.Icon = desired
		if ok, _, _ := notifyIcon.Call(1, uintptr(unsafe.Pointer(&data))); ok == 0 {
			data.Icon = previous // 次のtimer tickで再試行する
		}
	}
	toggleSleepTimer := func() {
		if stopping || ctx.Err() != nil {
			return
		}
		if _, err := tvrock.ToggleSleepTimerPause(); err != nil {
			showTrayError(err)
		}
		updateIcon()
	}
	addIcon := func() bool {
		data.Icon = selectedIcon()
		ok, _, _ := notifyIcon.Call(0, uintptr(unsafe.Pointer(&data)))
		if ok != 0 {
			data.Version = 4
			versionOK, _, _ := notifyIcon.Call(4, uintptr(unsafe.Pointer(&data)))
			iconVersion4 = versionOK != 0
		}
		return ok != 0
	}
	callback := windows.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch {
		case msg == wmTimer && wp == trayStateTimer:
			updateIcon()
			return 0
		case taskbarCreated != 0 && msg == uint32(taskbarCreated):
			if !stopping {
				addIcon()
			}
			return 0
		case msg == trayCallback:
			event := uint32(lp & 0xffff)
			if trayActivatesSleepTimer(event, iconVersion4) {
				toggleSleepTimer()
				return 0
			}
			if trayOpensMenu(event, iconVersion4) && !menuOpen {
				// TrackPopupMenuEx はネストされたメッセージループを実行するため、再入による要求は無視する
				menuOpen = true
				command := trayMenu(hwnd)
				menuOpen = false
				switch command {
				case 1:
					if err := openExternal(baseURL + "/static/index.html"); err != nil {
						showTrayError(err)
					}
				case 2:
					if err := openRockbar(opts.rockbar, baseURL+"/static/rockbar/"); err != nil {
						showTrayError(err)
					}
				case 3:
					if confirmTrayExit(hwnd) {
						cancel()
					}
				case traySleepTimer:
					toggleSleepTimer()
				}
			}
			return 0
		case msg == wmQueryEndSession:
			return 1
		case msg == wmEndSession && wp != 0:
			cancel()
			return 0
		case msg == wmClose:
			stopping = true
			cancel()
			destroyWindow.Call(hwnd)
			return 0
		case msg == wmDestroy:
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wp, lp)
		return result
	})
	class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: callback, Instance: instance, ClassName: className}
	if atom, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return fmt.Errorf("トレイクラス登録: %w", err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)

	// 非表示のトップレベルウィンドウは、エクスプローラーの再起動やセッション終了のブロードキャストを受け取る。
	// WS_EX_TOOLWINDOW | WS_EX_TOPMOST、および WS_POPUP を指定する。最前面（topmost）のオーナーウィンドウを持つことで、
	// そのメニューをエクスプローラーのタスクバーや通知アイコンのオーバーフローウィンドウよりも前面に表示し続けることができる。
	hwnd, _, err := createWindow.Call(0x88, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0x80000000, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("トレイウィンドウ作成: %w", err)
	}
	defer destroyWindow.Call(hwnd)

	// NIF_SHOWTIP を指定すると、NOTIFYICON_VERSION_4 においても標準のツールチップが有効なままになります。
	data = trayIconData{Size: uint32(unsafe.Sizeof(trayIconData{})), Window: hwnd, ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: trayCallback, Icon: icon}
	copy(data.Tip[:], windows.StringToUTF16("TvRockGO"))
	if !addIcon() {
		return fmt.Errorf("トレイアイコンを登録できません")
	}
	defer notifyIcon.Call(2, uintptr(unsafe.Pointer(&data)))

	// ネイティブのタイマーディスパッチを利用することで、アイコンの変更処理をトレイのOSスレッド上で維持する。
	if id, _, err := user32.NewProc("SetTimer").Call(hwnd, trayStateTimer, 1000, 0); id == 0 {
		return fmt.Errorf("トレイ状態監視タイマーの作成: %w", err)
	}
	defer user32.NewProc("KillTimer").Call(hwnd, trayStateTimer)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			postMessage.Call(hwnd, wmClose, 0, 0)
		case <-done:
		}
	}()
	var message winMessage
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("トレイメッセージ取得: %w", err)
		}
		if result == 0 {
			return nil
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func trayMenu(hwnd uintptr) uintptr {
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return 0
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	for _, item := range trayMenuItems(tvrock.CurrentPowerManagerSetting()) {
		text, _ := windows.UTF16PtrFromString(item.Label)
		flags := uintptr(0)
		if item.Checked {
			flags = 0x8
		} // MF_CHECKED: native checkmark.
		user32.NewProc("AppendMenuW").Call(menu, flags, item.ID, uintptr(unsafe.Pointer(text)))
	}
	var point winPoint
	if ok, _, _ := user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return 0
	}
	// SetForegroundWindowがアクティブ化できるように、サイズゼロのオーナーウィンドウのみを表示する。
	// SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE | SWP_SHOWWINDOW
	user32.NewProc("SetWindowPos").Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x53)
	defer user32.NewProc("ShowWindow").Call(hwnd, 0)
	if ok, _, _ := user32.NewProc("SetForegroundWindow").Call(hwnd); ok == 0 {
		log.Println("warn: トレイメニューの所有ウィンドウを前面にできませんでした")
	}
	// 前面化に伴うExplorerの通知領域の処理後に、topmostの順序を再設定する。
	// WS_EX_TOPMOSTが既に付いていても、そのグループ内の最前面とは限らない。
	user32.NewProc("SetWindowPos").Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x13)
	// TPM_RETURNCMD | TPM_RIGHTBUTTON | TPM_RIGHTALIGN | TPM_BOTTOMALIGN.
	// アイコンの矩形で位置を上書きしたり、除外領域でメニューを押し出したりせず、カーソルを基準にする。
	selected, _, _ := user32.NewProc("TrackPopupMenuEx").Call(menu, 0x100|0x2|0x8|0x20, uintptr(point.X), uintptr(point.Y), hwnd, 0)
	postMessage.Call(hwnd, 0, 0, 0)
	notifyIcon.Call(3, uintptr(unsafe.Pointer(&trayIconData{Size: uint32(unsafe.Sizeof(trayIconData{})), Window: hwnd, ID: 1})))
	return selected
}

func openExternal(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	result, _, _ := shell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("ブラウザを開けません（コード %d）", result)
	}
	return nil
}
