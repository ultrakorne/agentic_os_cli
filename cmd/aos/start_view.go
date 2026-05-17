package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// applyLayout sizes each per-section list and the help footer to fit the
// current window. Width is uniform across sections so all boxes render at
// the same horizontal extent regardless of their content. Height per section
// defaults to len(items) so a 2-agent section stays 2 rows tall instead of
// being inflated to a third of the screen; when the natural sum overflows
// the available rows, every section's list height is scaled down
// proportionally and list.Model handles its own internal scroll.
func (m *startModel) applyLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	chrome := 2 // title + footer
	if m.toast != "" {
		chrome++
	}
	avail := m.height - chrome
	if avail < 1 {
		avail = 1
	}
	if len(m.sections) == 0 {
		return
	}
	// Each section paints 1 title row + 2 border rows around the inner list
	// area. perSectionChrome counts the rows we lose to that frame.
	const perSectionChrome = 3
	// Box outer width includes the two border chars and the two padding
	// chars (Padding(0,1) in renderSectionBox). list content width is what
	// remains inside.
	boxOuter := m.width - 2 // 1-col terminal margin each side
	if boxOuter < 10 {
		boxOuter = 10
	}
	listW := boxOuter - 4
	if listW < 1 {
		listW = 1
	}

	// First pass: each section wants len(items) rows.
	natural := make([]int, len(m.sections))
	totalWant := 0
	for i := range m.sections {
		h := len(m.sections[i].list.Items())
		if h < 1 {
			h = 1 // keep at least one row so list renders its empty state
		}
		natural[i] = h
		totalWant += h + perSectionChrome
	}

	heights := make([]int, len(m.sections))
	if totalWant <= avail {
		// Plenty of room — give every section exactly what it asked for.
		copy(heights, natural)
	} else {
		// Out of room: split available space across sections proportionally
		// to natural demand, with a minimum of 1 list row per section so
		// every box still shows at least one item via list.Model's scroll.
		rows := avail - perSectionChrome*len(m.sections)
		if rows < len(m.sections) {
			rows = len(m.sections)
		}
		for i, n := range natural {
			share := rows * n / totalSum(natural)
			if share < 1 {
				share = 1
			}
			heights[i] = share
		}
	}

	for i := range m.sections {
		m.sections[i].list.SetSize(listW, heights[i])
	}
	m.help.SetWidth(m.width)
}

func totalSum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	if t == 0 {
		return 1
	}
	return t
}

func (m *startModel) View() tea.View {
	if m.width <= 0 || m.height <= 0 {
		return tea.NewView("")
	}

	// Refresh each list's delegate so the per-row Render knows whether its
	// containing section currently has parent focus. Without this, every
	// list highlights its Index() unconditionally — so a non-focused
	// section keeps a stale accent on whatever item the cursor last sat on.
	m.refreshDelegates()

	var b strings.Builder
	b.WriteString(m.renderTitle())
	b.WriteByte('\n')

	if len(m.sections) == 0 {
		b.WriteString(m.renderEmptyState())
		b.WriteByte('\n')
	} else {
		for i, sec := range m.sections {
			b.WriteString(m.renderSectionTitle(i, sec))
			b.WriteByte('\n')
			b.WriteString(m.renderSectionBox(i, sec))
			b.WriteByte('\n')
		}
	}

	if m.toast != "" {
		b.WriteString(styleWarn.Render("▲ " + m.toast))
		b.WriteByte('\n')
	}

	// When the active list is filtering, render its FilterInput here at
	// the parent level. We hid the list's internal filter row in
	// newStartModel so it doesn't shove an empty row into every section's
	// box; the trade-off is that we re-surface the prompt above the
	// footer when actually filtering.
	if m.activeListSettingFilter() {
		b.WriteString(styleAccent.Render("/") + " " + m.sections[m.focused].list.FilterInput.View())
		b.WriteByte('\n')
	}

	// Drive the footer mode so help.View shows the right binding set.
	keys := m.keys
	switch {
	case len(m.sections) == 0:
		keys.mode = footerModeEmpty
	case m.activeListSettingFilter():
		keys.mode = footerModeFiltering
	default:
		keys.mode = footerModeMain
	}
	b.WriteString(m.help.View(keys))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m *startModel) renderTitle() string {
	left := styleAccent.Render("aos start")
	right := styleMuted.Render(m.aosHome)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderSectionTitle is the one-line header above each section's box:
// number, section name, agent count, scheduled count. Focused section gets
// the accent color; unfocused uses the theme's default foreground (no Faint)
// so it stays readable — only the box border dims, not the label itself.
func (m *startModel) renderSectionTitle(idx int, sec sectionPanel) string {
	labelStyle := lipgloss.NewStyle().Bold(true)
	countsStyle := lipgloss.NewStyle()
	if idx == m.focused {
		labelStyle = labelStyle.Foreground(colorAccent)
	} else {
		labelStyle = labelStyle.Foreground(colorHeader)
		countsStyle = countsStyle.Faint(true)
	}
	agentCount := len(sec.list.Items())
	label := labelStyle.Render(fmt.Sprintf("%d · %s", idx+1, displaySection(sec.name)))
	counts := countsStyle.Render(fmt.Sprintf("%d %s · %d scheduled",
		agentCount, pluralAgents(agentCount), sec.scheduledCount))
	return label + "  " + counts
}

// renderSectionBox wraps the list's View in a rounded lipgloss border with
// an explicit inner width so every section box renders at the same
// horizontal extent regardless of the longest row in its list. The focused
// section gets an accent-colored border; unfocused uses the muted border
// slot, but only on the border itself — the contents render at full
// foreground brightness.
func (m *startModel) renderSectionBox(idx int, sec sectionPanel) string {
	innerW := sec.list.Width()
	if innerW < 1 {
		innerW = 1
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Width(innerW).
		Padding(0, 1)
	if idx == m.focused {
		border = border.BorderForeground(colorAccent)
	}
	return border.Render(sec.list.View())
}

// refreshDelegates re-installs each section's ItemDelegate with the current
// sectionFocused flag so the per-row Render only highlights items in the
// active section.
func (m *startModel) refreshDelegates() {
	for i := range m.sections {
		d := agentDelegate{
			idWidth:        m.delegateIDWidth(),
			sectionFocused: i == m.focused,
		}
		m.sections[i].list.SetDelegate(d)
	}
}

// delegateIDWidth recomputes the id column width across all agents. Stored
// on the model would be cheaper, but lists rarely change size after init
// and this keeps the delegate stateless from the model's perspective.
func (m *startModel) delegateIDWidth() int {
	max := 0
	for _, s := range m.sections {
		for _, it := range s.list.Items() {
			a, ok := it.(agentItem)
			if !ok {
				continue
			}
			if w := lipgloss.Width(a.agent.ID); w > max {
				max = w
			}
		}
	}
	if max > 32 {
		max = 32
	}
	return max + 2
}

func (m *startModel) renderEmptyState() string {
	msg := strings.Join([]string{
		"no agents found",
		"",
		"drop an executable script into " + m.aosHome + "/agents/",
		"the filename (without extension) becomes the agent id.",
	}, "\n")
	box := lipgloss.NewStyle().
		Padding(1, 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Render(msg)
	availH := m.height - 2 // title + footer
	if availH < 1 {
		availH = 1
	}
	return lipgloss.Place(m.width, availH, lipgloss.Center, lipgloss.Center, box)
}

func pluralAgents(n int) string {
	if n == 1 {
		return "agent"
	}
	return "agents"
}

// statusGlyph maps a run status (or absence) to a single glyph. Mirrors
// AgentCard.tsx's status row in the Electron renderer.
func statusGlyph(status string, scheduled bool) string {
	switch status {
	case "running":
		return "●"
	case "success":
		return "◆"
	case "error":
		return "▲"
	case "missed":
		return "▽"
	}
	if scheduled {
		return "◇"
	}
	return "·"
}

func relativeFromNow(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}
