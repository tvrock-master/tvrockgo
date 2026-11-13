//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRockbarApplicationAndBrowserFallback(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Rockbar.exe")
	if err := os.WriteFile(app, []byte("test fixture, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing.exe")
	const url = "http://localhost:8080/static/rockbar/"
	tests := []struct {
		name                   string
		paths                  []string
		startErr, browserErr   error
		wantLaunch, wantBrowse int
	}{
		{name: "prefer installed application", paths: []string{missing, app}, wantLaunch: 1},
		{name: "application missing", paths: []string{missing}, wantBrowse: 1},
		{name: "directory is not application", paths: []string{root}, wantBrowse: 1},
		{name: "launch failure falls back", paths: []string{app, app}, startErr: errors.New("cannot launch"), wantLaunch: 1, wantBrowse: 1},
		{name: "browser error reported", paths: []string{missing}, browserErr: errors.New("cannot browse"), wantBrowse: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launched, browsed := 0, 0
			err := launchRockbarOrBrowser(tt.paths, url, func(path string) error {
				launched++
				if path != app {
					t.Errorf("unexpected app: %s", path)
				}
				return tt.startErr
			}, func(target string) error {
				browsed++
				if target != url {
					t.Errorf("unexpected browser URL: %s", target)
				}
				return tt.browserErr
			})
			if launched != tt.wantLaunch || browsed != tt.wantBrowse {
				t.Fatalf("launch=%d browse=%d", launched, browsed)
			}
			if !errors.Is(err, tt.browserErr) {
				t.Fatalf("error=%v, want %v", err, tt.browserErr)
			}
		})
	}
}

func TestRockbarDisplayIconPath(t *testing.T) {
	for _, tt := range []struct{ value, want string }{
		{`C:\Apps\Rockbar\Rockbar.exe,0`, `C:\Apps\Rockbar\Rockbar.exe`},
		{`"C:\My Apps\Rockbar.exe",0`, `C:\My Apps\Rockbar.exe`},
		{`C:\Apps\Rockbar\Rockbar.exe`, `C:\Apps\Rockbar\Rockbar.exe`},
		{`C:\Apps\Rockbar\uninstallerIcon.ico`, ""},
		{`C:\Apps\Rockbar\Uninstall Rockbar.exe`, ""},
		{`C:\Apps\Rockbar-Setup-1.0.0-x64.exe`, ""},
	} {
		if got := rockbarDisplayIconPath(tt.value); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestRockbarInstallerDisplayName(t *testing.T) {
	for _, name := range []string{"Rockbar", "Rockbar 1.0.0", "Rockbar 2.1.0-beta.1", "Rockbar 2.1.0+build.4", "Rockbar 2.1.0-beta.1+build.4"} {
		if !rockbarInstallName.MatchString(name) {
			t.Errorf("installer name not recognized: %s", name)
		}
	}
	for _, name := range []string{"Other Rockbar", "Rockbar Tools", "Rockbar-Setup-1.0.0-x64.exe"} {
		if rockbarInstallName.MatchString(name) {
			t.Errorf("unrelated installer recognized: %s", name)
		}
	}
}
