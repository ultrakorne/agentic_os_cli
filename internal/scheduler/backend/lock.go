package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	syncLockName       = ".backend.lock"
	syncLockWait       = 10 * time.Second
	syncLockStaleAfter = 30 * time.Second
	syncLockRetryEvery = 100 * time.Millisecond
)

// acquireSyncLock serializes Sync against itself across processes. The
// cron-era code held an equivalent lock around crontab rewrites; without it,
// two concurrent refreshes (TUI save + scheduled tick + manual `aos refresh`)
// race the per-agent plist/unit writes and produce phantom "already loaded"
// or "unit file does not exist" failures.
//
// Returns a release function or an error if the lock can't be acquired
// within syncLockWait. The lock file is at <aosHome>/.backend.lock; a stale
// lock (older than syncLockStaleAfter) is removed and reacquired so a killed
// `aos` process doesn't wedge future syncs.
func acquireSyncLock(aosHome string) (release func(), err error) {
	if aosHome == "" {
		// Without a home dir there's nothing reasonable to lock against.
		return func() {}, nil
	}
	path := filepath.Join(aosHome, syncLockName)
	deadline := time.Now().Add(syncLockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("backend lock: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil {
			if time.Since(info.ModTime()) > syncLockStaleAfter {
				_ = os.Remove(path)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("backend lock %s held by another process", path)
		}
		time.Sleep(syncLockRetryEvery)
	}
}
