//go:build windows

package tvrock

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func newCustomCommand(ctx context.Context, cc *CustomCommand) (*exec.Cmd, error) {
	app := cc.AppPath
	args := append([]string(nil), cc.Options...)
	if cc.commandType() == "executable" {
		if strings.ContainsAny(app, `/\`) {
			var err error
			app, err = filepath.Abs(app)
			if err != nil {
				return nil, err
			}
		}
		cmd := exec.CommandContext(ctx, app, args...)
		cmd.Dir = filepath.Dir(app)
		return cmd, nil
	}
	path, err := filepath.Abs(app)
	if err != nil {
		return nil, err
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	if cc.commandType() == "powershell" {
		app = filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")
		args = append([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", path}, args...)
		cmd := exec.CommandContext(ctx, app, args...)
		cmd.Dir = filepath.Dir(path)
		return cmd, nil
	}
	// Goの通常の引数エスケープはcmd.exe用ではないため、検証済みの固定値を引用する。
	if strings.ContainsAny(path, "\"%!\r\n") {
		return nil, syscall.EINVAL
	}
	app = filepath.Join(systemDir, "cmd.exe")
	line := `"` + app + `" /d /v:off /s /c ""` + path + `"`
	for _, arg := range args {
		line += ` "` + arg + `"`
	}
	line += `"`
	cmd := exec.CommandContext(ctx, app)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	cmd.Dir = filepath.Dir(path)
	return cmd, nil
}
