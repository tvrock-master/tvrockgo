package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/comail/colog"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/tvrock-master/tvrockgo/tvrock"
)

type startupOptions struct {
	console bool
	dataDir string
	rockbar string
	command []string
}

func parseOptions(args []string, output io.Writer) (startupOptions, error) {
	var opts startupOptions
	flags := flag.NewFlagSet("tvrockgo", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.BoolVar(&opts.console, "console", false, "コンソールで起動する")
	flags.StringVar(&opts.dataDir, "data-dir", "", "設定・DB・ログの基準ディレクトリ（既定: 実行ファイルの場所）")
	flags.StringVar(&opts.rockbar, "rockbar", "", "Windows版Rockbarの実行ファイル（省略時は自動検出、見つからなければブラウザ版）")
	flags.Func("timemachine", "旧TvRockの番組データを`パス`から取り込み、完了後に終了する", func(path string) error {
		if path == "" {
			return fmt.Errorf("--timemachineには取り込み元のパスを指定してください")
		}
		opts.command = []string{"--timemachine", path}
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("不要な引数です: %q。番組データの取り込みには --timemachine <パス> を指定してください", flags.Args())
	}
	return opts, nil
}

func logInit(console bool) (io.Closer, error) {
	if err := os.MkdirAll("log", 0o755); err != nil {
		return nil, err
	}
	rl, err := rotatelogs.New("./log/%Y%m%d%H%M.log")
	if err != nil {
		return nil, err
	}
	colog.SetDefaultLevel(colog.LDebug)
	colog.SetMinLevel(colog.LTrace)
	colog.SetFormatter(&colog.StdFormatter{Colors: console, Flag: log.Ldate | log.Ltime | log.Lshortfile})
	colog.Register()
	if console || serviceStandardOutput() {
		colog.SetOutput(io.MultiWriter(rl, os.Stdout))
	} else {
		colog.SetOutput(rl)
	}
	return rl, nil
}

func main() {
	// 設定やデータベースに触れる前に解析を行う。GUIビルドであってもヘルプを表示するためにコンソールが必要となる
	opts, err := parseOptions(os.Args[1:], io.Discard)
	console := opts.console || len(opts.command) != 0 || err != nil
	if setupErr := prepareConsole(console); setupErr != nil {
		reportStartupError(setupErr, console)
		os.Exit(1)
	}
	if err != nil {
		_, _ = parseOptions(os.Args[1:], os.Stderr)
		if err == flag.ErrHelp {
			return
		}
		reportStartupError(err, console)
		os.Exit(2)
	}
	if err := run(opts); err != nil {
		reportStartupError(err, console)
		os.Exit(1)
	}
}

func run(opts startupOptions) (runErr error) {
	// 作業ディレクトリを切り替える前にユーザーから提供されたパスを解決する
	if opts.rockbar != "" {
		absolute, err := filepath.Abs(opts.rockbar)
		if err != nil {
			return err
		}
		opts.rockbar = absolute
	}
	if len(opts.command) != 0 {
		absolute, err := filepath.Abs(opts.command[1])
		if err != nil {
			return err
		}
		opts.command[1] = absolute
	}
	dataDir := opts.dataDir
	if dataDir == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		dataDir = filepath.Dir(exe)
	}
	if err := os.Chdir(dataDir); err != nil {
		return err
	}
	logger, err := logInit(opts.console || len(opts.command) != 0)
	if err != nil {
		return err
	}
	defer logger.Close()
	defer func() {
		if runErr != nil {
			log.Printf("error: 起動・実行処理: %v", runErr)
		}
	}()
	if !acquireSingleInstanceLock() {
		return fmt.Errorf("TvRockGOは既に起動しているか、起動ロックを取得できません。ログを確認してください")
	}
	defer releaseSingleInstanceLock()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if opts.console || len(opts.command) != 0 {
		stopped := make(chan struct{})
		cleanup, err := installPlatformStop(cancel, stopped)
		if err != nil {
			return err
		}
		defer cleanup()
		defer close(stopped)
	}
	if len(opts.command) != 0 {
		return runCommand(ctx, opts.command)
	}
	shutdown := newShutdownHandler()
	defer shutdown(cancel)
	if err := tvrock.Initialize(ctx); err != nil {
		return err
	}

	ready := make(chan struct{})
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- tvrock.RunService(func() { close(ready) })
		cancel()
	}()
	select {
	case <-ready:
		tvrock.LaunchTerminal()
	case err := <-serverResult:
		return err
	case <-ctx.Done():
		shutdown(cancel)
		return <-serverResult
	}
	if opts.console {
		<-ctx.Done()
	} else if err := runPlatform(ctx, cancel, opts); err != nil {
		shutdown(cancel)
		<-serverResult
		return err
	}
	shutdown(cancel)
	err = <-serverResult
	log.Println("info: tvrockgoを終了")
	return err
}

func newShutdownHandler() func(cancel context.CancelFunc) {
	var once sync.Once
	return func(cancel context.CancelFunc) {
		once.Do(func() {
			cancel()
			tvrock.Close()
			log.Println("info: exit tvrockgo service")
		})
	}
}

func runCommand(ctx context.Context, args []string) error {
	log.Printf("info: execute scan timemachine [%s]", args[1])
	err := tvrock.ImportTimeMachine(ctx, args[1])
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		log.Println("info: 番組データの取り込みを中断しました")
		return nil
	}
	return err
}
