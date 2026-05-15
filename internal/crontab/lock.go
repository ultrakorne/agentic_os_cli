package crontab

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	lockFilename     = ".crontab.lock"
	lockStale        = 30 * time.Second
	lockWaitTimeout  = 10 * time.Second
	lockPollInterval = 100 * time.Millisecond
)

// acquireLock returns a release func, or (nil, nil) if the wait timed out
// (treated as "skip this round"). Mirrors crontab.ts:acquireCrontabLock.
func acquireLock(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, lockFilename)
	start := nowFn()
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() {
				_ = os.Remove(path)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if st, statErr := os.Stat(path); statErr == nil {
			if nowFn().Sub(st.ModTime()) > lockStale {
				_ = os.Remove(path)
				continue
			}
		} else {
			continue
		}
		if nowFn().Sub(start) > lockWaitTimeout {
			return nil, nil
		}
		time.Sleep(lockPollInterval)
	}
}
