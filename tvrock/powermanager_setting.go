package tvrock

import "sync"

var powerManagerUpdateMu sync.Mutex

// CurrentPowerManagerSettingは、API、パワーワーカー、およびWindowsトレイ間で共有されるスナップショットを返す。
// ライブ状態の設定を他のGoroutineに直接公開してはならない。
func CurrentPowerManagerSetting() PowerManagerSetting {
	configure.mux.RLock()
	defer configure.mux.RUnlock()
	return configure.PowerManagerSetting
}

func updatePowerManagerSetting(update func(*PowerManagerSetting)) (PowerManagerSetting, error) {
	powerManagerUpdateMu.Lock()
	defer powerManagerUpdateMu.Unlock()
	previous := CurrentPowerManagerSetting()
	setting := previous
	update(&setting)
	if setting == previous {
		return setting, nil
	}
	configure.mux.Lock()
	configure.PowerManagerSetting = setting
	if err := configure.saveLocked(TvRockConfigureFile); err != nil {
		configure.PowerManagerSetting = previous
		configure.mux.Unlock()
		return previous, err
	}
	configure.mux.Unlock()
	deviceController.Notify(ControllerUpdatePowerManagerSetting, "")
	return setting, nil
}

// ToggleSleepTimerPauseはトレイからも利用されるため、保存処理やクライアントへの通知は
// Webアプリケーションからの更新時と同様のルールに従います。
func ToggleSleepTimerPause() (PowerManagerSetting, error) {
	return updatePowerManagerSetting(func(setting *PowerManagerSetting) {
		if setting.PowerManager {
			setting.SleepTimerPause = !setting.SleepTimerPause
		}
	})
}
