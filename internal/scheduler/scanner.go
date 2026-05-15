package scheduler

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	defaultSection = "Agents"
	metaSuffix     = ".meta.json"
)

var supportedExts = map[string]struct{}{
	".sh":   {},
	".bash": {},
	".zsh":  {},
	"":      {},
}

type Agent struct {
	ID         string
	ScriptPath string
	Section    string
	MetaPath   string
	Meta       AgentMeta
}

type ScanIssue struct {
	Kind string // "duplicate", "not-executable"
	Path string
	Note string
}

type ScanResult struct {
	Agents []Agent
	Issues []ScanIssue
}

// ScanAgents walks agentsDir per the rules in src/main/agents/scanner.ts.
func ScanAgents(agentsDir string) (ScanResult, error) {
	res := ScanResult{}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return res, err
	}
	seen := map[string]string{}
	if err := collectInto(agentsDir, defaultSection, &res, seen); err != nil {
		return res, err
	}
	dirEntries, err := os.ReadDir(agentsDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
		dirEntries = nil
	}
	for _, e := range dirEntries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if err := collectInto(filepath.Join(agentsDir, e.Name()), e.Name(), &res, seen); err != nil {
			return res, err
		}
	}
	sort.Slice(res.Agents, func(i, j int) bool {
		return res.Agents[i].ID < res.Agents[j].ID
	})
	sort.Slice(res.Issues, func(i, j int) bool {
		return res.Issues[i].Path < res.Issues[j].Path
	})
	return res, nil
}

func collectInto(dir, section string, res *ScanResult, seen map[string]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.EqualFold(name, "readme.md") {
			continue
		}
		if strings.HasSuffix(name, metaSuffix) {
			continue
		}
		ext := filepath.Ext(name)
		if _, ok := supportedExts[ext]; !ok {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if ext == "" {
			ok, _ := hasShebang(full)
			if !ok {
				continue
			}
		}
		if !isExecutable(full) {
			continue
		}
		id := strings.TrimSuffix(name, ext)
		if strings.HasPrefix(id, "__") {
			continue
		}
		if prev, dup := seen[id]; dup {
			res.Issues = append(res.Issues, ScanIssue{
				Kind: "duplicate",
				Path: full,
				Note: fmt.Sprintf("duplicate id %q; kept %s", id, prev),
			})
			continue
		}
		seen[id] = full
		metaPath := filepath.Join(dir, id+metaSuffix)
		meta := readMeta(metaPath)
		res.Agents = append(res.Agents, Agent{
			ID:         id,
			ScriptPath: full,
			Section:    section,
			MetaPath:   metaPath,
			Meta:       meta,
		})
	}
	return nil
}

func isExecutable(path string) bool {
	return unix.Access(path, unix.X_OK) == nil
}

func hasShebang(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 2)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return n == 2 && buf[0] == '#' && buf[1] == '!', nil
}

func readMeta(path string) AgentMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentMeta{}
	}
	return ParseMeta(data)
}
