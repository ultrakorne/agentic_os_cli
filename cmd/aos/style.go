package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/term"
)

// Shared styling for human (non --json) output. Lipgloss auto-detects the
// terminal's color profile via termenv and strips styling when stdout isn't a
// TTY, so piping or redirecting these commands still produces clean text.
//
// Palette uses ANSI base 0-15 indices so the terminal's active theme decides
// the actual hues — accents, status, and muted text follow whichever scheme
// the user has configured (Dracula, Solarized, Gruvbox, light/dark, …).
// Anything 16-255 would be a fixed 256-cube slot and ignore the user's theme.
var (
	colorAccent   = lipgloss.Color("4")  // blue   — primary accent
	colorMuted    = lipgloss.Color("8")  // bright black / gray
	colorRunning  = lipgloss.Color("3")  // yellow — in-progress
	colorSuccess  = lipgloss.Color("2")  // green
	colorError    = lipgloss.Color("1")  // red
	colorWarning  = lipgloss.Color("11") // bright yellow — distinct from running
	colorHeader   = lipgloss.Color("6")  // cyan   — secondary accent
	colorEmphasis = lipgloss.Color("15") // bright white / theme foreground

	styleHeader = lipgloss.NewStyle().Foreground(colorHeader).Bold(true)
	// styleLabel / styleMuted use the SGR Faint attribute instead of a fixed
	// foreground so they stay legible on any theme. ANSI 8 (bright black) is
	// dim by design, but on some themes it sits almost on top of the
	// background; Faint asks the terminal to dim *its* default fg, which is
	// always a guaranteed readable contrast away from the background.
	styleLabel = lipgloss.NewStyle().Faint(true)
	styleValue = lipgloss.NewStyle().Foreground(colorEmphasis)
	styleMuted = lipgloss.NewStyle().Faint(true)
	styleAccent = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleWarn   = lipgloss.NewStyle().Foreground(colorWarning)
	styleErr    = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				Padding(0, 1)
	tableCellStyle = lipgloss.NewStyle().Padding(0, 1)
)

// printJSON marshals v with two-space indent and prints to stdout. Centralized
// so every --json branch produces the same shape of trailing newline /
// formatting and there's a single place to swap encoders if we ever need to.
func printJSON(v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(buf))
	return nil
}

// statusStyle returns the foreground style for a JobRun status. Unknown values
// fall back to muted gray so a typo never panics in the renderer.
func statusStyle(status string) lipgloss.Style {
	switch strings.ToLower(status) {
	case "running":
		return lipgloss.NewStyle().Foreground(colorRunning).Bold(true)
	case "success":
		return lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	case "error":
		return lipgloss.NewStyle().Foreground(colorError).Bold(true)
	case "missed":
		// Use the warning palette: a miss is "needs attention" but not the
		// same severity as a failed run that actually produced output.
		return lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	default:
		return styleMuted
	}
}

// newTable returns a table builder pre-configured with the rounded border,
// accent border color, and shared cell padding. Caller supplies headers +
// rows and may override the style for individual columns (e.g. status
// coloring) by chaining .StyleFunc() on the returned table.
//
// When stdout is a TTY and the table's natural width would overflow the
// terminal, the table is sized to termW and Wrap is enabled so lipgloss
// word-wraps the widest column(s) into multiple lines instead of breaking
// the border. Tables that fit comfortably are left tight to content (no
// stretching).
func newTable(headers []string, rows [][]string) *table.Table {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(colorAccent)).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return tableHeaderStyle
			}
			return tableCellStyle
		})
	if termW, ok := stdoutWidth(); ok && tableNaturalWidth(headers, rows) > termW {
		t = t.Width(termW).Wrap(true)
	}
	return t
}

// stdoutWidth returns the terminal width if stdout is a TTY, or (0, false)
// when output is piped/redirected (in which case we leave the table tight to
// content — downstream tools handle their own wrapping).
func stdoutWidth() (int, bool) {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 0, false
	}
	return w, true
}

// tableNaturalWidth estimates how wide the table would render without any
// width constraint: sum of each column's widest cell + per-column padding +
// (cols+1) vertical border chars (RoundedBorder draws separators between
// every column plus the two outer edges). Used to decide whether the table
// needs wrapping at all.
func tableNaturalWidth(headers []string, rows [][]string) int {
	cols := len(headers)
	if cols == 0 {
		return 0
	}
	maxW := make([]int, cols)
	for i, h := range headers {
		if w := lipgloss.Width(h); w > maxW[i] {
			maxW[i] = w
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= cols {
				break
			}
			if w := lipgloss.Width(cell); w > maxW[i] {
				maxW[i] = w
			}
		}
	}
	xPad := tableCellStyle.GetHorizontalPadding()
	total := cols*xPad + cols + 1 // padding + borders
	for _, w := range maxW {
		total += w
	}
	return total
}

// kvRow is one line in a key/value block. Style is optional and applied to the
// value column only; keys are always rendered with styleLabel for consistency.
type kvRow struct {
	Key   string
	Value string
	Style *lipgloss.Style
}

// printKV renders a styled key/value block. Keys right-align within their
// column so the values form a clean vertical edge regardless of label length.
// Empty values fall back to "-" so the block never has trailing whitespace
// holes that look like a rendering bug.
func printKV(rows []kvRow) {
	if len(rows) == 0 {
		return
	}
	keyWidth := 0
	for _, r := range rows {
		if n := lipgloss.Width(r.Key); n > keyWidth {
			keyWidth = n
		}
	}
	keyCol := styleLabel.Width(keyWidth)
	for _, r := range rows {
		val := r.Value
		if strings.TrimSpace(val) == "" {
			val = "-"
		}
		var rendered string
		if r.Style != nil {
			rendered = r.Style.Render(val)
		} else {
			rendered = styleValue.Render(val)
		}
		fmt.Println(keyCol.Render(r.Key) + "  " + rendered)
	}
}

// banner prints a one-line header like "aos refresh" in the accent style.
// Used at the top of multi-line human summaries so the user can spot which
// verb produced the output when scrolling back.
func banner(verb string) {
	fmt.Println(styleAccent.Render("aos " + verb))
}
