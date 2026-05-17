package main

import (
	"encoding/json"
	"testing"
)

func TestRunStub_shape(t *testing.T) {
	stub := runStub("r-1", "planner", "2026-01-01T00:00:00.000Z")
	buf, err := json.Marshal(stub)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]any{
		"id":         "r-1",
		"agentId":      "planner",
		"scheduleId": nil,
		"trigger":    "manual",
		"startedAt":  "2026-01-01T00:00:00.000Z",
		"endedAt":    nil,
		"status":     "running",
		"output":     "",
		"error":      nil,
		"exitCode":   nil,
		"outputPath": "r-1.out",
		"estimate":   float64(-1),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("stub[%q] = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected keys: got=%v want=%v", got, want)
	}
}

func TestRunStub_usesEstimate(t *testing.T) {
	stub := runStub("r-1", "planner", "2026-01-01T00:00:00.000Z", 2031)
	if got := stub["estimate"]; got != int64(2031) {
		t.Errorf("estimate = %v, want 2031", got)
	}
}


