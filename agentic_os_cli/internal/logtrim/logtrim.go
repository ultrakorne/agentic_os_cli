package logtrim

import (
	"bytes"
	"errors"
	"io"
	"os"
)

const (
	DefaultMaxBytes  = 256 * 1024
	DefaultKeepBytes = 128 * 1024
)

// Trim head-trims the file to keepBytes if it exceeds maxBytes. The retained
// tail starts at the first newline within the kept window so partial lines
// are dropped. Returns (trimmed, err). Missing file → trimmed=false, no err.
func Trim(path string, maxBytes, keepBytes int64) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Size() <= maxBytes {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.Seek(info.Size()-keepBytes, io.SeekStart); err != nil {
		return false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return false, err
	}
	if idx := bytes.IndexByte(data, '\n'); idx >= 0 && idx+1 < len(data) {
		data = data[idx+1:]
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	return true, nil
}
