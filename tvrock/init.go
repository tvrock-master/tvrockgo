package tvrock

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/comail/colog"
)

var globalContext context.Context
var initializeOnce sync.Once
var initializeError error

// init()では多重起動のチェックを通す前に実行されしまうのでmain側から直接呼び出す形に変更
func Initialize(ctx context.Context) error {
	initializeOnce.Do(func() {
		globalContext, globalCancel = context.WithCancel(ctx)
		initializeError = initialize()
	})
	return initializeError
}

// init()
func initialize() error {
	if err := configure.Load(TvRockConfigureFile); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		configureOriginal := TvRockConf{}
		if err := configureOriginal.Load(TvRockConfigureOriginalFile); err != nil {
			return err
		}
		configureOriginal.BuildHash()

		if err := configureOriginal.Save(TvRockConfigureFile); err != nil {
			return err
		}

		if err := configure.Load(TvRockConfigureFile); err != nil {
			return err
		}
	}

	if !configure.Debug {
		colog.SetMinLevel(colog.LInfo)
	}

	if configure.BuildHash() {
		if err := configure.Save(TvRockConfigureFile); err != nil {
			return err
		}
	}

	if err := configure.ensureLauncherToken(TvRockConfigureFile); err != nil {
		return err
	}

	normalizeRecorderSetting(&configure.RecorderSetting)

	if err := InitializeCron(); err != nil {
		return err
	}

	if err := InitializeCustomCommandManager(); err != nil {
		return err
	}

	if err := rockDB.Initialize(configure.DBFile,
		configure.ProgramDBFile,
		configure.CaptionDBFile,
		configure.PastDBFile,
		configure.ArchiveDBFile,
		configure.CaptionArchiveDBFile,
	); err != nil {
		return err
	}

	if err := launcher.Initialize(); err != nil {
		return err
	}

	if err := deviceController.Initialize(&launcher); err != nil {
		return err
	}

	if err := slot.Initialize(configure.Devices, configure.RecordingDevice.DeviceAssistance); err != nil {
		return err
	}

	if err := station.Initialize(&rockDB); err != nil {
		return err
	}

	if err := tvprogram.Initialize(&rockDB); err != nil {
		return err
	}

	if err := tasks.Initialize(&launcher); err != nil {
		return err
	}

	if err := scheduler.Initialize(&rockDB, &tvprogram, &launcher); err != nil {
		return err
	}

	if err := InitializePowerManager(); err != nil {
		return err
	}

	if err := InitializeSearch(); err != nil {
		return err
	}

	if err := InitializeStorageInfo(); err != nil {
		return err
	}

	if err := InitializeFetchProgramScheduler(); err != nil {
		return err
	}

	if err := InitializeArchiver(); err != nil {
		return err
	}

	return nil
}

func LaunchTerminal() {
	if configure.BootLauncher {
		serviceWorkers.start(func() { launcher.ExecLauncher(configure.Host, configure.LauncherHostID) })
		serviceWorkers.start(func() { remoteWatching.run(serviceContext()) })
	}

	if configure.BootScheduler {
		serviceWorkers.start(scheduler.ExecScheduler)
	}

	serviceWorkers.start(func() { runTSAnalysisWorker(serviceContext()) })
}

func Close() {
	shutdownOnce.Do(func() {
		playback.Lock()
		for _, session := range playback.sessions {
			session.cancel()
		}
		playback.Unlock()
		serviceWorkers.stop()
		if globalCancel != nil {
			globalCancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		log.Println("info: WEBサービスを停止")
		if err := ShutdownService(ctx); err != nil {
			log.Println("error: WEBサービスの終了処理でエラーが発生しました", err)
		}
		log.Println("info: 定期処理の終了を待機")
		if cronManager != nil {
			cronManager.Close()
		}
		log.Println("info: バックグラウンド処理の終了を待機")
		serviceWorkers.wait()
		log.Println("info: 検索インデックスを閉じます")
		if err := bleveProgram.Close(); err != nil {
			log.Println("error: bleve index close:", err)
		}
		log.Println("info: データベースを閉じます")
		rockDB.CloseDB()
		log.Println("info: データベースの終了処理が完了")
	})
}
