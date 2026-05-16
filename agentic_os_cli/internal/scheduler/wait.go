package scheduler

import (
	"context"
	"errors"
	"time"
)

// ErrWaitCanceled is returned by WaitForRun when the supplied context is
// canceled before the run reaches a terminal status. Callers can distinguish
// this from a missing run record via errors.Is.
var ErrWaitCanceled = errors.New("wait canceled")

// WaitForRun polls <runsDir>/<runID>.json until the on-disk record reports a
// terminal status (StatusSuccess or StatusError), the context is canceled, or
// a non-recoverable read error occurs.
//
// The wrapper writes the record atomically (temp+rename), but the file does
// not exist for the first hundred milliseconds or so after spawn — wait paths
// tolerate that by treating a NotFoundError as "still running" and continuing
// to poll. Any other read/parse error is returned to the caller verbatim so a
// genuinely broken record surfaces instead of looping forever.
//
// interval defaults to 250ms when <= 0; callers in tests can shorten it.
func WaitForRun(ctx context.Context, runsDir, runID string, interval time.Duration) (JobRun, error) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		run, err := ReadRun(runsDir, runID)
		if err != nil {
			var nf NotFoundError
			if !errors.As(err, &nf) {
				return JobRun{}, err
			}
		} else if run.Status == StatusSuccess || run.Status == StatusError {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return JobRun{}, ErrWaitCanceled
		case <-t.C:
		}
	}
}
