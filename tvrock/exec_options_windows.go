//go:build windows

package tvrock

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureBackgroundCommandは、ヘルパープロセスに対するコンソールの割り当てを抑制します。
// 標準ストリームやGUIアプリケーションのウィンドウには影響しません。
func configureBackgroundCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
