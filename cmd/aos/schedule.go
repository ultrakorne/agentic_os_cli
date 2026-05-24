package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ultrakorne/aos_cli/internal/config"
	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// schedule flags. The schedule kind is inferred from which flags the user
// passed — never typed explicitly — so the CLI shape mirrors the sidecar
// JSON shape one-to-one. Conflicting flags (e.g. --every-hours alongside
// --hour) are rejected outright rather than picking a winner.
var (
	schedEveryHours int
	schedHour       int
	schedMinute     int
	schedDays       string
	schedOff        bool
)

// Sentinels for "flag not provided". Cobra doesn't distinguish unset from
// zero for int flags, so we use Changed() on the FlagSet for that.
const (
	schedHourUnset       = -1
	schedEveryHoursUnset = 0
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule <id>",
	Short: "Set or clear an agent's schedule",
	Long: `Set or clear an agent's schedule. The kind is inferred from the flags:

  hourly:  --every-hours N --minute M
  daily:   --hour H --minute M --days mon,tue,wed,thu,fri
           --hour H --minute M --days mon-fri

Pass --off to clear an existing schedule. After a successful write, cron is
reconciled in-process (same as ` + "`aos refresh`" + `).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSchedule,
}

func runSchedule(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	id := args[0]
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.AosHome == "" {
		return errors.New("aos not initialized — run `aos init <path>` first")
	}

	agentsDir := filepath.Join(cfg.AosHome, "agents")
	agent, _, err := scheduler.FindAgentByID(agentsDir, id)
	if err != nil {
		return err
	}

	spec, err := parseSchedFlags(cmd)
	if err != nil {
		return err
	}

	meta, err := scheduler.WriteSchedule(agent.MetaPath, spec, time.Now())
	if err != nil {
		return fmt.Errorf("write meta: %w", err)
	}

	refresh, refErr := runRefresh()
	if refErr != nil {
		// Surface the write but let the user know cron didn't reconcile.
		fmt.Fprintf(os.Stderr, "warn: refresh after schedule write failed: %v\n", refErr)
	} else {
		emitWarnings(refresh.Warnings)
	}

	if JSONOutput() {
		return printScheduleJSON(agent.ID, meta, refresh, refErr)
	}
	return printScheduleHuman(agent.ID, meta, refresh, refErr)
}

// scheduleInput is the explicit form of "what schedule does the user want?",
// decoupled from cobra's "inferred from which flags are set" form. The TUI
// popup constructs one directly; parseSchedFlags translates cobra state into
// one and forwards to buildScheduleSpec so both callers go through the same
// field validation.
type scheduleInput struct {
	Kind       string // "off", "hourly", or "daily"
	EveryHours int
	Hour       int
	Minute     int
	Days       []scheduler.Weekday
}

func buildScheduleSpec(in scheduleInput) (*scheduler.ScheduleSpec, error) {
	switch in.Kind {
	case "off":
		return nil, nil
	case "hourly":
		spec := &scheduler.ScheduleSpec{
			Kind:       "hourly",
			EveryHours: in.EveryHours,
			Minute:     in.Minute,
		}
		if err := scheduler.ValidateSchedule(*spec); err != nil {
			return nil, err
		}
		return spec, nil
	case "daily":
		spec := &scheduler.ScheduleSpec{
			Kind:   "daily",
			Days:   in.Days,
			Hour:   in.Hour,
			Minute: in.Minute,
		}
		if err := scheduler.ValidateSchedule(*spec); err != nil {
			return nil, err
		}
		return spec, nil
	default:
		return nil, fmt.Errorf("unknown schedule kind %q", in.Kind)
	}
}

func parseSchedFlags(cmd *cobra.Command) (*scheduler.ScheduleSpec, error) {
	fs := cmd.Flags()
	everySet := fs.Changed("every-hours")
	hourSet := fs.Changed("hour")
	minuteSet := fs.Changed("minute")
	daysSet := fs.Changed("days")

	if schedOff {
		if everySet || hourSet || minuteSet || daysSet {
			return nil, errors.New("--off cannot be combined with schedule flags")
		}
		return buildScheduleSpec(scheduleInput{Kind: "off"})
	}

	switch {
	case everySet && (hourSet || daysSet):
		return nil, errors.New("--every-hours is for hourly schedules; do not combine with --hour or --days")
	case everySet:
		if !minuteSet {
			return nil, errors.New("hourly schedule requires --minute")
		}
		return buildScheduleSpec(scheduleInput{
			Kind:       "hourly",
			EveryHours: schedEveryHours,
			Minute:     schedMinute,
		})
	case hourSet || daysSet:
		if !hourSet || !minuteSet || !daysSet {
			return nil, errors.New("daily schedule requires --hour, --minute, and --days")
		}
		days, err := parseDays(schedDays)
		if err != nil {
			return nil, err
		}
		return buildScheduleSpec(scheduleInput{
			Kind:   "daily",
			Days:   days,
			Hour:   schedHour,
			Minute: schedMinute,
		})
	default:
		return nil, errors.New("provide a schedule (--every-hours / --hour+--days) or --off to clear")
	}
}

var weekdayOrder = []scheduler.Weekday{
	scheduler.Sun, scheduler.Mon, scheduler.Tue, scheduler.Wed,
	scheduler.Thu, scheduler.Fri, scheduler.Sat,
}

var weekdayIndex = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// parseDays accepts a comma list (`mon,wed,fri`) or a single inclusive range
// (`mon-fri`). Whitespace is tolerated; case is ignored. Duplicate days are
// folded silently — the cron compiler dedupes anyway.
func parseDays(raw string) ([]scheduler.Weekday, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("--days is empty")
	}
	if strings.Contains(s, "-") && !strings.Contains(s, ",") {
		return parseDayRange(s)
	}
	parts := strings.Split(s, ",")
	out := make([]scheduler.Weekday, 0, len(parts))
	for _, p := range parts {
		d, err := parseDay(p)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func parseDayRange(s string) ([]scheduler.Weekday, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid day range %q", s)
	}
	from, err := parseDay(parts[0])
	if err != nil {
		return nil, err
	}
	to, err := parseDay(parts[1])
	if err != nil {
		return nil, err
	}
	fromIdx := weekdayIndex[string(from)]
	toIdx := weekdayIndex[string(to)]
	if fromIdx > toIdx {
		return nil, fmt.Errorf("day range %q wraps the week (start must precede end)", s)
	}
	out := make([]scheduler.Weekday, 0, toIdx-fromIdx+1)
	for i := fromIdx; i <= toIdx; i++ {
		out = append(out, weekdayOrder[i])
	}
	return out, nil
}

func parseDay(raw string) (scheduler.Weekday, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := weekdayIndex[s]; !ok {
		return "", fmt.Errorf("unknown weekday %q (use sun..sat)", raw)
	}
	return scheduler.Weekday(s), nil
}

func printScheduleHuman(id string, meta scheduler.AgentMeta, refresh scheduler.RefreshOutcome, refErr error) error {
	banner("schedule " + id)
	if meta.Schedule == nil {
		clearedStyle := styleMuted
		printKV([]kvRow{{Key: "schedule", Value: "cleared", Style: &clearedStyle}})
	} else {
		rows := []kvRow{{Key: "kind", Value: meta.Schedule.Kind}}
		switch meta.Schedule.Kind {
		case "hourly":
			rows = append(rows,
				kvRow{Key: "everyHours", Value: fmt.Sprintf("%d", meta.Schedule.EveryHours)},
				kvRow{Key: "minute", Value: fmt.Sprintf("%d", meta.Schedule.Minute)},
			)
		case "daily":
			rows = append(rows,
				kvRow{Key: "days", Value: joinDays(meta.Schedule.Days)},
				kvRow{Key: "hour", Value: fmt.Sprintf("%d", meta.Schedule.Hour)},
				kvRow{Key: "minute", Value: fmt.Sprintf("%d", meta.Schedule.Minute)},
			)
		}
		if meta.ScheduledAt != "" {
			rows = append(rows, kvRow{Key: "scheduledAt", Value: meta.ScheduledAt})
		}
		printKV(rows)
	}
	fmt.Println(styleMuted.Render("— refresh —"))
	if refErr != nil {
		errS := styleErr
		printKV([]kvRow{{Key: "error", Value: refErr.Error(), Style: &errS}})
	} else {
		printRefreshHuman(refresh)
	}
	return nil
}

func printScheduleJSON(id string, meta scheduler.AgentMeta, refresh scheduler.RefreshOutcome, refErr error) error {
	payload := map[string]any{
		"id":          id,
		"schedule":    meta.Schedule,
		"scheduledAt": nullIfEmpty(meta.ScheduledAt),
	}
	if refErr != nil {
		payload["refresh"] = map[string]any{"error": refErr.Error()}
	} else {
		payload["refresh"] = refresh
	}
	return printJSON(payload)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func joinDays(days []scheduler.Weekday) string {
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = string(d)
	}
	return strings.Join(parts, ",")
}

func init() {
	scheduleCmd.Flags().IntVar(&schedEveryHours, "every-hours", schedEveryHoursUnset, "hourly cadence in hours (1..12); selects hourly kind")
	scheduleCmd.Flags().IntVar(&schedHour, "hour", schedHourUnset, "hour of day 0..23; selects daily kind")
	scheduleCmd.Flags().IntVar(&schedMinute, "minute", 0, "minute of the hour 0..59")
	scheduleCmd.Flags().StringVar(&schedDays, "days", "", "comma list (mon,wed,fri) or inclusive range (mon-fri); selects daily kind")
	scheduleCmd.Flags().BoolVar(&schedOff, "off", false, "clear the agent's schedule")
	rootCmd.AddCommand(scheduleCmd)
}
