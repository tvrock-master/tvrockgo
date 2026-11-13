//go:build windows

package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

var singleInstanceLockFile *os.File

func acquireSingleInstanceLock() bool {
	file, err := os.OpenFile(singleInstanceLockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Printf("error: open single instance lock file failed: %v", err)
		return false
	}

	overlapped := &windows.Overlapped{}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return false
		}
		log.Printf("error: acquire single instance lock failed: %v", err)
		return false
	}

	singleInstanceLockFile = file
	return true
}

func singleInstanceLockPath() string {
	return filepath.Join(os.TempDir(), "tvrockgo.lock")
}

func releaseSingleInstanceLock() {
	if singleInstanceLockFile != nil {
		overlapped := &windows.Overlapped{}
		_ = windows.UnlockFileEx(windows.Handle(singleInstanceLockFile.Fd()), 0, 1, 0, overlapped)
		_ = singleInstanceLockFile.Close()
		singleInstanceLockFile = nil
	}
}
