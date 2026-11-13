//go:build linux || darwin

package tvrock

import "os/exec"

// コンソール割り当てフラグはWindows固有のものです。Unixプロセスオプションは維持してください
func configureBackgroundCommand(cmd *exec.Cmd) {}
