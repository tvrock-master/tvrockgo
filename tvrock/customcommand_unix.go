//go:build !windows

package tvrock

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func newCustomCommand(ctx context.Context, cc *CustomCommand) (*exec.Cmd, error) {
	if cc.commandType() == "batch" {
		return nil, fmt.Errorf("type: batch is supported only on Windows")
	}
	app, args := cc.AppPath, append([]string(nil), cc.Options...)
	if cc.commandType() == "executable" && strings.ContainsRune(app, '/') {
		var err error
		app, err = filepath.Abs(app)
		if err != nil {
			return nil, err
		}
	}
	dir := filepath.Dir(app)
	if cc.commandType() == "powershell" {
		path, err := filepath.Abs(app)
		if err != nil {
			return nil, err
		}
		app = "pwsh"
		args = append([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", path}, args...)
		dir = filepath.Dir(path)
	}
	cmd := exec.CommandContext(ctx, app, args...)
	cmd.Dir = dir
	return cmd, nil
}
