package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// detailsModel is a modal popup opened from the dashboard when the user
// presses Enter on an agent row. It composes bubbles primitives end-to-end:
//   - textarea       — multi-line description editor
//   - textinput      — numeric schedule fields
//   - table          — run history list
//   - viewport       — selected run's run-record + captured output
//   - help           — contextual footer (pane-aware ShortHelp)
//
// Layout: two tabs ("config" merges description + schedule; "history" lists
// past runs and their outputs). The schedule kind pills and weekday chips
// are the only bespoke widgets — bubbles has no radio / multi-toggle
// equivalent at the time of writing, so they're rendered inline with focus
// highlights and driven from the same arrow-key flow as the rest.
type detailsModel struct {
	aosHome string
	runsDir string

	agent    scheduler.Agent
	metaPath string

	width, height int

	active detailsPane

	// Config pane state — description + schedule on a single tab. focus
	// walks through every editable element (description, kind row, kind-
	// specific fields, save button). `editing` is true while the focused
	// element (textarea or textinput) is actively consuming input — in
	// that mode every key (except ctrl+c, esc, ctrl+s) feeds the field.
	desc           textarea.Model
	configFocus    configFocus
	schedKind      detailsKind
	schedDayCursor int
	editing        bool
	everyHours     textinput.Model
	hour           textinput.Model
	minute         textinput.Model
	days           [7]bool

	// History pane.
	runsTable   table.Model
	runOut      viewport.Model
	runOutID    string
	runsRecords []scheduler.Run

	help help.Model
	keys detailsKeyMap

	toast      string
	toastUntil time.Time
}

type detailsPane int

const (
	paneConfig detailsPane = iota
	paneHistory
	numPanes // sentinel — keep last; used as the modulus for tab cycling
)

type detailsKind int

const (
	kindOff detailsKind = iota
	kindHourly
	kindDaily
)

func (k detailsKind) String() string {
	switch k {
	case kindOff:
		return "off"
	case kindHourly:
		return "hourly"
	case kindDaily:
		return "daily"
	}
	return "off"
}

type configFocus int

const (
	focusDesc configFocus = iota
	focusKind
	focusEveryHours
	focusMinute
	focusDays
	focusHour
)

// Messages from the popup back to startModel.

// popupClosedMsg signals the parent to drop its popup reference. Sent when
// the user hits Esc at nav-level.
type popupClosedMsg struct{}

// agentMetaUpdatedMsg tells the parent that the on-disk meta for an agent
// changed (description or schedule). Parent re-reads the row and refreshes
// the dashboard so the popup-driven edit is reflected immediately.
type agentMetaUpdatedMsg struct {
	agentID string
	meta    scheduler.AgentMeta
}

// runOutputLoadedMsg carries the rendered run record + captured output from
// the goroutine that read them off disk. Keeps disk I/O out of Update.
type runOutputLoadedMsg struct {
	runID   string
	content string
	err     error
}

type detailsKeyMap struct {
	NextPane  key.Binding
	PrevPane  key.Binding
	JumpCfg   key.Binding
	JumpRuns  key.Binding
	Edit      key.Binding
	Save      key.Binding
	Toggle    key.Binding
	Move      key.Binding
	OpenRun   key.Binding
	Close     key.Binding
	Quit      key.Binding

	mode detailsFooterMode
}

type detailsFooterMode int

const (
	footerModeConfigNav detailsFooterMode = iota
	footerModeConfigEditing
	footerModeHistory
)

func newDetailsKeyMap() detailsKeyMap {
	return detailsKeyMap{
		NextPane: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		PrevPane: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧tab", "prev tab")),
		JumpCfg:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "config")),
		JumpRuns: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "history")),
		Edit:     key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/↵", "edit")),
		Save:     key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("^S", "save")),
		Toggle:   key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
		Move:     key.NewBinding(key.WithKeys("←", "→", "↑", "↓"), key.WithHelp("←/→/↑/↓", "move")),
		OpenRun:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "view")),
		Close:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		Quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^C", "quit")),
	}
}

func (k detailsKeyMap) ShortHelp() []key.Binding {
	switch k.mode {
	case footerModeConfigEditing:
		return []key.Binding{
			k.Save,
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "done")),
			k.Quit,
		}
	case footerModeHistory:
		return []key.Binding{k.Move, k.OpenRun, k.NextPane, k.Close, k.Quit}
	default: // footerModeConfigNav
		return []key.Binding{k.Move, k.Edit, k.Save, k.NextPane, k.Close, k.Quit}
	}
}

func (k detailsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

func newDetailsModel(aosHome string, agent scheduler.Agent, h help.Model) *detailsModel {
	runsDir := filepath.Join(aosHome, "runs")

	ta := textarea.New()
	ta.Placeholder = "no description"
	ta.SetValue(agent.Meta.Description)
	ta.CharLimit = 0
	ta.ShowLineNumbers = false

	mkInput := func(val string, w int) textinput.Model {
		ti := textinput.New()
		ti.SetValue(val)
		ti.CharLimit = 3
		ti.SetWidth(w)
		return ti
	}
	// Seed schedule editor with whatever the agent currently has, so a user
	// who opens the popup just to peek doesn't see zeros where they expect
	// the live config.
	kind := kindOff
	var (
		everyVal  = "1"
		hourVal   = "9"
		minuteVal = "0"
	)
	var days [7]bool
	if s := agent.Meta.Schedule; s != nil {
		switch s.Kind {
		case "hourly":
			kind = kindHourly
			everyVal = fmt.Sprint(s.EveryHours)
			minuteVal = fmt.Sprint(s.Minute)
		case "daily":
			kind = kindDaily
			hourVal = fmt.Sprint(s.Hour)
			minuteVal = fmt.Sprint(s.Minute)
			for _, d := range s.Days {
				if idx, ok := weekdayToChipIndex[d]; ok {
					days[idx] = true
				}
			}
		}
	}

	cols := []table.Column{
		{Title: "STATUS", Width: 8},
		{Title: "STARTED", Width: 19},
		{Title: "ELAPSED", Width: 9},
		{Title: "TRIGGER", Width: 9},
		{Title: "EXIT", Width: 5},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(8),
	)
	// Defaults render selected cells with a dim slate that's near-invisible
	// on themed terminals — same trick we pulled in start_model.go's help
	// styling. Re-color through ANSI slots so the theme decides hues.
	ts := table.DefaultStyles()
	ts.Header = ts.Header.Foreground(colorHeader).Bold(true).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(colorMuted)
	ts.Selected = ts.Selected.Foreground(colorEmphasis).Background(colorAccent).Bold(false)
	ts.Cell = ts.Cell.Foreground(colorEmphasis)
	t.SetStyles(ts)

	vp := viewport.New(viewport.WithHeight(6))
	vp.SetContent(styleMuted.Render("(press ↵ on a run to load its details)"))

	return &detailsModel{
		aosHome:     aosHome,
		runsDir:     runsDir,
		agent:       agent,
		metaPath:    agent.MetaPath,
		active:      paneConfig,
		desc:        ta,
		schedKind:   kind,
		configFocus: focusDesc,
		everyHours:  mkInput(everyVal, 4),
		hour:        mkInput(hourVal, 4),
		minute:      mkInput(minuteVal, 4),
		days:        days,
		runsTable:   t,
		runOut:      vp,
		help:        h,
		keys:        newDetailsKeyMap(),
	}
}

// chipIndex order matches what the daily schedule editor displays
// left-to-right; mirrors scheduler.weekdayOrder but starts at Mon so the
// week reads naturally.
var weekdayChipOrder = []scheduler.Weekday{
	scheduler.Mon, scheduler.Tue, scheduler.Wed, scheduler.Thu,
	scheduler.Fri, scheduler.Sat, scheduler.Sun,
}

var weekdayToChipIndex = map[scheduler.Weekday]int{
	scheduler.Mon: 0, scheduler.Tue: 1, scheduler.Wed: 2, scheduler.Thu: 3,
	scheduler.Fri: 4, scheduler.Sat: 5, scheduler.Sun: 6,
}

func (m *detailsModel) Init() tea.Cmd {
	return m.loadRunsCmd()
}

// loadRunsCmd fetches this agent's run history off the main thread. We don't
// cache from startModel because that one only keeps the latest run per agent;
// the popup wants the full history.
func (m *detailsModel) loadRunsCmd() tea.Cmd {
	runsDir := m.runsDir
	agentID := m.agent.ID
	return func() tea.Msg {
		runs, err := scheduler.ReadRuns(runsDir, agentID, 0)
		if err != nil {
			return detailsRunsLoadedMsg{err: err}
		}
		return detailsRunsLoadedMsg{runs: runs}
	}
}

type detailsRunsLoadedMsg struct {
	runs []scheduler.Run
	err  error
}

func (m *detailsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}
	// Description textarea: enough rows to show ~4 lines without being
	// dominant. Schedule fields sit below and need ~10 rows; the rest is
	// for the title + tabs + footer + spacing.
	m.desc.SetWidth(innerW)
	m.desc.SetHeight(4)

	// History pane: chrome (title + tabs + blank + footer + "rows X/Y"
	// caption + output header + viewport border) eats ~10 rows. Split
	// what's left 55/45 between the table and the output viewport so a
	// tall terminal actually fills with history instead of capping at 6.
	const historyChrome = 10
	avail := height - historyChrome
	if avail < 6 {
		avail = 6
	}
	tableH := avail * 55 / 100
	if tableH < 3 {
		tableH = 3
	}
	vpH := avail - tableH
	if vpH < 3 {
		vpH = 3
	}
	m.runsTable.SetHeight(tableH)
	m.runsTable.SetWidth(innerW)
	m.runOut.SetWidth(innerW - 2) // border padding
	m.runOut.SetHeight(vpH)

	m.help.SetWidth(width)
}

func (m *detailsModel) Update(msg tea.Msg) (*detailsModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case detailsRunsLoadedMsg:
		if msg.err != nil {
			m.setToast(fmt.Sprintf("read runs: %v", msg.err))
			return m, m.toastClearCmd()
		}
		m.runsRecords = msg.runs
		m.rebuildRunsTable()
		// Auto-load the latest run's details so the History pane shows
		// something useful the moment the user opens it.
		if len(msg.runs) > 0 {
			return m, m.loadRunOutputCmd(msg.runs[0])
		}
		return m, nil

	case runOutputLoadedMsg:
		if msg.err != nil {
			m.runOut.SetContent(styleErr.Render(msg.err.Error()))
			return m, nil
		}
		m.runOutID = msg.runID
		m.runOut.SetContent(msg.content)
		m.runOut.SetYOffset(0)
		return m, nil

	case refreshDoneMsg:
		// Save already optimistically set "saved"; only adjust the toast
		// when refresh actually failed. Successful refresh stays silent so
		// the user sees one steady "saved" and not a flicker.
		if msg.err != nil {
			m.setToast(fmt.Sprintf("saved, refresh failed: %v", msg.err))
			return m, m.toastClearCmd()
		}
		return m, nil

	case clearToastMsg:
		if time.Now().After(m.toastUntil) {
			m.toast = ""
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *detailsModel) handleKey(msg tea.KeyMsg) (*detailsModel, tea.Cmd) {
	// Ctrl+C always quits the whole program — bubbled up to the parent via a
	// tea.Quit cmd. The parent's Update sees the same key but since we
	// already returned tea.Quit, no double-handling happens.
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	// While a textarea/textinput is actively editing, every key (other than
	// Ctrl+C above, Esc to exit, and Ctrl+S to save) feeds the field.
	if m.editing {
		return m.handleConfigEdit(msg)
	}

	// Nav mode. Tab/shift-tab cycle tabs from any pane; digit keys jump
	// directly. Esc closes from any nav state.
	if key.Matches(msg, m.keys.Close) {
		return m, func() tea.Msg { return popupClosedMsg{} }
	}
	if key.Matches(msg, m.keys.NextPane) {
		m.active = detailsPane((int(m.active) + 1) % int(numPanes))
		return m, nil
	}
	if key.Matches(msg, m.keys.PrevPane) {
		m.active = detailsPane((int(m.active) + int(numPanes) - 1) % int(numPanes))
		return m, nil
	}
	switch msg.String() {
	case "1":
		m.active = paneConfig
		return m, nil
	case "2":
		m.active = paneHistory
		return m, nil
	}

	switch m.active {
	case paneConfig:
		return m.handleConfigNav(msg)
	case paneHistory:
		return m.handleHistoryNav(msg)
	}
	return m, nil
}

// ------------------------- Config pane -------------------------

func (m *detailsModel) handleConfigNav(msg tea.KeyMsg) (*detailsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.configFocusNext()
		return m, nil
	case "k", "up":
		m.configFocusPrev()
		return m, nil
	case "left", "h":
		return m.configAdjust(-1)
	case "right", "l":
		return m.configAdjust(+1)
	}
	if key.Matches(msg, m.keys.Toggle) {
		if m.configFocus == focusDays {
			m.days[m.schedDayCursor] = !m.days[m.schedDayCursor]
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Save) {
		return m.saveAll()
	}
	if key.Matches(msg, m.keys.Edit) {
		return m.configBeginEdit()
	}
	return m, nil
}

func (m *detailsModel) configFocusNext() {
	order := m.configFocusOrder()
	for i, f := range order {
		if f == m.configFocus && i+1 < len(order) {
			m.configFocus = order[i+1]
			return
		}
	}
}

func (m *detailsModel) configFocusPrev() {
	order := m.configFocusOrder()
	for i, f := range order {
		if f == m.configFocus && i > 0 {
			m.configFocus = order[i-1]
			return
		}
	}
}

// configFocusOrder returns the keyboard-cycle order of fields for the current
// schedule kind. Always starts with focusDesc; ends at the last kind-specific
// field. Save is bound to Ctrl+S (and the toast confirms it), so there's no
// "save" entry in the cycle.
func (m *detailsModel) configFocusOrder() []configFocus {
	switch m.schedKind {
	case kindHourly:
		return []configFocus{focusDesc, focusKind, focusEveryHours, focusMinute}
	case kindDaily:
		return []configFocus{focusDesc, focusKind, focusDays, focusHour, focusMinute}
	default:
		return []configFocus{focusDesc, focusKind}
	}
}

func (m *detailsModel) configAdjust(delta int) (*detailsModel, tea.Cmd) {
	switch m.configFocus {
	case focusKind:
		next := int(m.schedKind) + delta
		if next < int(kindOff) {
			next = int(kindOff)
		} else if next > int(kindDaily) {
			next = int(kindDaily)
		}
		m.schedKind = detailsKind(next)
		// New kind may have a different focus chain — clamp focus into it.
		m.configFocus = focusKind
	case focusDays:
		next := m.schedDayCursor + delta
		if next < 0 {
			next = 0
		} else if next > 6 {
			next = 6
		}
		m.schedDayCursor = next
	}
	return m, nil
}

func (m *detailsModel) configBeginEdit() (*detailsModel, tea.Cmd) {
	switch m.configFocus {
	case focusDesc:
		m.editing = true
		return m, m.desc.Focus()
	case focusEveryHours:
		m.editing = true
		return m, m.everyHours.Focus()
	case focusHour:
		m.editing = true
		return m, m.hour.Focus()
	case focusMinute:
		m.editing = true
		return m, m.minute.Focus()
	case focusDays:
		// Toggle the chip the cursor sits on; treats Enter as a press.
		m.days[m.schedDayCursor] = !m.days[m.schedDayCursor]
		return m, nil
	}
	return m, nil
}

func (m *detailsModel) handleConfigEdit(msg tea.KeyMsg) (*detailsModel, tea.Cmd) {
	if key.Matches(msg, m.keys.Save) {
		m.blurAllInputs()
		m.editing = false
		return m.saveAll()
	}
	if key.Matches(msg, m.keys.Close) {
		m.blurAllInputs()
		m.editing = false
		return m, nil
	}
	var cmd tea.Cmd
	switch m.configFocus {
	case focusDesc:
		m.desc, cmd = m.desc.Update(msg)
	case focusEveryHours:
		m.everyHours, cmd = m.everyHours.Update(msg)
	case focusHour:
		m.hour, cmd = m.hour.Update(msg)
	case focusMinute:
		m.minute, cmd = m.minute.Update(msg)
	}
	return m, cmd
}

func (m *detailsModel) blurAllInputs() {
	m.desc.Blur()
	m.everyHours.Blur()
	m.hour.Blur()
	m.minute.Blur()
}

// saveAll persists description and schedule in one shot, the way a "save"
// button is expected to behave. Both writes are skipped when the underlying
// value didn't actually change — touching mtime for nothing would confuse any
// downstream watcher. Cron reconcile is dispatched as a tea.Cmd so the shell
// out to `crontab -l/-w` doesn't freeze the Update goroutine on slow disks.
func (m *detailsModel) saveAll() (*detailsModel, tea.Cmd) {
	descTrimmed := strings.TrimRight(m.desc.Value(), "\n")
	descChanged := descTrimmed != m.agent.Meta.Description

	in := scheduleInput{Kind: m.schedKind.String()}
	switch m.schedKind {
	case kindHourly:
		v, err := atoiSafe(m.everyHours.Value())
		if err != nil {
			m.setToast("every-hours: " + err.Error())
			return m, m.toastClearCmd()
		}
		min, err := atoiSafe(m.minute.Value())
		if err != nil {
			m.setToast("minute: " + err.Error())
			return m, m.toastClearCmd()
		}
		in.EveryHours = v
		in.Minute = min
	case kindDaily:
		days := make([]scheduler.Weekday, 0, 7)
		for i, on := range m.days {
			if on {
				days = append(days, weekdayChipOrder[i])
			}
		}
		if len(days) == 0 {
			m.setToast("daily schedule needs at least one day")
			return m, m.toastClearCmd()
		}
		in.Days = days
		h, err := atoiSafe(m.hour.Value())
		if err != nil {
			m.setToast("hour: " + err.Error())
			return m, m.toastClearCmd()
		}
		min, err := atoiSafe(m.minute.Value())
		if err != nil {
			m.setToast("minute: " + err.Error())
			return m, m.toastClearCmd()
		}
		in.Hour = h
		in.Minute = min
	}
	spec, err := buildScheduleSpec(in)
	if err != nil {
		m.setToast(err.Error())
		return m, m.toastClearCmd()
	}
	schedChanged := !schedulesEqual(spec, m.agent.Meta.Schedule)

	meta := m.agent.Meta
	if descChanged {
		updated, err := scheduler.WriteDescription(m.metaPath, descTrimmed)
		if err != nil {
			m.setToast(fmt.Sprintf("save desc: %v", err))
			return m, m.toastClearCmd()
		}
		meta = updated
	}
	if schedChanged {
		updated, err := scheduler.WriteSchedule(m.metaPath, spec, time.Now())
		if err != nil {
			m.setToast(fmt.Sprintf("save sched: %v", err))
			return m, m.toastClearCmd()
		}
		meta = updated
	}
	m.agent.Meta = meta

	if !descChanged && !schedChanged {
		m.setToast("no changes")
		return m, m.toastClearCmd()
	}

	m.setToast("saved")
	cmds := []tea.Cmd{m.toastClearCmd()}
	agentID := m.agent.ID
	cmds = append(cmds, func() tea.Msg {
		return agentMetaUpdatedMsg{agentID: agentID, meta: meta}
	})
	// Reconcile cron in the background — RunRefresh shells out to crontab
	// and re-scans every agent meta. Surfacing the result via a tea.Msg
	// keeps the UI responsive on slow systems.
	if schedChanged {
		cmds = append(cmds, refreshScheduleCmd())
	}
	return m, tea.Batch(cmds...)
}

// refreshDoneMsg carries the outcome of an async RunRefresh kicked off from
// the popup save flow. The popup updates its toast on receipt.
type refreshDoneMsg struct {
	err error
}

func refreshScheduleCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := RunRefresh()
		return refreshDoneMsg{err: err}
	}
}

// schedulesEqual returns true when two ScheduleSpec pointers describe the
// same firing pattern. Day order doesn't matter (a "mon,fri" save followed
// by "fri,mon" should not rewrite the file). Both nil counts as equal so
// the "no change" path also catches off → off.
func schedulesEqual(a, b *scheduler.ScheduleSpec) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind || a.EveryHours != b.EveryHours ||
		a.Hour != b.Hour || a.Minute != b.Minute {
		return false
	}
	if len(a.Days) != len(b.Days) {
		return false
	}
	have := make(map[scheduler.Weekday]struct{}, len(a.Days))
	for _, d := range a.Days {
		have[d] = struct{}{}
	}
	for _, d := range b.Days {
		if _, ok := have[d]; !ok {
			return false
		}
	}
	return true
}

// atoiSafe wraps strconv.Atoi with a UI-friendlier error message — the
// stdlib's "strconv.Atoi: parsing \"x\": invalid syntax" leaks the package
// path into the toast, which looks broken next to the rest of the UI copy.
func atoiSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("expected a number, got %q", s)
	}
	return n, nil
}

// ------------------------- History pane -------------------------

func (m *detailsModel) handleHistoryNav(msg tea.KeyMsg) (*detailsModel, tea.Cmd) {
	if key.Matches(msg, m.keys.OpenRun) {
		if r, ok := m.selectedRun(); ok {
			return m, m.loadRunOutputCmd(r)
		}
		return m, nil
	}
	// Scroll the run output viewport with ctrl+d/u so the user can read long
	// captures without leaving the table.
	switch msg.String() {
	case "ctrl+d":
		m.runOut.HalfPageDown()
		return m, nil
	case "ctrl+u":
		m.runOut.HalfPageUp()
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.runsTable.Cursor()
	m.runsTable, cmd = m.runsTable.Update(msg)
	// If the cursor moved, auto-load that row's details so the viewport
	// reflects the highlight without an extra keystroke.
	if m.runsTable.Cursor() != prev {
		if r, ok := m.selectedRun(); ok {
			return m, tea.Batch(cmd, m.loadRunOutputCmd(r))
		}
	}
	return m, cmd
}

func (m *detailsModel) selectedRun() (scheduler.Run, bool) {
	idx := m.runsTable.Cursor()
	if idx < 0 || idx >= len(m.runsRecords) {
		return scheduler.Run{}, false
	}
	return m.runsRecords[idx], true
}

// loadRunOutputCmd reads the .out sibling file off the main thread. The
// viewport shows only the captured output — every other field (status,
// elapsed, exit, trigger, startedAt) is already on the table row above, so
// repeating the kv block here just steals space from the actual logs.
//
// ReadRunOutput returns (nil, nil) when the .out file is legitimately
// absent (running, or the run produced no output) — that's not an error,
// just an empty pane. Any other failure (missing .json record, permission
// denied, IO error) flows through runOutputLoadedMsg.err so the user sees
// the actual problem instead of a misleading "(no output)".
func (m *detailsModel) loadRunOutputCmd(r scheduler.Run) tea.Cmd {
	runsDir := m.runsDir
	return func() tea.Msg {
		out, err := scheduler.ReadRunOutput(runsDir, r.ID)
		if err != nil {
			return runOutputLoadedMsg{runID: r.ID, err: err}
		}
		content := string(out)
		if content == "" {
			if r.Error != nil && *r.Error != "" {
				content = styleErr.Render(*r.Error)
			} else {
				content = styleMuted.Render("(no output)")
			}
		}
		return runOutputLoadedMsg{runID: r.ID, content: content}
	}
}

func (m *detailsModel) rebuildRunsTable() {
	rows := make([]table.Row, 0, len(m.runsRecords))
	for _, r := range m.runsRecords {
		rows = append(rows, table.Row{
			statusStyle(string(r.Status)).Render(string(r.Status)),
			formatStartedAt(r.StartedAt),
			elapsedString(r),
			r.Trigger,
			exitString(r),
		})
	}
	m.runsTable.SetRows(rows)
	if len(rows) > 0 {
		m.runsTable.SetCursor(0)
	}
}

// ------------------------- View -------------------------

func (m *detailsModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.renderTitle())
	b.WriteByte('\n')
	b.WriteString(m.renderTabs())
	b.WriteByte('\n')
	b.WriteByte('\n')

	switch m.active {
	case paneConfig:
		b.WriteString(m.renderConfigPane())
	case paneHistory:
		b.WriteString(m.renderHistoryPane())
	}
	b.WriteByte('\n')

	if m.toast != "" {
		b.WriteString(styleWarn.Render("▲ " + m.toast))
		b.WriteByte('\n')
	}

	keys := m.keys
	switch {
	case m.editing:
		keys.mode = footerModeConfigEditing
	case m.active == paneHistory:
		keys.mode = footerModeHistory
	default:
		keys.mode = footerModeConfigNav
	}
	b.WriteString(m.help.View(keys))
	return b.String()
}

func (m *detailsModel) renderTitle() string {
	left := styleAccent.Render(m.agent.ID)
	right := styleMuted.Render(displaySection(m.agent.Section) + " · " + m.agent.ScriptPath)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderTabs draws the tab strip. Unselected tabs use the terminal's default
// foreground (no Foreground call) so they stay legible on every theme —
// previous "Foreground(colorMuted)" rendered nearly invisible on themes
// where ANSI 8 sits very close to the background. Selected tab keeps the
// accent fill so it pops.
func (m *detailsModel) renderTabs() string {
	tabs := []string{"config", "history"}
	var parts []string
	for i, name := range tabs {
		label := fmt.Sprintf(" %d · %s ", i+1, name)
		st := lipgloss.NewStyle().Padding(0, 1)
		if detailsPane(i) == m.active {
			st = st.Foreground(colorEmphasis).Background(colorAccent).Bold(true)
		} else {
			// No Foreground — use the user's default fg, which is always a
			// guaranteed-readable contrast against the background.
			st = st.Bold(false)
		}
		parts = append(parts, st.Render(label))
	}
	return strings.Join(parts, " ")
}

func (m *detailsModel) renderConfigPane() string {
	var b strings.Builder

	// Description block.
	b.WriteString(m.renderDescBlock())
	b.WriteString("\n\n")

	// Schedule block.
	b.WriteString(lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("schedule"))
	b.WriteByte('\n')
	b.WriteString(m.renderKindRow())
	b.WriteString("\n\n")
	switch m.schedKind {
	case kindHourly:
		b.WriteString(m.renderFieldRow("every hours", &m.everyHours, focusEveryHours))
		b.WriteString(m.renderFieldRow("minute     ", &m.minute, focusMinute))
	case kindDaily:
		b.WriteString(m.renderDaysRow())
		b.WriteString(m.renderFieldRow("hour  ", &m.hour, focusHour))
		b.WriteString(m.renderFieldRow("minute", &m.minute, focusMinute))
	}
	return b.String()
}

func (m *detailsModel) renderDescBlock() string {
	header := lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("description")
	focused := m.configFocus == focusDesc
	hint := ""
	switch {
	case focused && m.editing:
		hint = " " + styleAccent.Render("[editing — ^S save, esc done]")
	case focused:
		hint = " " + styleMuted.Render("[e edit]")
	}
	// Border around the textarea so focus state is unambiguous; otherwise
	// the textarea blends into the rest of the pane when not editing.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		BorderForeground(colorMuted)
	if focused {
		box = box.BorderForeground(colorAccent)
	}
	return header + hint + "\n" + box.Render(m.desc.View())
}

func (m *detailsModel) renderKindRow() string {
	var parts []string
	for i, name := range []string{"off", "hourly", "daily"} {
		st := lipgloss.NewStyle().Padding(0, 2).Margin(0, 1)
		selected := detailsKind(i) == m.schedKind
		focused := m.configFocus == focusKind
		switch {
		case selected && focused:
			st = st.Background(colorAccent).Foreground(colorEmphasis).Bold(true)
		case selected:
			st = st.Background(colorMuted).Foreground(colorEmphasis).Bold(true)
		case focused:
			st = st.Foreground(colorAccent)
		default:
			// Default fg — readable on every theme.
		}
		parts = append(parts, st.Render(name))
	}
	hint := ""
	if m.configFocus == focusKind {
		hint = "  " + styleMuted.Render("←/→ change")
	}
	return strings.Join(parts, "") + hint
}

func (m *detailsModel) renderFieldRow(label string, ti *textinput.Model, focus configFocus) string {
	labelStyle := lipgloss.NewStyle().Foreground(colorHeader)
	inputView := ti.View()
	box := lipgloss.NewStyle().Padding(0, 1)
	switch {
	case m.configFocus == focus && m.editing:
		box = box.Foreground(colorEmphasis).Background(colorAccent)
	case m.configFocus == focus:
		box = box.Foreground(colorAccent).Bold(true)
	default:
		box = box.Foreground(colorEmphasis)
	}
	hint := ""
	if m.configFocus == focus && !m.editing {
		hint = " " + styleMuted.Render("[e edit]")
	}
	return labelStyle.Render(label) + "  " + box.Render(inputView) + hint + "\n"
}

func (m *detailsModel) renderDaysRow() string {
	labelStyle := lipgloss.NewStyle().Foreground(colorHeader)
	var parts []string
	for i, d := range weekdayChipOrder {
		chip := strings.ToUpper(string(d)[:1]) + string(d)[1:]
		st := lipgloss.NewStyle().Padding(0, 1).Margin(0, 1)
		on := m.days[i]
		focused := m.configFocus == focusDays && m.schedDayCursor == i
		switch {
		case on && focused:
			st = st.Background(colorAccent).Foreground(colorEmphasis).Bold(true).Underline(true)
		case on:
			st = st.Background(colorMuted).Foreground(colorEmphasis)
		case focused:
			st = st.Foreground(colorAccent).Underline(true)
		default:
			// Default fg — readable on every theme.
		}
		parts = append(parts, st.Render(chip))
	}
	hint := ""
	if m.configFocus == focusDays {
		hint = "  " + styleMuted.Render("←/→ move • space toggle")
	}
	return labelStyle.Render("days") + "  " + strings.Join(parts, "") + hint + "\n"
}

func (m *detailsModel) renderHistoryPane() string {
	var b strings.Builder
	if len(m.runsRecords) == 0 {
		b.WriteString(styleMuted.Render("(no runs yet)"))
		b.WriteByte('\n')
	} else {
		b.WriteString(m.runsTable.View())
		b.WriteByte('\n')
		b.WriteString(m.renderRunsCaption())
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	header := lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("output")
	if m.runOutID != "" {
		header += " " + styleMuted.Render(m.runOutID)
	}
	b.WriteString(header)
	b.WriteByte('\n')
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(0, 1)
	b.WriteString(box.Render(m.runOut.View()))
	return b.String()
}

// renderRunsCaption shows "run X of N" so the user knows whether older runs
// exist past the bottom of the visible window — bubbles/table's internal
// viewport clips silently, which on its own is easy to miss.
func (m *detailsModel) renderRunsCaption() string {
	total := len(m.runsRecords)
	if total == 0 {
		return ""
	}
	cur := m.runsTable.Cursor() + 1
	return styleMuted.Render(fmt.Sprintf("run %d of %d", cur, total))
}

func (m *detailsModel) setToast(s string) {
	m.toast = s
	m.toastUntil = time.Now().Add(toastTTL)
}

func (m *detailsModel) toastClearCmd() tea.Cmd {
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return clearToastMsg{} })
}
