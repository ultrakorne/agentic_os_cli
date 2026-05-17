package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

// ErrWaitCanceled bubbles up when the user hit Ctrl+C while `aos run --wait`
// was polling the run record. The detached wrapper is left running; we just
// stop watching and exit non-zero so shells/scripts can tell the difference
// between "run finished" and "I gave up waiting".
var ErrWaitCanceled = errors.New("wait canceled")

// waitFlow runs the bubble-tea progress/spinner on stderr while the detached
// wrapper writes the run record into the store, then prints the .out bytes to
// stdout. estimate < 0 means "no historical data" and forces the indeterminate
// spinner. The caller (runRun) has already printed the Run stub on stdout
// before calling us, so the final layout is:
//
//	stdout: <stub (human or json)>      ← printed by runRun
//	stderr: <progress/spinner>           ← printed here while waiting
//	stdout: <raw .out bytes>             ← printed here after waiting
func waitFlow(store *scheduler.FileRunStore, runID, agentID string, startedAt time.Time, estimate time.Duration) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newWaitModel(ctx, store, runID, agentID, startedAt, estimate)
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	finalM, err := p.Run()
	if err != nil {
		return fmt.Errorf("wait tui: %w", err)
	}
	wm, ok := finalM.(waitModel)
	if !ok {
		return fmt.Errorf("wait tui: unexpected final model %T", finalM)
	}
	if wm.canceled {
		cancel()
		fmt.Fprintf(os.Stderr, "wait canceled — run %s is still executing in the background\n", runID)
		return ErrWaitCanceled
	}
	if wm.err != nil {
		return wm.err
	}
	if wm.final == nil {
		return errors.New("wait ended without a final run record")
	}
	return finalizeRun(store, runID, *wm.final, os.Stdout)
}

// finalizeRun writes the .out bytes for runID to stdout, then returns a
// non-nil error if the run did not succeed (status=error, or exitCode != 0).
// The plan requires output-first, error-second so a failed run's stderr still
// reaches the user before we abort.
func finalizeRun(store *scheduler.FileRunStore, runID string, run scheduler.Run, stdout io.Writer) error {
	data, _ := store.Output(runID)
	if len(data) > 0 {
		_, _ = stdout.Write(data)
	}
	if run.Status == scheduler.StatusError {
		if run.ExitCode != nil {
			return fmt.Errorf("run %s exited with code %d", runID, *run.ExitCode)
		}
		return fmt.Errorf("run %s failed", runID)
	}
	if run.ExitCode != nil && *run.ExitCode != 0 {
		return fmt.Errorf("run %s exited with code %d", runID, *run.ExitCode)
	}
	return nil
}

// waitTickMsg drives a steady redraw cadence so the elapsed counter and
// progress percentage update smoothly even when the underlying poller is
// idle between ticks.
type waitTickMsg time.Time

// waitDoneMsg arrives once the polling goroutine reaches a terminal record
// or its context is canceled.
type waitDoneMsg struct {
	run scheduler.Run
	err error
}

type waitModel struct {
	ctx       context.Context
	store     *scheduler.FileRunStore
	runID     string
	agentID   string
	startedAt time.Time
	estimate  time.Duration

	sp  spinner.Model
	bar progress.Model

	final    *scheduler.Run
	err      error
	canceled bool
}

func newWaitModel(ctx context.Context, store *scheduler.FileRunStore, runID, agentID string, startedAt time.Time, estimate time.Duration) waitModel {
	// Progress bar fills with the "running" status color (ANSI yellow) because
	// it *is* a running indicator — same hue the status field uses elsewhere,
	// so the bar and the word "running" read as one signal. The empty track
	// uses the muted ANSI slot. ANSI 0-15 means the terminal theme decides the
	// actual hues, not the bubbles default purple→pink. The spinner has no
	// Style set, so it renders in the default terminal foreground.
	bar := progress.New(
		progress.WithColors(colorRunning),
		progress.WithoutPercentage(),
	)
	bar.EmptyColor = colorMuted
	return waitModel{
		ctx:       ctx,
		store:     store,
		runID:     runID,
		agentID:   agentID,
		startedAt: startedAt,
		estimate:  estimate,
		sp:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		bar:       bar,
	}
}

func (m waitModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return waitTickMsg(t) }),
		m.waitCmd(),
	}
	if useSpinner(m.estimate) {
		cmds = append(cmds, m.sp.Tick)
	}
	return tea.Batch(cmds...)
}

// useSpinner picks the indeterminate spinner for runs we can't draw a useful
// bar for: no estimate (< 0), or an estimate under 1s where the bar would
// effectively snap from empty to full and look glitchy.
func useSpinner(estimate time.Duration) bool {
	return estimate < time.Second
}

// waitCmd runs the blocking poll in a goroutine. The goroutine respects ctx;
// when the user cancels we cancel ctx and the poll returns ErrWaitCanceled
// quickly. We translate that into our local canceled flag so the caller can
// distinguish it from a real read error.
func (m waitModel) waitCmd() tea.Cmd {
	return func() tea.Msg {
		run, err := scheduler.WaitForRun(m.ctx, m.store, m.runID, 250*time.Millisecond)
		return waitDoneMsg{run: run, err: err}
	}
}

func (m waitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Raw-mode Ctrl+C is read by the input reader as a key event (not a
		// SIGINT), so intercepting "ctrl+c" here is what records the cancel
		// before the program tears down. Without this our caller couldn't
		// distinguish a user cancel from normal completion.
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.canceled = true
			return m, tea.Quit
		}
	case waitTickMsg:
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return waitTickMsg(t) })
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	case waitDoneMsg:
		if msg.err != nil {
			if errors.Is(msg.err, scheduler.ErrWaitCanceled) {
				m.canceled = true
			} else {
				m.err = msg.err
			}
		} else {
			r := msg.run
			m.final = &r
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m waitModel) View() tea.View {
	elapsed := time.Since(m.startedAt).Truncate(time.Millisecond)
	if useSpinner(m.estimate) {
		return tea.NewView(fmt.Sprintf("%s %s\n", m.sp.View(), elapsed))
	}
	pct := float64(elapsed) / float64(m.estimate)
	if pct > 0.99 {
		pct = 0.99
	}
	if pct < 0 {
		pct = 0
	}
	return tea.NewView(fmt.Sprintf("%s  %s\n", m.bar.ViewAs(pct), elapsed))
}
