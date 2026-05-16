package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Shared styling for human (non --json) output. Lipgloss auto-detects the
// terminal's color profile via termenv and strips styling when stdout isn't a
// TTY, so piping or redirecting these commands still produces clean text.
//
// Palette uses ANSI 256-color indices rather than hex so the output remains
// legible on both light and dark backgrounds without baking in a theme.
var (
	colorAccent   = lipgloss.Color("99")  // purple
	colorMuted    = lipgloss.Color("245") // gray
	colorRunning  = lipgloss.Color("214") // amber
	colorSuccess  = lipgloss.Color("42")  // green
	colorError    = lipgloss.Color("203") // red
	colorWarning  = lipgloss.Color("220") // yellow
	colorHeader   = lipgloss.Color("117") // sky blue
	colorEmphasis = lipgloss.Color("252") // light gray (slightly above muted)

	styleHeader = lipgloss.NewStyle().Foreground(colorHeader).Bold(true)
	styleLabel  = lipgloss.NewStyle().Foreground(colorMuted)
	styleValue  = lipgloss.NewStyle().Foreground(colorEmphasis)
	styleMuted  = lipgloss.NewStyle().Foreground(colorMuted)
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
	default:
		return styleMuted
	}
}

// newTable returns a table builder pre-configured with the rounded border,
// accent border color, and the row-striping StyleFunc used across every
// listing command. Caller supplies headers + rows and may override the style
// for individual columns (e.g. status coloring) via SetCellStyleFunc.
func newTable(headers []string, rows [][]string) *table.Table {
	return table.New().
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
