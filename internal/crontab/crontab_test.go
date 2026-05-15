package crontab

import (
	"strings"
	"testing"
)

// ---------- ExtractManaged ----------

func TestExtractManagedEmpty(t *testing.T) {
	ex := ExtractManaged("")
	if ex.HasMarker || ex.Conflict {
		t.Errorf("empty input: HasMarker=%v Conflict=%v, want both false", ex.HasMarker, ex.Conflict)
	}
	if len(ex.Managed) != 0 {
		t.Errorf("Managed = %v, want empty", ex.Managed)
	}
}

func TestExtractManagedNoMarkersGoesToBefore(t *testing.T) {
	in := "PATH=/usr/bin\n0 * * * * echo hi\n"
	ex := ExtractManaged(in)
	if ex.HasMarker {
		t.Error("HasMarker = true, want false")
	}
	if ex.Conflict {
		t.Error("Conflict = true, want false")
	}
	if ex.Before != "PATH=/usr/bin\n0 * * * * echo hi" {
		t.Errorf("Before = %q", ex.Before)
	}
	if ex.After != "" {
		t.Errorf("After = %q, want empty", ex.After)
	}
}

func TestExtractManagedCleanBlock(t *testing.T) {
	in := "user line 1\n" +
		BeginMarker + "\n" +
		"*/10 * * * * mid1\n" +
		"0 0 * * * mid2\n" +
		EndMarker + "\n" +
		"user tail\n"
	ex := ExtractManaged(in)
	if !ex.HasMarker {
		t.Fatal("HasMarker = false, want true")
	}
	if ex.Conflict {
		t.Error("Conflict = true, want false")
	}
	if ex.Before != "user line 1" {
		t.Errorf("Before = %q", ex.Before)
	}
	if ex.After != "user tail" {
		t.Errorf("After = %q", ex.After)
	}
	wantManaged := []string{"*/10 * * * * mid1", "0 0 * * * mid2"}
	if len(ex.Managed) != len(wantManaged) {
		t.Fatalf("Managed len = %d, want %d (%v)", len(ex.Managed), len(wantManaged), ex.Managed)
	}
	for i, m := range wantManaged {
		if ex.Managed[i] != m {
			t.Errorf("Managed[%d] = %q, want %q", i, ex.Managed[i], m)
		}
	}
}

func TestExtractManagedUnclosedIsConflict(t *testing.T) {
	in := BeginMarker + "\n*/10 * * * * x\n"
	ex := ExtractManaged(in)
	if !ex.Conflict {
		t.Error("Conflict = false, want true (begin with no end)")
	}
}

func TestExtractManagedOrphanEndIsConflict(t *testing.T) {
	in := "*/10 * * * * x\n" + EndMarker + "\n"
	ex := ExtractManaged(in)
	if !ex.Conflict {
		t.Error("Conflict = false, want true (end with no begin)")
	}
}

func TestExtractManagedDuplicateBeginsIsConflict(t *testing.T) {
	in := BeginMarker + "\n" +
		"*/10 * * * * a\n" +
		BeginMarker + "\n" +
		"*/10 * * * * b\n" +
		EndMarker + "\n"
	ex := ExtractManaged(in)
	if !ex.Conflict {
		t.Error("Conflict = false, want true (nested begins)")
	}
}

// ---------- BuildManagedBlock ----------

func TestBuildManagedBlockTickOnly(t *testing.T) {
	tick := BuildTickCommand("/usr/local/bin/aos", "/data")
	out := BuildManagedBlock(nil, "/wrap.sh", "/data", tick)
	if !strings.HasPrefix(out, BeginMarker+"\n") {
		t.Errorf("missing begin: %q", out)
	}
	if !strings.HasSuffix(out, "\n"+EndMarker) {
		t.Errorf("missing end: %q", out)
	}
	if !strings.Contains(out, TickCronSchedule) {
		t.Errorf("missing tick schedule %q in %q", TickCronSchedule, out)
	}
	if !strings.Contains(out, "# agentic_os:"+TickMarkerID) {
		t.Errorf("missing tick marker: %q", out)
	}
}

func TestBuildManagedBlockEntries(t *testing.T) {
	entries := []Entry{
		{AgentID: "alpha", ScriptPath: "/agents/alpha.sh", Expression: "*/5 * * * *"},
		{AgentID: "beta", ScriptPath: "/agents/beta.sh", Expression: "0 9 * * *"},
	}
	out := BuildManagedBlock(entries, "/wrap.sh", "/data", "")
	for _, want := range []string{
		"*/5 * * * * '/wrap.sh' '/data' 'alpha' 'alpha' '/agents/alpha.sh' # agentic_os:alpha",
		"0 9 * * * '/wrap.sh' '/data' 'beta' 'beta' '/agents/beta.sh' # agentic_os:beta",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q in:\n%s", want, out)
		}
	}
}

func TestBuildManagedBlockQuotesSingleQuotes(t *testing.T) {
	entries := []Entry{
		{AgentID: "weird'id", ScriptPath: "/path/with'quote.sh", Expression: "* * * * *"},
	}
	out := BuildManagedBlock(entries, "/wrap.sh", "/data", "")
	// shellQuote turns ' into '\''
	if !strings.Contains(out, `'weird'\''id'`) {
		t.Errorf("AgentID not shell-escaped: %s", out)
	}
	if !strings.Contains(out, `'/path/with'\''quote.sh'`) {
		t.Errorf("ScriptPath not shell-escaped: %s", out)
	}
}

func TestBuildTickCommandShape(t *testing.T) {
	got := BuildTickCommand("/opt/aos/aos", "/data dir")
	want := `'/opt/aos/aos' tick >> '/data dir/tick.log' 2>&1`
	if got != want {
		t.Errorf("BuildTickCommand = %q, want %q", got, want)
	}
}

// ---------- computeNext ----------

func TestComputeNextEmptyEntriesRemovesBlock(t *testing.T) {
	current := "before\n" + BeginMarker + "\nold line\n" + EndMarker + "\nafter\n"
	ex := ExtractManaged(current)
	got := computeNext(current, ex, nil, "/w", "/d", "")
	if strings.Contains(got, BeginMarker) || strings.Contains(got, EndMarker) {
		t.Errorf("markers not stripped: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("user lines lost: %q", got)
	}
}

func TestComputeNextEmptyEntriesNoMarkerIsNoop(t *testing.T) {
	current := "user1\nuser2\n"
	ex := ExtractManaged(current)
	got := computeNext(current, ex, nil, "/w", "/d", "")
	if got != current {
		t.Errorf("mutated input without markers: %q -> %q", current, got)
	}
}

func TestComputeNextReplacesExistingBlock(t *testing.T) {
	current := "user\n" + BeginMarker + "\nstale\n" + EndMarker + "\n"
	ex := ExtractManaged(current)
	entries := []Entry{{AgentID: "a", ScriptPath: "/a.sh", Expression: "* * * * *"}}
	got := computeNext(current, ex, entries, "/w", "/d", "")
	if strings.Contains(got, "stale") {
		t.Errorf("stale entry survived: %q", got)
	}
	if !strings.Contains(got, "agentic_os:a") {
		t.Errorf("new entry missing: %q", got)
	}
	if !strings.Contains(got, "user\n") {
		t.Errorf("user line lost: %q", got)
	}
}

func TestComputeNextAppendsWhenNoMarker(t *testing.T) {
	current := "user line\n"
	ex := ExtractManaged(current)
	entries := []Entry{{AgentID: "a", ScriptPath: "/a.sh", Expression: "* * * * *"}}
	got := computeNext(current, ex, entries, "/w", "/d", "")
	if !strings.HasPrefix(got, "user line\n") {
		t.Errorf("user line not preserved at top: %q", got)
	}
	if !strings.Contains(got, BeginMarker) || !strings.Contains(got, EndMarker) {
		t.Errorf("markers missing: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output must end in newline: %q", got)
	}
}

// ---------- purgeAllManaged ----------

func TestPurgeAllManagedRemovesBlock(t *testing.T) {
	in := "keep1\n" + BeginMarker + "\nmid\n" + EndMarker + "\nkeep2\n"
	out := purgeAllManaged(in)
	if strings.Contains(out, BeginMarker) || strings.Contains(out, EndMarker) || strings.Contains(out, "mid") {
		t.Errorf("block survived purge: %q", out)
	}
	if !strings.Contains(out, "keep1") || !strings.Contains(out, "keep2") {
		t.Errorf("user lines dropped: %q", out)
	}
}

func TestPurgeAllManagedDropsStrayEnd(t *testing.T) {
	in := "keep\n" + EndMarker + "\nkeep2\n"
	out := purgeAllManaged(in)
	if strings.Contains(out, EndMarker) {
		t.Errorf("stray end survived: %q", out)
	}
	if !strings.Contains(out, "keep") || !strings.Contains(out, "keep2") {
		t.Errorf("user lines dropped: %q", out)
	}
}

func TestPurgeAllManagedHandlesMultipleBlocks(t *testing.T) {
	in := BeginMarker + "\nA\n" + EndMarker + "\nuser\n" +
		BeginMarker + "\nB\n" + EndMarker + "\n"
	out := purgeAllManaged(in)
	if strings.Contains(out, "A") || strings.Contains(out, "B") {
		t.Errorf("managed content survived: %q", out)
	}
	if !strings.Contains(out, "user") {
		t.Errorf("user line lost: %q", out)
	}
}
