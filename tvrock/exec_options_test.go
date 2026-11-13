package tvrock

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func backgroundOutputCommand(storage bool) (string, []string) {
	if runtime.GOOS == "windows" {
		if storage {
			return "cmd.exe", []string{"/d", "/c", "echo Caption FreeSpace Size VolumeName&echo C: 20 100&echo D: 40 200 Recorded Programs"}
		}
		return "cmd.exe", []string{"/d", "/c", "echo stdout-marker&echo stderr-marker 1>&2&exit /b 7"}
	}
	if storage {
		return "/bin/sh", []string{"-c", "printf 'Caption FreeSpace Size VolumeName\nC: 20 100\nD: 40 200 Recorded Programs\n'"}
	}
	return "/bin/sh", []string{"-c", "printf 'stdout-marker\n'; printf 'stderr-marker\n' >&2; exit 7"}
}

func TestBackgroundCommandPreservesOutputAndExitStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path, args := backgroundOutputCommand(false)
	cmd := exec.CommandContext(ctx, path, args...)
	configureBackgroundCommand(cmd)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("exit error = %v, output = %q", err, out)
	}
	for _, marker := range []string{"stdout-marker", "stderr-marker"} {
		if !strings.Contains(string(out), marker) {
			t.Errorf("missing %s in %q", marker, out)
		}
	}
}

func TestBackgroundCustomCommandPreservesStorageResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path, args := backgroundOutputCommand(true)
	manager := &CustomCommandManager{BusyList: make(map[string]*exec.Cmd)}
	rows, err := manager.execCustomCommandContext(ctx, &CustomCommand{AppPath: path, Options: args}, "", "", "", 0, 0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got := parseStorageInfo(rows)
	want := map[string]*StorageInfo{
		"C:":                  {Using: 80, Size: 100},
		"D:Recorded Programs": {Using: 160, Size: 200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("storage = %#v, want %#v (rows: %v)", got, want, rows)
	}
	if manager.GetBusyCount() != 0 {
		t.Fatal("completed command remained busy")
	}
}
