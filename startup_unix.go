//go:build linux || darwin

package main

import (
	"context"
	"fmt"
	"os"
)

func prepareConsole(bool) error { return nil }
func installPlatformStop(context.CancelFunc, <-chan struct{}) (func(), error) {
	return func() {}, nil
}
func serviceStandardOutput() bool          { return true }
func reportStartupError(err error, _ bool) { fmt.Fprintln(os.Stderr, err) }

// systemd/launchd がこのフォアグラウンドプロセスを監視します。フォークやデタッチは行わないでください
func runPlatform(ctx context.Context, _ context.CancelFunc, _ startupOptions) error {
	<-ctx.Done()
	return nil
}
