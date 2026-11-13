//go:build linux || darwin

package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var singleInstanceLockFile *os.File

func acquireSingleInstanceLock() bool {
	lockPath := singleInstanceLockPath()
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Printf("error: open single instance lock file failed: %v", err)
		return false
	}

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false
		}
		log.Printf("error: acquire single instance lock failed: %v", err)
		return false
	}

	singleInstanceLockFile = file
	return true
}

func singleInstanceLockPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "tvrockgo.lock")
	}

	lockDir := filepath.Join(cacheDir, "tvrockgo")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return filepath.Join(os.TempDir(), "tvrockgo.lock")
	}

	return filepath.Join(lockDir, "tvrockgo.lock")
}

func releaseSingleInstanceLock() {
	if singleInstanceLockFile != nil {
		_ = unix.Flock(int(singleInstanceLockFile.Fd()), unix.LOCK_UN)
		_ = singleInstanceLockFile.Close()
		singleInstanceLockFile = nil
	}
}
