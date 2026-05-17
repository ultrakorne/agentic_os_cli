package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// startModel is the Bubble Tea model for `aos start`. It composes one
// list.Model per section, a help.Model for the contextual footer, and an
// fsnotify-driven map of the latest run per agent. Section navigation,
// run-dispatch, and filesystem-event wiring are owned here; per-section
// scrolling, j/k/g/G/`/` filter, and item rendering are delegated to
// list.Model.
type startModel struct {
	aosHome string
	store   *scheduler.FileRunStore
	wrapper string

	sections []sectionPanel
	focused  int

	// agentLoc maps agent id -> (section index, item index in that list) so
	// fsnotify events can update the matching list.Item without scanning
	// every list.
	agentLoc map[string]agentLocation

	help help.Model
	keys keyMap

	width, height int

	toast      string
	toastUntil time.Time

	events chan fsnotify.Event
	errs   chan error

	// popup is non-nil while the agent-details overlay is open. While set,
	// Update routes input through the popup and View renders it instead of
	// the section grid.
	popup *detailsModel
}

type sectionPanel struct {
	name           string
	scheduledCount int
	list           list.Model
}

type agentLocation struct {
	section int
	item    int
}

// agentItem is the list.Item the per-section list works with. FilterValue is
// the agent id so list.DefaultFilter's fuzzy matcher narrows by id.
type agentItem struct {
	agent   scheduler.Agent
	lastRun scheduler.Run
}

func (a agentItem) FilterValue() string { return a.agent.ID }

// agentDelegate is the per-row renderer wired into list.Model. Single line,
// fixed-width id column so the focus highlight never shifts trailing text.
// sectionFocused gates the selection highlight: a list's `Index()` always
// points at *some* item, but we only want that item drawn as "selected" when
// its containing section is the active one — otherwise non-active sections
// would visually fight the active one for the user's attention.
type agentDelegate struct {
	idWidth        int
	sectionFocused bool
}

func (agentDelegate) Height() int                             { return 1 }
func (agentDelegate) Spacing() int                            { return 0 }
func (agentDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d agentDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(agentItem)
	if !ok {
		return
	}
	selected := d.sectionFocused && index == m.Index()

	glyph := statusGlyph(string(it.lastRun.Status), it.agent.Meta.Schedule != nil)
	glyphStyled := statusStyle(string(it.lastRun.Status)).Render(glyph)

	idStyle := lipgloss.NewStyle().Width(d.idWidth).Padding(0, 1).Bold(true)
	if selected {
		idStyle = idStyle.Foreground(colorEmphasis).Background(colorAccent)
	}
	idCell := idStyle.Render(it.agent.ID)

	when := "never run"
	if it.lastRun.ID != "" {
		when = relativeFromNow(it.lastRun.StartedAtTime)
	}
	whenCell := lipgloss.NewStyle().Width(11).Render(styleMuted.Render(when))

	sched := summarizeSchedule(it.agent.Meta.Schedule)
	var schedCell string
	if sched == "-" {
		schedCell = styleMuted.Render("unscheduled")
	} else {
		schedCell = lipgloss.NewStyle().Foreground(colorHeader).Render(sched)
	}

	warn := ""
	if len(it.agent.Warnings) > 0 {
		warn = "  " + styleWarn.Render("⚠ "+strings.Join(it.agent.Warnings, ","))
	}

	row := glyphStyled + " " + idCell + " " + whenCell + " " + schedCell + warn
	// Force every row to exactly the list's width: truncate if it would
	// overflow, pad if shorter. Without this, the surrounding lipgloss
	// border would shrink to the longest row in *each* list independently,
	// making adjacent section boxes render at different widths.
	if w := m.Width(); w > 0 {
		row = lipgloss.NewStyle().Width(w).MaxWidth(w).Render(row)
	}
	_, _ = io.WriteString(w, row)
}

// keyMap holds every binding the screen advertises. ShortHelp returns the
// subset that's valid in the current mode so the bubbles/help footer changes
// with context.
type keyMap struct {
	Move     key.Binding
	Section  key.Binding
	Run      key.Binding
	Details  key.Binding
	Filter   key.Binding
	Quit     key.Binding

	// mode is consulted by ShortHelp to pick which bindings to advertise.
	// Not a binding itself; just state.
	mode footerMode
}

type footerMode int

const (
	footerModeMain footerMode = iota
	footerModeFiltering
	footerModeEmpty
)

func newKeyMap() keyMap {
	return keyMap{
		Move:    key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		Section: key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "section")),
		Run:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "run")),
		Details: key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "details")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^C", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	switch k.mode {
	case footerModeFiltering:
		// list.Model's built-in filter UX shows its own prompt; we just
		// remind the user how to escape and quit. The keys are real
		// (Keys() != nil) so bubbles/help renders them — the displayed
		// label comes from WithHelp.
		return []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("type", "narrow")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "keep")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
			k.Quit,
		}
	case footerModeEmpty:
		return []key.Binding{k.Quit}
	default:
		return []key.Binding{k.Run, k.Details, k.Move, k.Section, k.Filter, k.Quit}
	}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// Messages.

type runChangedMsg struct{ runID string }
type watcherClosedMsg struct{}
type watcherErrMsg struct{ err error }
type clearToastMsg struct{}

const toastTTL = 5 * time.Second

func newStartModel(aosHome string, store *scheduler.FileRunStore, scan scheduler.ScanResult, runs []scheduler.Run, events chan fsnotify.Event, errs chan error) startModel {
	// Group agents by section in scan order. ScanAgents already returns
	// agents sorted by id; sections come from the first occurrence.
	type group struct {
		name           string
		items          []agentItem
		scheduledCount int
	}
	groupsByName := map[string]int{}
	var groups []group
	for _, a := range scan.Agents {
		idx, ok := groupsByName[a.Section]
		if !ok {
			idx = len(groups)
			groups = append(groups, group{name: a.Section})
			groupsByName[a.Section] = idx
		}
		groups[idx].items = append(groups[idx].items, agentItem{agent: a})
		if a.Meta.Schedule != nil {
			groups[idx].scheduledCount++
		}
	}

	// Build the lastRun map up front so the initial render shows accurate
	// status glyphs without waiting for the watcher.
	latest := map[string]scheduler.Run{}
	for _, r := range runs {
		cur, ok := latest[r.AgentID]
		if !ok || r.StartedAtTime.After(cur.StartedAtTime) {
			latest[r.AgentID] = r
		}
	}

	// Pre-compute the id column width across ALL agents so columns align
	// across sections. Capped to avoid one giant id pushing everything off
	// screen. +2 for the cell padding (1 each side).
	idMax := 0
	for _, a := range scan.Agents {
		if w := lipgloss.Width(a.ID); w > idMax {
			idMax = w
		}
	}
	if idMax > 32 {
		idMax = 32
	}
	delegate := agentDelegate{idWidth: idMax + 2}

	sections := make([]sectionPanel, 0, len(groups))
	agentLoc := map[string]agentLocation{}
	for sIdx, g := range groups {
		items := make([]list.Item, len(g.items))
		for i, it := range g.items {
			if lr, ok := latest[it.agent.ID]; ok {
				it.lastRun = lr
			}
			items[i] = it
			agentLoc[it.agent.ID] = agentLocation{section: sIdx, item: i}
		}

		l := list.New(items, delegate, 0, 0)
		// Strip everything list draws around the items — we want a bare
		// scrollable column. The screen owns the title (above the box) and
		// the help footer (below all boxes). Hiding the filter input is
		// also load-bearing: list.View() reserves one row at the top while
		// `showFilter && filteringEnabled` (the defaults), which would
		// inject an empty row inside every section's border box. We turn
		// it off here and render the filter prompt ourselves at the parent
		// level when the user opens it.
		l.SetShowTitle(false)
		l.SetShowFilter(false)
		l.SetShowStatusBar(false)
		l.SetShowPagination(false)
		l.SetShowHelp(false)
		// h and l are list's PrevPage / NextPage by default. We want them
		// for section switching at the parent, so disable here.
		l.KeyMap.PrevPage.SetEnabled(false)
		l.KeyMap.NextPage.SetEnabled(false)
		// list's own quit binding ('q') would shadow our "q is reserved"
		// rule. Strip it; only Ctrl+C at the parent quits.
		l.DisableQuitKeybindings()

		sections = append(sections, sectionPanel{
			name:           g.name,
			scheduledCount: g.scheduledCount,
			list:           l,
		})
	}

	// help.New uses a hardcoded gray hex (#4A4A4A on dark) for the
	// description text, which renders nearly invisible on most themes.
	// Re-style with ANSI palette slots so the terminal theme decides the
	// hues and the footer stays readable.
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colorEmphasis)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(colorMuted)
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = h.Styles.ShortDesc
	h.Styles.FullSeparator = h.Styles.ShortSeparator
	h.Styles.Ellipsis = h.Styles.ShortSeparator

	return startModel{
		aosHome:  aosHome,
		store:    store,
		wrapper:  filepath.Join(aosHome, "wrapper.sh"),
		sections: sections,
		agentLoc: agentLoc,
		help:     h,
		keys:     newKeyMap(),
		events:   events,
		errs:     errs,
	}
}

func (m *startModel) Init() tea.Cmd {
	return tea.Batch(m.watchCmd(), m.watchErrCmd())
}

func (m *startModel) watchCmd() tea.Cmd {
	return func() tea.Msg {
		e, ok := <-m.events
		if !ok {
			return watcherClosedMsg{}
		}
		name := filepath.Base(e.Name)
		if !strings.HasSuffix(name, ".json") {
			return runChangedMsg{}
		}
		return runChangedMsg{runID: strings.TrimSuffix(name, ".json")}
	}
}

func (m *startModel) watchErrCmd() tea.Cmd {
	return func() tea.Msg {
		err, ok := <-m.errs
		if !ok || err == nil {
			return watcherClosedMsg{}
		}
		return watcherErrMsg{err: err}
	}
}

func (m *startModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Popup intercepts: while the agent-details overlay is open it owns the
	// screen. Resize and timer/toast messages still need to fan out to the
	// parent so the layout stays consistent if the popup closes; the popup
	// itself also wants to see them. Special parent-side messages
	// (popupClosedMsg, agentMetaUpdatedMsg) are handled below before
	// forwarding.
	switch msg := msg.(type) {
	case popupClosedMsg:
		m.popup = nil
		return m, nil
	case agentMetaUpdatedMsg:
		// The popup is the only thing that emits this; it already updates
		// its own state synchronously inside saveAll. Parent's job is just
		// to mirror the change onto the underlying dashboard row.
		m.applyMetaUpdate(msg.agentID, msg.meta)
		return m, nil
	case runChangedMsg:
		// Watcher events MUST be handled at the parent regardless of popup
		// state. Each watchCmd reads one event from m.events and exits; if
		// we don't re-arm it here, no further filesystem changes get
		// pulled into the program after a single event fires while the
		// popup is open. We still apply the change so the underlying
		// dashboard reflects fresh data the moment the popup closes.
		var cmd tea.Cmd
		if msg.runID != "" {
			cmd = m.applyRunChange(msg.runID)
		}
		return m, tea.Batch(cmd, m.watchCmd())
	case watcherErrMsg:
		// Same re-arm rule as runChangedMsg — drop the error into a toast
		// at the parent (popup doesn't have a watcher surface) and re-arm.
		m.setToast(fmt.Sprintf("watcher: %v", msg.err))
		return m, tea.Batch(m.watchErrCmd(), m.toastClearCmd())
	case watcherClosedMsg:
		return m, nil
	}

	if m.popup != nil {
		// WindowSize is critical for both models; let popup re-layout, and
		// keep the parent's stored size in sync so close-then-render still
		// works.
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = ws.Width
			m.height = ws.Height
			m.applyLayout()
		}
		var cmd tea.Cmd
		m.popup, cmd = m.popup.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case clearToastMsg:
		if time.Now().After(m.toastUntil) {
			m.toast = ""
		}
		return m, nil
	}

	// Anything else — forward to the focused list so it can keep its own
	// internal state (spinner ticks, status message timeouts, etc.).
	if cmd := m.forwardToActive(msg); cmd != nil {
		return m, cmd
	}
	return m, nil
}

func (m *startModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits, regardless of mode.
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	// When the active list is editing its filter input, every key (other
	// than Ctrl+C above) must feed the filter — including 'x', 'h', 'l',
	// digits. The list handles Esc/Enter/backspace internally.
	if m.activeListSettingFilter() {
		return m, m.forwardToActive(msg)
	}

	// j/k cross section boundaries when the cursor is at the edge of the
	// active list. Inside a list they fall through to bubbles/list's own
	// CursorUp/CursorDown. 1-9 is a direct jump for many sections.
	switch msg.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(msg.String()[0] - '0')
		m.jumpToSection(n - 1)
		return m, nil
	case "x":
		return m, m.runFocused()
	case "enter":
		return m.openPopup()
	case "j", "down":
		if m.atListBottom() && m.hopSection(+1) {
			return m, nil
		}
	case "k", "up":
		if m.atListTop() && m.hopSection(-1) {
			return m, nil
		}
	}

	// Everything else is a list-level concern (j/k inside bounds, g/G, /, …).
	return m, m.forwardToActive(msg)
}

// atListTop reports whether the active list's cursor is on the first item.
// Empty lists report true so j on an empty section behaves the same as on
// the last item — i.e. we can still hop to the next section.
func (m *startModel) atListTop() bool {
	if len(m.sections) == 0 {
		return true
	}
	return m.sections[m.focused].list.Index() <= 0
}

func (m *startModel) atListBottom() bool {
	if len(m.sections) == 0 {
		return true
	}
	l := m.sections[m.focused].list
	items := l.VisibleItems()
	return len(items) == 0 || l.Index() >= len(items)-1
}

// hopSection moves focus by delta and parks the cursor at the entry edge of
// the new section (top when moving down, bottom when moving up). Returns
// true if the hop happened; false means there's no section in that direction
// with items, so the caller should fall through to normal list behaviour.
func (m *startModel) hopSection(delta int) bool {
	target := m.focused + delta
	for target >= 0 && target < len(m.sections) {
		l := &m.sections[target].list
		if len(l.VisibleItems()) > 0 {
			m.focused = target
			if delta > 0 {
				l.Select(0)
			} else {
				l.Select(len(l.VisibleItems()) - 1)
			}
			return true
		}
		target += delta
	}
	return false
}

func (m *startModel) activeListSettingFilter() bool {
	if len(m.sections) == 0 {
		return false
	}
	return m.sections[m.focused].list.SettingFilter()
}

func (m *startModel) forwardToActive(msg tea.Msg) tea.Cmd {
	if len(m.sections) == 0 {
		return nil
	}
	updated, cmd := m.sections[m.focused].list.Update(msg)
	m.sections[m.focused].list = updated
	return cmd
}

func (m *startModel) jumpToSection(idx int) {
	if idx < 0 || idx >= len(m.sections) {
		return
	}
	m.focused = idx
}

func (m *startModel) runFocused() tea.Cmd {
	if len(m.sections) == 0 {
		return nil
	}
	sel := m.sections[m.focused].list.SelectedItem()
	if sel == nil {
		return nil
	}
	it, ok := sel.(agentItem)
	if !ok {
		return nil
	}
	opts := scheduler.SpawnOpts{
		AosHome:    m.aosHome,
		AgentID:    it.agent.ID,
		ScriptPath: it.agent.ScriptPath,
		RunID:      m.store.NewID(),
		Trigger:    "manual",
	}
	wrapper := m.wrapper
	return func() tea.Msg {
		if err := scheduler.SpawnWrapperDetached(wrapper, opts); err != nil {
			return watcherErrMsg{err: fmt.Errorf("run %s: %w", it.agent.ID, err)}
		}
		return nil
	}
}

// openPopup builds a detailsModel around the currently focused agent and
// stores it on the parent. Init() kicks off the run-history load so the
// History pane fills in lazily.
func (m *startModel) openPopup() (tea.Model, tea.Cmd) {
	if len(m.sections) == 0 {
		return m, nil
	}
	sel := m.sections[m.focused].list.SelectedItem()
	if sel == nil {
		return m, nil
	}
	it, ok := sel.(agentItem)
	if !ok {
		return m, nil
	}
	popup := newDetailsModel(m.aosHome, m.store, it.agent, m.help)
	popup.SetSize(m.width, m.height)
	m.popup = popup
	return m, popup.Init()
}

// applyMetaUpdate refreshes the in-memory agent record after the popup writes
// a new description / schedule, so the section list row reflects the change
// without a full rescan. Scheduled count is recomputed too — if the kind
// flipped on or off, the section header's count needs to follow.
func (m *startModel) applyMetaUpdate(agentID string, meta scheduler.AgentMeta) {
	loc, ok := m.agentLoc[agentID]
	if !ok {
		return
	}
	sec := &m.sections[loc.section]
	cur, ok := sec.list.Items()[loc.item].(agentItem)
	if !ok {
		return
	}
	prevScheduled := cur.agent.Meta.Schedule != nil
	cur.agent.Meta = meta
	nowScheduled := meta.Schedule != nil
	if prevScheduled && !nowScheduled {
		sec.scheduledCount--
	} else if !prevScheduled && nowScheduled {
		sec.scheduledCount++
	}
	sec.list.SetItem(loc.item, cur)
}

// applyRunChange re-reads the run record named runID, looks up which agent
// it belongs to, and updates that agent's item in its list. Returns the cmd
// from list.SetItem so any side-effect (none today) is captured.
func (m *startModel) applyRunChange(runID string) tea.Cmd {
	r, err := m.store.Get(runID)
	if err != nil || r.AgentID == "" {
		return nil
	}
	loc, ok := m.agentLoc[r.AgentID]
	if !ok {
		return nil
	}
	sec := &m.sections[loc.section]
	cur, ok := sec.list.Items()[loc.item].(agentItem)
	if !ok {
		return nil
	}
	if cur.lastRun.ID != "" && cur.lastRun.StartedAtTime.After(r.StartedAtTime) {
		// We already have a newer record (e.g. user already saw it). Skip.
		return nil
	}
	cur.lastRun = r
	return sec.list.SetItem(loc.item, cur)
}

func (m *startModel) setToast(s string) {
	m.toast = s
	m.toastUntil = time.Now().Add(toastTTL)
	m.applyLayout()
}

func (m *startModel) toastClearCmd() tea.Cmd {
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return clearToastMsg{} })
}
