package tvrock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("TVROCK_COMMAND_TEST_HELPER") == "1" {
		os.Exit(runCustomCommandHelper())
	}
	os.Exit(runTests(m))
}

// テストにおいて、ユーザー設定の読み込み、サービスの開始、あるいは記録用データベースの変更を行ってはならない。
// 単体テストで使用するストアのみを初期化すること。
func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "tvrock-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	globalContext = context.Background()
	configure = TvRockConf{
		RecorderSetting:         RecorderSetting{FilenameEx: "@TT", RecordFileExtension: ".ts"},
		TaskSetting:             TaskSetting{MaxTasks: 16, ExecTime: 60},
		SearchMaxWorkerThreads:  2,
		SearchLimit:             1000,
		CurrentProgramThreshold: 86400,
		SqliteSetting:           []string{"_pragma=busy_timeout(5000)"},
	}
	normalizeFeatures(&configure)
	if err := rockDB.Initialize(
		filepath.Join(dir, "data.db"), filepath.Join(dir, "program.db"), filepath.Join(dir, "caption.db"),
		filepath.Join(dir, "past.db"), filepath.Join(dir, "archive.db"), filepath.Join(dir, "caption-archive.db"),
	); err != nil {
		rockDB.CloseDB()
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer rockDB.CloseDB()
	for _, initialize := range []func() error{
		launcher.Initialize,
		func() error { return deviceController.Initialize(&launcher) },
		func() error { return slot.Initialize(nil, nil) },
		func() error { return station.Initialize(&rockDB) },
		func() error { return tvprogram.Initialize(&rockDB) },
		func() error { return tasks.Initialize(&launcher) },
		func() error { return scheduler.Initialize(&rockDB, &tvprogram, &launcher) },
	} {
		if err := initialize(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}
