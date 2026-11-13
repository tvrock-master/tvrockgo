package tvrock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"
)

func TestCustomCommandValidation(t *testing.T) {
	for _, cc := range []CustomCommand{
		{AppPath: "tool.exe"}, {Type: "executable", AppPath: "tool.exe", Options: []string{"--fixed", "日本語 & text"}},
		{Type: "batch", AppPath: "C:/Tools & Samples/Touch.cmd", Options: []string{"Touch", "two words"}},
		{Type: "powershell", AppPath: "Touch.ps1", Options: []string{"-Mode", "Touch"}},
	} {
		if err := cc.validate(); err != nil {
			t.Errorf("valid command %#v: %v", cc, err)
		}
	}
	for _, cc := range []CustomCommand{
		{}, {Type: "unknown", AppPath: "tool.exe"}, {AppPath: "Touch.cmd"}, {AppPath: "Touch.ps1"},
		{Type: "batch", AppPath: "tool.exe"}, {Type: "powershell", AppPath: "tool.exe"},
		{Type: "batch", AppPath: "bad%PATH%.cmd"}, {Type: "batch", AppPath: "ok.cmd", Options: []string{"x&echo injected"}},
		{Type: "batch", AppPath: "ok.cmd", Options: []string{"%PATH%"}},
		{Type: "batch", AppPath: "ok.cmd", Options: []string{"!PATH!"}},
		{Type: "batch", AppPath: "ok.cmd", Options: []string{"\"quoted\""}},
	} {
		if err := cc.validate(); err == nil {
			t.Errorf("unsafe command accepted: %#v", cc)
		}
	}
	for _, token := range FilenameExCommands {
		for _, kind := range []string{"executable", "batch", "powershell"} {
			ext := map[string]string{"executable": ".exe", "batch": ".cmd", "powershell": ".ps1"}[kind]
			cc := CustomCommand{Type: kind, AppPath: "command" + ext, Options: []string{"prefix" + token + "suffix"}}
			if err := cc.validate(); err == nil || !strings.Contains(err.Error(), token) {
				t.Errorf("%s accepts token %s: %v", kind, token, err)
			}
			cc.Options, cc.AppPath = nil, token+ext
			if err := cc.validate(); err == nil {
				t.Errorf("%s accepts token in path: %s", kind, token)
			}
		}
	}
}

func TestCustomCommandContextMapping(t *testing.T) {
	start := time.Date(2026, 1, 1, 1, 2, 3, 0, time.Local)
	prg := &TvProgram{Sid: 0x7fff0001, Eid: 54321, Start: start.Unix(), End: start.Add(time.Hour).Unix(), Title: "DB title", Subtitle: "副題 & <tag>"}
	if err := tvprogram.Insert(CurrentSlot, prg); err != nil {
		t.Fatal(err)
	}
	defer rockDB.GetProgram(CurrentSlot).Exec("DELETE FROM program WHERE sid=? AND eid=?", prg.Sid, prg.Eid)
	station.ModifyStation(&TvStation{Sid: prg.Sid, Name: "局名"})
	defer func() { station.Lock(); delete(station.Cache, prg.Sid); station.Unlock() }()
	file := filepath.Join(t.TempDir(), "番組.ts")
	before := Now()
	data, err := buildCustomCommandContext("予約のタイトル", file, prg.Sid, prg.Eid, 7, "one,two", "station")
	if err != nil {
		t.Fatal(err)
	}
	after := Now()
	if data.Title != "予約のタイトル" || data.Subtitle != prg.Subtitle || data.StationName != "局名" || data.DurationMinutes != 60 {
		t.Fatalf("program data: %+v", data)
	}
	if data.RecordFile != file || data.RecordFileName != "番組.ts" || data.RecordDirectory != filepath.Dir(file)+string(os.PathSeparator) {
		t.Fatalf("file data: %+v", data)
	}
	if data.ErrorCount != 7 || data.KeywordHashtags != "#one #two" || data.StationHashtags != "#station" {
		t.Fatalf("tags/errors: %+v", data)
	}
	if data.Start.Hour != "01" || data.Start.Minute != "02" || data.Start.Second != "03" || data.Start.Weekday != "木" || data.Start.WeekdayEnglish != "Thursday" {
		t.Fatalf("start: %+v", data.Start)
	}
	if data.Start28.Hour != "25" || data.Start28.Year != "2025" || data.Start28.Month != "12" || data.Start28.Day != "31" || data.Start28.Weekday != "水" {
		t.Fatalf("extended start: %+v", data.Start28)
	}
	if data.End.Hour != "02" || data.End28.Hour != "26" || data.StartTime != "01:02" || data.StartSeconds != "03" || data.EndTime != "02:02" {
		t.Fatalf("end/time data: %+v", data)
	}
	if data.Current != customCommandClock(before, false) && data.Current != customCommandClock(after, false) {
		t.Fatalf("current clock: %+v", data.Current)
	}
	if data.Current28 != customCommandClock(before, true) && data.Current28 != customCommandClock(after, true) {
		t.Fatalf("extended current clock: %+v", data.Current28)
	}
	fallback, err := buildCustomCommandContext("", "", prg.Sid, prg.Eid, 0, "", "")
	if err != nil || fallback.Title != prg.Title {
		t.Fatalf("title fallback: %+v, %v", fallback, err)
	}
	empty, err := buildCustomCommandContext("standalone", "", 0, 0, 0, "", "")
	if err != nil || empty.Title != "standalone" || empty.Start != (CustomCommandClock{}) || empty.DurationMinutes != 0 || empty.RecordFileName != "" || empty.KeywordHashtags != "" {
		t.Fatalf("missing program: %+v, %v", empty, err)
	}
	for _, hour := range []int{0, 3, 4, 23} {
		tm := time.Date(2026, 1, 1, hour, 0, 0, 0, time.Local)
		clock := customCommandClock(tm, true)
		if hour < 4 && clock.Day != "31" || hour >= 4 && clock.Day != "1" {
			t.Fatalf("28-hour boundary %d: %+v", hour, clock)
		}
	}
}

func TestCustomCommandTemplate(t *testing.T) {
	encoded, err := os.ReadFile("../conf/tvrock.org.yml")
	if err != nil {
		t.Fatal(err)
	}
	var conf TvRockConf
	if err := yaml.Unmarshal(encoded, &conf); err != nil {
		t.Fatal(err)
	}
	if len(conf.CustomCommand) != 3 {
		t.Fatalf("custom command examples: %d", len(conf.CustomCommand))
	}
	for _, cc := range conf.CustomCommand {
		if err := cc.validate(); err != nil {
			t.Fatalf("%s: %v", cc.ID, err)
		}
		if _, err := os.Stat(filepath.Join("..", cc.AppPath)); err != nil {
			t.Fatalf("missing %s sample: %v", cc.ID, err)
		}
	}
	if err := conf.StorageCommand.validate(); err != nil {
		t.Fatal(err)
	}
}

type customCommandProbe struct {
	Data CustomCommandContext `json:"data"`
	Path string               `json:"path"`
	Args []string             `json:"args"`
}

// TestMainからDB初期化前に呼ぶ。録画アプリを起動せず、子プロセスの受け取りを調べる。
func runCustomCommandHelper() int {
	path := os.Getenv("TVROCK_CONTEXT_FILE")
	encoded, err := os.ReadFile(path)
	if err != nil {
		return 10
	}
	var data CustomCommandContext
	if json.Unmarshal(encoded, &data) != nil {
		return 11
	}
	args := os.Args
	for i, arg := range args {
		if arg != "--probe" {
			continue
		}
		probe, _ := json.Marshal(customCommandProbe{Data: data, Path: path, Args: args[i+2:]})
		if os.WriteFile(args[i+1], probe, 0600) != nil {
			return 12
		}
		if os.Getenv("TVROCK_COMMAND_TEST_FAIL") == "1" {
			return 7
		}
		if os.Getenv("TVROCK_COMMAND_TEST_WAIT") == "1" {
			time.Sleep(30 * time.Second)
		}
		return 0
	}
	return 13
}

func TestCustomCommandJSONLifecycle(t *testing.T) {
	t.Setenv("TVROCK_COMMAND_TEST_HELPER", "1")
	t.Setenv("TVROCK_CONTEXT_FILE", "must-be-replaced")
	manager := &CustomCommandManager{BusyList: make(map[string]*exec.Cmd)}
	app, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			probeFile := filepath.Join(t.TempDir(), "probe.json")
			t.Setenv("TVROCK_COMMAND_TEST_FAIL", map[bool]string{true: "1", false: ""}[mode == "failure"])
			t.Setenv("TVROCK_COMMAND_TEST_WAIT", map[bool]string{true: "1", false: ""}[mode == "cancel"])
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if mode == "cancel" {
				go func() {
					for ctx.Err() == nil {
						if _, err := os.Stat(probeFile); err == nil {
							cancel()
							return
						}
						time.Sleep(10 * time.Millisecond)
					}
				}()
			}
			title := "日本語 & whoami | < > ^ %PATH% !VAR! \"quoted\" 'single'\nnext @TT"
			_, err := manager.execCustomCommandContext(ctx, &CustomCommand{Type: "executable", AppPath: app, Options: []string{"--", "--probe", probeFile, "two words", "&fixed"}}, "", title, "", 0, 0, 0, "", "")
			if mode == "success" && err != nil {
				t.Fatal(err)
			}
			if mode == "failure" {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 7 {
					t.Fatalf("exit error: %v", err)
				}
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel: %v", err)
			}
			encoded, readErr := os.ReadFile(probeFile)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var probe customCommandProbe
			if err := json.Unmarshal(encoded, &probe); err != nil {
				t.Fatal(err)
			}
			if probe.Data.Title != title || !reflect.DeepEqual(probe.Args, []string{"two words", "&fixed"}) {
				t.Fatalf("data/arguments corrupted: %+v", probe)
			}
			if _, err := os.Stat(filepath.Dir(probe.Path)); !os.IsNotExist(err) {
				t.Fatalf("context directory remains: %v", err)
			}
			if manager.GetBusyCount() != 0 {
				t.Fatal("command remained busy")
			}
		})
	}
	if runtime.GOOS != "windows" {
		if _, err := newCustomCommand(context.Background(), &CustomCommand{Type: "batch", AppPath: "test.cmd"}); err == nil {
			t.Fatal("batch accepted on non-Windows")
		}
	}
}
