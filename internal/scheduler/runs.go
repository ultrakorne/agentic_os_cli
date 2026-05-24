package scheduler

import "time"

type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusSuccess RunStatus = "success"
	StatusError   RunStatus = "error"
	// StatusMissed marks a scheduled slot the wrapper never fired. Recorded
	// by `aos tick` / `aos refresh`, with startedAt = the expected slot and
	// endedAt = nil. Only the latest uncovered slot per agent is persisted;
	// see internal/scheduler/missed_record.go.
	StatusMissed RunStatus = "missed"
)

// Run mirrors the on-disk wrapper output. Optional fields use pointers so
// JSON round-trips as `null` (matching what the wrapper writes and what
// downstream consumers expect) instead of dropping or zero-defaulting them.
//
// Reads and writes go through FileRunStore (run_store.go); this file only
// defines the data type so the missed/catchup detectors can reference Run
// without an import cycle.
type Run struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agentId"`
	Trigger    string    `json:"trigger"`
	StartedAt  string    `json:"startedAt"`
	EndedAt    *string   `json:"endedAt"`
	Status     RunStatus `json:"status"`
	Output     string    `json:"output"`
	Error      *string   `json:"error"`
	ExitCode   *int      `json:"exitCode"`
	OutputPath *string   `json:"outputPath"`

	// derived
	StartedAtTime time.Time `json:"-"`
}
