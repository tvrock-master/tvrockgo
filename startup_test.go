package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		console      bool
		dir, rockbar string
		command      int
		importPath   string
		wantErr      bool
	}{
		{name: "default"},
		{name: "console", args: []string{"--console", "--data-dir", "state"}, console: true, dir: "state"},
		{name: "rockbar", args: []string{"--rockbar", "Rockbar.exe"}, rockbar: "Rockbar.exe"},
		{name: "import", args: []string{"--timemachine", "archive"}, command: 2, importPath: "archive"},
		{name: "import with equals and spaces", args: []string{"--timemachine=C:\\Import Data\\tvrock"}, command: 2, importPath: "C:\\Import Data\\tvrock"},
		{name: "options before import", args: []string{"--console", "--data-dir", "state", "--timemachine", "archive"}, console: true, dir: "state", command: 2, importPath: "archive"},
		{name: "options after import", args: []string{"--timemachine", "archive", "--console", "--data-dir", "state"}, console: true, dir: "state", command: 2, importPath: "archive"},
		{name: "missing import path", args: []string{"--timemachine"}, wantErr: true},
		{name: "empty import path", args: []string{"--timemachine="}, wantErr: true},
		{name: "extra import arg", args: []string{"--timemachine", "archive", "extra"}, wantErr: true},
		{name: "legacy import syntax", args: []string{"/timemachine", "archive"}, wantErr: true},
		{name: "unknown option", args: []string{"--unknown"}, wantErr: true},
		{name: "unknown command", args: []string{"import"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.args, io.Discard)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v", err)
			}
			if tt.wantErr {
				return
			}
			if got.console != tt.console || got.dataDir != tt.dir || got.rockbar != tt.rockbar || len(got.command) != tt.command {
				t.Fatalf("options = %+v", got)
			}
			if tt.command != 0 && (got.command[0] != "--timemachine" || got.command[1] != tt.importPath) {
				t.Fatalf("import command = %q; want --timemachine %q", got.command, tt.importPath)
			}
		})
	}
	var help bytes.Buffer
	if _, err := parseOptions([]string{"--help"}, &help); err != flag.ErrHelp {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(help.String(), "-timemachine") {
		t.Fatalf("help does not describe timemachine: %s", help.String())
	}
}

func TestLocalWebURL(t *testing.T) {
	for _, tt := range []struct{ address, want string }{
		{"localhost:80", "http://localhost:80"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080"},
		{":8080", "http://127.0.0.1:8080"},
		{"[::]:8080", "http://[::1]:8080"},
		{"[::1]:80", "http://[::1]:80"},
		{"192.0.2.1:8080", "http://192.0.2.1:8080"},
	} {
		got, err := localWebURL(tt.address)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %q, %v; want %q", tt.address, got, err, tt.want)
		}
	}
	if _, err := localWebURL("invalid"); err == nil {
		t.Fatal("expected invalid address error")
	}
}

func TestRunCommandCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runCommand(ctx, []string{"--timemachine", filepath.Join(t.TempDir(), "missing")}); err != nil {
		t.Fatalf("cancellation should finish normally: %v", err)
	}
}

func TestRunCommandReturnsImportError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runCommand(context.Background(), []string{"--timemachine", t.TempDir()}); err == nil {
		t.Fatal("import initialization error was discarded")
	}
}
