package scheduler

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusSuccess RunStatus = "success"
	StatusError   RunStatus = "error"
)

type JobRun struct {
	ID         string    `json:"id"`
	JobID      string    `json:"jobId"`
	ScheduleID *string   `json:"scheduleId"`
	Trigger    string    `json:"trigger"`
	StartedAt  string    `json:"startedAt"`
	EndedAt    *string   `json:"endedAt"`
	Status     RunStatus `json:"status"`
	ExitCode   *int      `json:"exitCode"`
	OutputPath *string   `json:"outputPath"`

	// derived
	StartedAtTime time.Time `json:"-"`
}

func LoadRuns(runsDir string) ([]JobRun, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]JobRun, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(runsDir, e.Name()))
		if err != nil {
			continue
		}
		var r JobRun
		if err := json.Unmarshal(data, &r); err != nil {
			continue
		}
		if r.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, r.StartedAt); err == nil {
				r.StartedAtTime = t
			}
		}
		out = append(out, r)
	}
	return out, nil
}
