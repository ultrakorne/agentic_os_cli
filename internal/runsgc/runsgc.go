// Package runsgc trims the on-disk runs directory down to a hard cap. Each
// run owns a `<id>.json` metadata file and optionally a paired `<id>.out`
// output file; the GC groups them by stem so the two files are always
// deleted together, never leaving an orphan behind.
package runsgc

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Result is the summary of a Sweep call: how many run pairs were on disk
// before the sweep, how many were deleted, and how many files were unlinked
// (a deleted run unlinks 1 or 2 files depending on whether the .out existed).
type Result struct {
	Before  int
	Deleted int
	Files   int
}

// Sweep enforces the cap on runsDir. A non-positive cap is treated as "no
// cap" — Sweep is a no-op. A missing runsDir is not an error.
func Sweep(runsDir string, cap int) (Result, error) {
	var res Result
	if cap <= 0 {
		return res, nil
	}

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return res, nil
		}
		return res, err
	}

	type pair struct {
		stem  string
		files []string
		mtime int64
	}
	pairs := make(map[string]*pair)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".json" && ext != ".out" {
			continue
		}
		stem := strings.TrimSuffix(name, ext)
		p, ok := pairs[stem]
		if !ok {
			p = &pair{stem: stem}
			pairs[stem] = p
		}
		p.files = append(p.files, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixNano(); m > p.mtime {
			p.mtime = m
		}
	}

	res.Before = len(pairs)
	if res.Before <= cap {
		return res, nil
	}

	list := make([]*pair, 0, len(pairs))
	for _, p := range pairs {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].mtime != list[j].mtime {
			return list[i].mtime < list[j].mtime
		}
		// Tiebreak on stem so the order is deterministic when the
		// filesystem reports identical mtimes (common on fast test runs
		// and coarse-mtime filesystems).
		return list[i].stem < list[j].stem
	})

	drop := list[:len(list)-cap]
	for _, p := range drop {
		for _, f := range p.files {
			if err := os.Remove(filepath.Join(runsDir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return res, err
			}
			res.Files++
		}
		res.Deleted++
	}
	return res, nil
}
