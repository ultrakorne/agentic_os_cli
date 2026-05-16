package scheduler

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

func containsString(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

func writeScript(t *testing.T, path string, body string, exec bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func TestScanAgents_executableIsCollected(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.sh")
	writeScript(t, script, "#!/usr/bin/env bash\necho hi\n", true)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 1 || res.Agents[0].ID != "ok" {
		t.Fatalf("expected one agent 'ok', got %+v", res.Agents)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", res.Issues)
	}
}

func TestScanAgents_notExecutable_flaggedAtTopLevel(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "planner.sh")
	writeScript(t, script, "#!/usr/bin/env bash\necho hi\n", false)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 1 || res.Agents[0].ScriptPath != script {
		t.Fatalf("expected agent record for %s, got %+v", script, res.Agents)
	}
	if !containsString(res.Agents[0].Warnings, "not-executable") {
		t.Fatalf("expected warnings=[not-executable], got %+v", res.Agents[0].Warnings)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("expected no issues (warnings only), got %+v", res.Issues)
	}
}

func TestScanAgents_notExecutable_flaggedInSubfolder(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "Assistant", "daily_planner.sh")
	writeScript(t, script, "#!/usr/bin/env bash\necho hi\n", false)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 1 || res.Agents[0].ScriptPath != script {
		t.Fatalf("expected agent record for %s, got %+v", script, res.Agents)
	}
	if !containsString(res.Agents[0].Warnings, "not-executable") {
		t.Fatalf("expected not-executable warning, got %+v", res.Agents[0].Warnings)
	}
}

func TestScanAgents_notExecutable_noExtRequiresShebang(t *testing.T) {
	dir := t.TempDir()
	withShebang := filepath.Join(dir, "launcher")
	withoutShebang := filepath.Join(dir, "datafile")
	writeScript(t, withShebang, "#!/usr/bin/env bash\necho hi\n", false)
	writeScript(t, withoutShebang, "just text, not a script\n", false)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	paths := make([]string, 0, len(res.Agents))
	for _, a := range res.Agents {
		if containsString(a.Warnings, "not-executable") {
			paths = append(paths, a.ScriptPath)
		}
	}
	if len(paths) != 1 || paths[0] != withShebang {
		t.Fatalf("expected only %s flagged, got %v", withShebang, paths)
	}
}

func TestScanAgents_notExecutable_ignoresSidecarsReadmesDotfiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.meta.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# notes"), 0o644); err != nil {
		t.Fatalf("readme: %v", err)
	}
	writeScript(t, filepath.Join(dir, ".hidden.sh"), "#!/usr/bin/env bash\n", false)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 0 {
		t.Fatalf("expected no agent records (all skipped), got %+v", res.Agents)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", res.Issues)
	}
}

func TestScanAgents_duplicateIDsAcrossSections(t *testing.T) {
	dir := t.TempDir()
	top := filepath.Join(dir, "foo.sh")
	nested := filepath.Join(dir, "Assistant", "foo.sh")
	writeScript(t, top, "#!/usr/bin/env bash\n", true)
	writeScript(t, nested, "#!/usr/bin/env bash\n", true)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 1 || res.Agents[0].ScriptPath != top {
		t.Fatalf("expected first-wins on top-level %s, got %+v", top, res.Agents)
	}
	if len(res.Issues) != 1 || res.Issues[0].Kind != "duplicate" {
		t.Fatalf("expected one duplicate issue, got %+v", res.Issues)
	}
}

func TestScanAgents_agentsAreSortedByID(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.sh")
	b := filepath.Join(dir, "b.sh")
	writeScript(t, b, "#!/usr/bin/env bash\n", true)
	writeScript(t, a, "#!/usr/bin/env bash\n", true)

	res, err := ScanAgents(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %+v", res.Agents)
	}
	ids := []string{res.Agents[0].ID, res.Agents[1].ID}
	sortedIDs := append([]string{}, ids...)
	sort.Strings(sortedIDs)
	if ids[0] != sortedIDs[0] || ids[1] != sortedIDs[1] {
		t.Fatalf("agents not sorted by id: %v", ids)
	}
}
