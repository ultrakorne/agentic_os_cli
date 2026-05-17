package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// newTestModel builds a startModel with a synthetic ScanResult split across
// two sections (Agents: alpha, bravo, charlie; tools: ping, pong). The
// fsnotify channels are dummy; Update never reads from them under the key
// paths these tests exercise.
func newTestModel(t *testing.T) *startModel {
	t.Helper()
	scan := scheduler.ScanResult{
		Agents: []scheduler.Agent{
			{ID: "alpha", Section: "Agents"},
			{ID: "bravo", Section: "Agents"},
			{ID: "charlie", Section: "Agents"},
			{ID: "ping", Section: "tools"},
			{ID: "pong", Section: "tools"},
		},
	}
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	t.Cleanup(func() { close(events); close(errs) })
	m := newStartModel("/fake/home", scan, nil, events, errs)
	m.width = 80
	m.height = 24
	m.applyLayout()
	return &m
}

func updateKey(m *startModel, s string) (*startModel, tea.Cmd) {
	updated, cmd := m.Update(keyMsg(s))
	sm, ok := updated.(*startModel)
	if !ok {
		panic("Update did not return *startModel")
	}
	return sm, cmd
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	}
	if len(s) == 1 {
		// textinput.Model (used by list's filter prompt) inserts characters
		// based on Key.Text, not Code. We set both so the same synthesized
		// message routes correctly through both parent key-matching and
		// child text-input handling.
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	panic("unsupported key in test helper: " + s)
}

// focusedID returns the agent id selected in the currently-focused section's
// list, or "" if nothing is selected.
func focusedID(m *startModel) string {
	if len(m.sections) == 0 {
		return ""
	}
	sel := m.sections[m.focused].list.SelectedItem()
	if sel == nil {
		return ""
	}
	it, ok := sel.(agentItem)
	if !ok {
		return ""
	}
	return it.agent.ID
}

func TestStart_initialFocus(t *testing.T) {
	m := newTestModel(t)
	if m.focused != 0 {
		t.Errorf("initial focused section = %d, want 0", m.focused)
	}
	if got := focusedID(m); got != "alpha" {
		t.Errorf("initial focus = %q, want %q", got, "alpha")
	}
}

func TestStart_jkMovesWithinSection(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "j")
	if got := focusedID(m); got != "bravo" {
		t.Errorf("after j focus = %q, want bravo", got)
	}
	m, _ = updateKey(m, "j")
	if got := focusedID(m); got != "charlie" {
		t.Errorf("after jj focus = %q, want charlie", got)
	}
	m, _ = updateKey(m, "k")
	if got := focusedID(m); got != "bravo" {
		t.Errorf("after jjk focus = %q, want bravo", got)
	}
}

// j on the last item of a section hops to the first item of the next
// non-empty section. k on the first item of a section hops back to the
// last item of the previous one. j on the last item of the last section
// stays put (no wrap).
func TestStart_jCrossesIntoNextSection(t *testing.T) {
	m := newTestModel(t)
	for range 3 {
		m, _ = updateKey(m, "j") // alpha -> bravo -> charlie -> ping
	}
	if m.focused != 1 {
		t.Errorf("after jjj focused = %d, want 1 (crossed into tools)", m.focused)
	}
	if got := focusedID(m); got != "ping" {
		t.Errorf("after jjj focus = %q, want ping (top of next section)", got)
	}
}

func TestStart_kCrossesIntoPreviousSection(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "2") // jump to section 2 (tools — ping is first)
	if m.focused != 1 {
		t.Fatalf("setup: `2` should land on section 1, got %d", m.focused)
	}
	m, _ = updateKey(m, "k") // at top of section 1 → hop back to bottom of section 0
	if m.focused != 0 {
		t.Errorf("k from top of section 1 → focused = %d, want 0", m.focused)
	}
	if got := focusedID(m); got != "charlie" {
		t.Errorf("k crossing back should land on last item of prev section; got %q, want charlie", got)
	}
}

func TestStart_jOnLastSectionLastItemStays(t *testing.T) {
	m := newTestModel(t)
	for range 10 {
		m, _ = updateKey(m, "j") // walk all the way to the bottom-most item
	}
	if got := focusedID(m); got != "pong" {
		t.Errorf("after 10×j focus = %q, want pong (last in last section)", got)
	}
	if m.focused != 1 {
		t.Errorf("focused = %d, want 1 (last section)", m.focused)
	}
}

// h and l are reserved keys (no-ops) — section switching is via j/k wrap or
// number jump. Guard against accidentally re-binding them later.
func TestStart_hlAreNoOps(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "l")
	if m.focused != 0 || focusedID(m) != "alpha" {
		t.Errorf("l should be a no-op; focused=%d id=%q", m.focused, focusedID(m))
	}
	m, _ = updateKey(m, "h")
	if m.focused != 0 || focusedID(m) != "alpha" {
		t.Errorf("h should be a no-op; focused=%d id=%q", m.focused, focusedID(m))
	}
}

func TestStart_numberJumpsSection(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "2")
	if m.focused != 1 {
		t.Errorf("after `2` focused = %d, want 1", m.focused)
	}
	m, _ = updateKey(m, "1")
	if m.focused != 0 {
		t.Errorf("after `21` focused = %d, want 0", m.focused)
	}
	m, _ = updateKey(m, "9") // out of range, no-op
	if m.focused != 0 {
		t.Errorf("after `9` (oor) focused = %d, want 0 (unchanged)", m.focused)
	}
}

func TestStart_GjumpsToLastInActiveSection(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "G")
	if got := focusedID(m); got != "charlie" {
		t.Errorf("after G focus = %q, want charlie", got)
	}
}

func TestStart_filterNarrowsListAndFooterAdapts(t *testing.T) {
	m := newTestModel(t)
	m, _ = updateKey(m, "/")
	if !m.activeListSettingFilter() {
		t.Fatal("/ should put the active list into filter mode")
	}
	// Typing should feed the list's filter input, not the parent's section nav.
	m, _ = updateKey(m, "b")
	m, _ = updateKey(m, "r")
	// Footer should advertise the filter binding set, not the main set.
	keys := m.keys
	keys.mode = footerModeFiltering
	got := m.help.View(keys)
	if !strings.Contains(got, "esc") || !strings.Contains(got, "narrow") {
		t.Errorf("filter-mode footer missing expected hints: %q", got)
	}
	// Commit and verify we're back to the main mode.
	m, _ = updateKey(m, "enter")
	if m.activeListSettingFilter() {
		t.Error("enter should exit filter input mode")
	}
}

func TestStart_ctrlCQuits(t *testing.T) {
	m := newTestModel(t)
	_, cmd := updateKey(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("ctrl+c should produce a quit cmd, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c cmd produced %T, want tea.QuitMsg", cmd())
	}
}

// q on the main view must not quit. bubbles/list's own Quit binding was
// disabled in newStartModel; this guards against a future regression where
// somebody re-enables it.
func TestStart_qOnMainViewDoesNotQuit(t *testing.T) {
	m := newTestModel(t)
	_, cmd := updateKey(m, "q")
	if cmd != nil {
		// The list may still emit nil/non-Quit cmds (status timeout
		// tickers etc.); fail only if it's tea.Quit.
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not produce tea.Quit on the main view")
		}
	}
}

func TestStart_xReturnsCmdWhenAgentFocused(t *testing.T) {
	m := newTestModel(t)
	_, cmd := updateKey(m, "x")
	if cmd == nil {
		t.Fatal("x with a focused card should return a spawn cmd")
	}
}

func TestStart_footerShowsMainKeysByDefault(t *testing.T) {
	m := newTestModel(t)
	keys := m.keys
	keys.mode = footerModeMain
	got := m.help.View(keys)
	for _, want := range []string{"x", "run", "j/k", "1-9", "^C"} {
		if !strings.Contains(got, want) {
			t.Errorf("main footer missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "hjkl") {
		t.Errorf("main footer should not advertise hjkl (h/l were removed): %q", got)
	}
}

// TestStart_inactiveSectionHasNoHighlight checks the fix for the
// cross-section highlight bug: only the focused section's selected item is
// drawn with the accent background.
func TestStart_inactiveSectionHasNoHighlight(t *testing.T) {
	m := newTestModel(t)
	// Focus stays on section 0 (Agents). The view should highlight "alpha"
	// (its selected item) but NOT "ping" (section 1's selected item).
	v := m.View()
	out := stripANSI(v.Content)
	// "alpha" should appear once (in its row).
	if !strings.Contains(out, "alpha") {
		t.Fatalf("output missing alpha: %q", out)
	}
	if !strings.Contains(out, "ping") {
		t.Fatalf("output missing ping: %q", out)
	}
	// The accent-background style would be the only place either id appears
	// with a leading background-color SGR (color 44 / "37;44m" approx).
	// We can't reliably parse the SGR sequence after stripping, so instead
	// inspect the original raw view for the bg-set position.
	raw := v.Content
	alphaIdx := strings.Index(raw, "alpha")
	pingIdx := strings.Index(raw, "ping")
	if alphaIdx < 0 || pingIdx < 0 {
		t.Fatal("alpha or ping missing from raw output")
	}
	// Within ~40 bytes before each agent id, look for "48;5;" or "48;2;"
	// or "4[0-7]m" — any background-color SGR. Active section should have
	// one near alpha; inactive section should not have one near ping.
	hasBgBefore := func(s string, idx int) bool {
		start := idx - 60
		if start < 0 {
			start = 0
		}
		win := s[start:idx]
		return strings.Contains(win, "\x1b[44m") || strings.Contains(win, "\x1b[48;")
	}
	if !hasBgBefore(raw, alphaIdx) {
		t.Errorf("alpha (focused section) should have bg highlight: %q", raw[max(0, alphaIdx-60):alphaIdx])
	}
	if hasBgBefore(raw, pingIdx) {
		t.Errorf("ping (unfocused section) should NOT have bg highlight: %q", raw[max(0, pingIdx-60):pingIdx])
	}
}

// stripANSI removes ESC[…m and similar escape sequences for readable test
// assertions. Crude but good enough for substring matching.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestStart_emptyStateFooter(t *testing.T) {
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	t.Cleanup(func() { close(events); close(errs) })
	m := newStartModel("/fake/home", scheduler.ScanResult{}, nil, events, errs)
	m.width = 80
	m.height = 24
	m.applyLayout()
	keys := m.keys
	keys.mode = footerModeEmpty
	got := m.help.View(keys)
	if !strings.Contains(got, "^C") {
		t.Errorf("empty-state footer should advertise ^C, got %q", got)
	}
	if strings.Contains(got, "j/k") {
		t.Errorf("empty-state footer should not advertise j/k, got %q", got)
	}
}
