package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// openPopupAt opens the popup on the agent currently focused in the section
// grid. Returns the resulting model so the caller can chain further key
// presses through it.
func openPopupAt(t *testing.T, m *startModel) *startModel {
	t.Helper()
	m, _ = updateKey(m, "enter")
	if m.popup == nil {
		t.Fatalf("popup not opened after enter")
	}
	return m
}

func TestPopup_EnterOpensCurrentAgent(t *testing.T) {
	m := newTestModel(t)
	if m.popup != nil {
		t.Fatalf("popup unexpectedly set on fresh model")
	}
	m = openPopupAt(t, m)
	if m.popup.agent.ID != "alpha" {
		t.Errorf("expected popup for alpha, got %q", m.popup.agent.ID)
	}
	if m.popup.active != paneConfig {
		t.Errorf("expected popup to open on Config pane, got %v", m.popup.active)
	}
	if m.popup.configFocus != focusDesc {
		t.Errorf("expected initial focus on description, got %v", m.popup.configFocus)
	}
}

func TestPopup_EscClosesAtNav(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	// Esc at nav level emits popupClosedMsg through a Cmd. The parent's
	// Update then drops m.popup. We execute the cmd inline and feed the
	// result back.
	_, cmd := m.popup.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatalf("expected close cmd from esc at nav level")
	}
	msg := cmd()
	if _, ok := msg.(popupClosedMsg); !ok {
		t.Fatalf("expected popupClosedMsg, got %T", msg)
	}
	updated, _ := m.Update(msg)
	sm := updated.(*startModel)
	if sm.popup != nil {
		t.Errorf("expected popup nil after popupClosedMsg, still set")
	}
}

func TestPopup_CtrlCQuitsFromPopup(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	_, cmd := updateKey(m, "ctrl+c")
	if cmd == nil {
		t.Fatalf("expected quit cmd from ctrl+c in popup")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c in popup did not return tea.QuitMsg")
	}
}

func TestPopup_TabCyclesPanes(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	m, _ = updateKey(m, "tab")
	if m.popup.active != paneHistory {
		t.Errorf("after one tab, expected paneHistory, got %v", m.popup.active)
	}
	m, _ = updateKey(m, "tab")
	if m.popup.active != paneConfig {
		t.Errorf("after two tabs, expected paneConfig again, got %v", m.popup.active)
	}
	m, _ = updateKey(m, "shift+tab")
	if m.popup.active != paneHistory {
		t.Errorf("after shift+tab from config, expected paneHistory, got %v", m.popup.active)
	}
}

func TestPopup_NumbersJumpToPane(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	m, _ = updateKey(m, "2")
	if m.popup.active != paneHistory {
		t.Errorf("expected paneHistory after '2', got %v", m.popup.active)
	}
	m, _ = updateKey(m, "1")
	if m.popup.active != paneConfig {
		t.Errorf("expected paneConfig after '1', got %v", m.popup.active)
	}
}

func TestPopup_EditModeBlocksPaneSwitch(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	// In nav mode, `e` enters edit on the description (initial focus).
	m, _ = updateKey(m, "e")
	if !m.popup.editing {
		t.Fatalf("expected editing after 'e' in nav, got false")
	}
	// While editing, "2" should NOT switch panes — it should feed the
	// textarea (the digit is inserted as text).
	m, _ = updateKey(m, "2")
	if m.popup.active != paneConfig {
		t.Errorf("pane switched while editing description")
	}
	// Esc exits edit mode.
	m, _ = updateKey(m, "esc")
	if m.popup.editing {
		t.Errorf("expected editing=false after esc, still true")
	}
}

func TestPopup_ScheduleKindChange(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	// Walk focus down past the description to the kind row.
	m, _ = updateKey(m, "j")
	if m.popup.configFocus != focusKind {
		t.Fatalf("expected focusKind after one j, got %v", m.popup.configFocus)
	}
	if m.popup.schedKind != kindOff {
		t.Fatalf("expected initial kind kindOff, got %v", m.popup.schedKind)
	}
	m, _ = updateKey(m, "right")
	if m.popup.schedKind != kindHourly {
		t.Errorf("right at kind row should move to hourly, got %v", m.popup.schedKind)
	}
	m, _ = updateKey(m, "right")
	if m.popup.schedKind != kindDaily {
		t.Errorf("right again should move to daily, got %v", m.popup.schedKind)
	}
	// Past the rightmost: clamp, stay on daily.
	m, _ = updateKey(m, "right")
	if m.popup.schedKind != kindDaily {
		t.Errorf("right past daily should clamp, got %v", m.popup.schedKind)
	}
	m, _ = updateKey(m, "left")
	if m.popup.schedKind != kindHourly {
		t.Errorf("left should move back to hourly, got %v", m.popup.schedKind)
	}
}

func TestPopup_ConfigFocusOrderFollowsKind(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	m, _ = updateKey(m, "j")      // focus → kind
	m, _ = updateKey(m, "right")  // kind → hourly
	// Hourly cycle from kind onwards: everyHours → minute. No save entry
	// — saving is bound to ctrl+s, not a focusable element.
	expect := []configFocus{focusEveryHours, focusMinute}
	for i, want := range expect {
		m, _ = updateKey(m, "j")
		if m.popup.configFocus != want {
			t.Errorf("hourly cycle step %d: want %v, got %v", i, want, m.popup.configFocus)
		}
	}
	// One more j should stay at the last field (clamp).
	m, _ = updateKey(m, "j")
	if m.popup.configFocus != focusMinute {
		t.Errorf("expected j past last field to clamp at focusMinute, got %v", m.popup.configFocus)
	}
}

func TestPopup_ScheduleDaysToggle(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	m, _ = updateKey(m, "j")     // focus → kind
	m, _ = updateKey(m, "right") // hourly
	m, _ = updateKey(m, "right") // daily
	if m.popup.schedKind != kindDaily {
		t.Fatalf("expected kindDaily, got %v", m.popup.schedKind)
	}
	// Walk to days field via j.
	m, _ = updateKey(m, "j")
	if m.popup.configFocus != focusDays {
		t.Fatalf("expected focusDays, got %v", m.popup.configFocus)
	}
	// Toggle the highlighted day (Mon).
	m, _ = updateKey(m, " ")
	if !m.popup.days[0] {
		t.Errorf("expected Mon toggled on after space")
	}
	// Cursor right to Tue, toggle.
	m, _ = updateKey(m, "right")
	if m.popup.schedDayCursor != 1 {
		t.Errorf("expected day cursor 1, got %d", m.popup.schedDayCursor)
	}
	m, _ = updateKey(m, " ")
	if !m.popup.days[1] {
		t.Errorf("expected Tue toggled on")
	}
	// Untoggle Mon by cursoring back and pressing space again.
	m, _ = updateKey(m, "left")
	m, _ = updateKey(m, " ")
	if m.popup.days[0] {
		t.Errorf("expected Mon toggled off, still on")
	}
}

func TestPopup_FooterModeShiftsByContext(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	// Config nav: footer should show move + edit + next tab.
	view := m.popup.View()
	if !strings.Contains(view, "edit") || !strings.Contains(view, "next tab") {
		t.Errorf("config nav footer missing edit/next tab: %q", lastFooterLine(view))
	}
	// History pane: footer should switch to row/view-output language.
	m, _ = updateKey(m, "2")
	view = m.popup.View()
	if !strings.Contains(view, "view") {
		t.Errorf("history footer missing 'view': %q", lastFooterLine(view))
	}
}

func TestPopup_HistoryEmptyShowsHint(t *testing.T) {
	m := newTestModel(t)
	m = openPopupAt(t, m)
	m, _ = updateKey(m, "2")
	view := m.popup.View()
	if !strings.Contains(view, "no runs yet") {
		t.Errorf("expected 'no runs yet' on history with no records; view tail = %q", lastFooterLine(view))
	}
}

func lastFooterLine(view string) string {
	lines := strings.Split(view, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
