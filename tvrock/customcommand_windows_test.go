//go:build windows

package tvrock

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCustomCommandWindowsScripts(t *testing.T) {
	// このテストの子プロセスだけでローカル生成スクリプトを許可する。OSの設定は変更しない。
	t.Setenv("PSExecutionPolicyPreference", "RemoteSigned")
	dir := filepath.Join(t.TempDir(), "日本語 & scripts")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ps := filepath.Join(dir, "probe.ps1")
	script := `param([string]$FixedArg)
$ErrorActionPreference = 'Stop'
$data = Get-Content -LiteralPath $env:TVROCK_CONTEXT_FILE -Raw -Encoding UTF8 | ConvertFrom-Json
$probe = @{ data = $data; path = $env:TVROCK_CONTEXT_FILE; args = @($FixedArg) } | ConvertTo-Json -Depth 8
[IO.File]::WriteAllText($env:TVROCK_COMMAND_TEST_OUTPUT, $probe, (New-Object Text.UTF8Encoding($false)))
`
	if err := os.WriteFile(ps, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	bat := filepath.Join(dir, "probe.cmd")
	if err := os.WriteFile(bat, []byte("@echo off\r\nsetlocal DisableDelayedExpansion\r\n\"%SystemRoot%\\System32\\WindowsPowerShell\\v1.0\\powershell.exe\" -NoLogo -NoProfile -NonInteractive -File \"%~dp0probe.ps1\" -FixedArg \"%~1\"\r\nexit /b %errorlevel%\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"batch", "powershell"} {
		t.Run(kind, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "result.json")
			t.Setenv("TVROCK_COMMAND_TEST_OUTPUT", out)
			path, args := ps, []string{"-FixedArg", "two words"}
			if kind == "batch" {
				path, args = bat, []string{"two words"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			title := "番組 & echo injected | <tag> %PATH% !VALUE! ^ \"引用\"\n次の行 @TT"
			manager := &CustomCommandManager{BusyList: make(map[string]*exec.Cmd)}
			if _, err := manager.execCustomCommandContext(ctx, &CustomCommand{Type: kind, AppPath: path, Options: args}, "", title, "", 0, 0, 0, "", ""); err != nil {
				t.Fatal(err)
			}
			encoded, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var probe customCommandProbe
			if err := json.Unmarshal(encoded, &probe); err != nil {
				t.Fatal(err)
			}
			if probe.Data.Title != title || !reflect.DeepEqual(probe.Args, []string{"two words"}) {
				t.Fatalf("script data: %+v", probe)
			}
			if _, err := os.Stat(filepath.Dir(probe.Path)); !os.IsNotExist(err) {
				t.Fatalf("context remains: %v", err)
			}
		})
	}
}

func TestCustomCommandPowerShellExamples(t *testing.T) {
	t.Setenv("PSExecutionPolicyPreference", "RemoteSigned")
	script, err := filepath.Abs("../docs/custom-command-examples/WriteRecordInfo.ps1")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"Touch", "Tweet"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			manager := &CustomCommandManager{BusyList: make(map[string]*exec.Cmd)}
			title := "日本語 & echo unsafe %PATH% !VAR! \"quoted\""
			_, err := manager.execCustomCommandContext(ctx, &CustomCommand{Type: "powershell", AppPath: script, Options: []string{"-Mode", mode, "-OutputDirectory", dir}}, "", title, "", 0, 0, 0, "key", "station")
			if err != nil {
				t.Fatal(err)
			}
			file := "record.txt"
			if mode == "Tweet" {
				file = "tweet.txt"
			}
			encoded, err := os.ReadFile(filepath.Join(dir, file))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), title) {
				t.Fatalf("example corrupted title: %q", encoded)
			}
			if mode == "Tweet" && !strings.Contains(string(encoded), "(0分)") {
				t.Fatalf("example lost Japanese duration: %q", encoded)
			}
		})
	}
}
